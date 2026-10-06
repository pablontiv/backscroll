package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pablontiv/backscroll/internal/compat"
)

var beginMigrationTx = func(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, nil)
}

const snapshotRetention = 2

type retainedSnapshot struct {
	path   string
	suffix int
	info   os.FileInfo
}

// SnapshotDatabase creates and validates a durable sibling backup of srcPath.
// The backup is published atomically without replacing an existing path.
func SnapshotDatabase(ctx context.Context, srcPath string, plans ...compat.MigrationPlan) (snapshotPath string, err error) {
	plan, err := snapshotPlan(ctx, srcPath, plans)
	if err != nil {
		return "", err
	}
	stem, err := snapshotStem(srcPath, plan)
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(srcPath)

	existing, nextSuffix, err := inspectRetainedSnapshots(ctx, stem, plan.From)
	if err != nil {
		return "", err
	}
	targetPath := stem
	if nextSuffix > 0 {
		targetPath = fmt.Sprintf("%s.%d", stem, nextSuffix)
	}

	tempFile, err := os.CreateTemp(directory, "."+filepath.Base(stem)+".tmp-")
	if err != nil {
		return "", fmt.Errorf("create snapshot temporary name: %w", err)
	}
	tempPath := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("close snapshot temporary file: %w", closeErr)
	}
	if removeErr := os.Remove(tempPath); removeErr != nil {
		return "", fmt.Errorf("prepare snapshot temporary path: %w", removeErr)
	}

	published := false
	defer func() {
		if tempPath != "" {
			if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove snapshot temporary file: %w", removeErr))
			}
		}
		if err != nil && published {
			if removeErr := os.Remove(targetPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove failed snapshot publication: %w", removeErr))
			}
			if syncErr := fsyncPath(directory); syncErr != nil {
				err = errors.Join(err, syncErr)
			}
		}
	}()

	if err := vacuumSnapshot(ctx, srcPath, tempPath); err != nil {
		return "", err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return "", fmt.Errorf("set snapshot permissions: %w", err)
	}
	if err := validateSnapshot(ctx, tempPath, plan.From); err != nil {
		return "", fmt.Errorf("validate new snapshot: %w", err)
	}
	if err := fsyncPath(tempPath); err != nil {
		return "", err
	}

	if err := pruneSnapshots(existing, snapshotRetention-1); err != nil {
		return "", err
	}
	if len(existing) > snapshotRetention-1 {
		if err := fsyncPath(directory); err != nil {
			return "", err
		}
	}

	if err := os.Link(tempPath, targetPath); err != nil {
		return "", fmt.Errorf("publish snapshot without clobbering %s: %w", targetPath, err)
	}
	published = true
	if err := os.Remove(tempPath); err != nil {
		return "", fmt.Errorf("remove published snapshot temporary link: %w", err)
	}
	tempPath = ""
	if err := fsyncPath(directory); err != nil {
		return "", err
	}
	return targetPath, nil
}

func snapshotPlan(ctx context.Context, srcPath string, plans []compat.MigrationPlan) (compat.MigrationPlan, error) {
	if len(plans) > 1 {
		return compat.MigrationPlan{}, fmt.Errorf("snapshot accepts at most one migration plan")
	}
	if len(plans) == 1 {
		if len(plans[0].Steps) == 0 {
			return compat.MigrationPlan{}, fmt.Errorf("snapshot requires a non-empty migration plan")
		}
		return plans[0], nil
	}

	db, err := OpenReadOnly(srcPath)
	if err != nil {
		return compat.MigrationPlan{}, fmt.Errorf("inspect snapshot source: %w", err)
	}
	plan, diag, inspectErr := compat.InspectIndex(ctx, db.DB())
	closeErr := db.Close()
	if inspectErr != nil {
		return compat.MigrationPlan{}, fmt.Errorf("inspect snapshot source: %w", inspectErr)
	}
	if diag != nil {
		return compat.MigrationPlan{}, fmt.Errorf("inspect snapshot source: %s: %s", diag.Code, diag.Summary)
	}
	if closeErr != nil {
		return compat.MigrationPlan{}, fmt.Errorf("close snapshot source inspection: %w", closeErr)
	}
	if len(plan.Steps) == 0 {
		plan.Steps = []compat.MigrationStep{{Version: plan.From.AppliedVersion, Name: "current schema backup"}}
	}
	return plan, nil
}

func snapshotStem(srcPath string, plan compat.MigrationPlan) (string, error) {
	signature := strings.TrimPrefix(plan.From.Signature, "sha256:")
	if len(signature) < 12 {
		return "", fmt.Errorf("invalid source schema signature %q", plan.From.Signature)
	}
	for _, char := range signature[:12] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return "", fmt.Errorf("invalid source schema signature %q", plan.From.Signature)
		}
	}
	toVersion := plan.Steps[len(plan.Steps)-1].Version
	if toVersion <= 0 {
		return "", fmt.Errorf("invalid migration target version %d", toVersion)
	}
	return fmt.Sprintf("%s.snapshot-v%d-%s-to-v%d", srcPath, plan.From.AppliedVersion, signature[:12], toVersion), nil
}

func inspectRetainedSnapshots(ctx context.Context, stem string, expected compat.SchemaShape) ([]retainedSnapshot, int, error) {
	entries, err := os.ReadDir(filepath.Dir(stem))
	if err != nil {
		return nil, 0, fmt.Errorf("list retained snapshots: %w", err)
	}
	base := filepath.Base(stem)
	var snapshots []retainedSnapshot
	maxSuffix := -1
	for _, entry := range entries {
		suffix, ok := snapshotSuffix(base, entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(filepath.Dir(stem), entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, 0, fmt.Errorf("inspect retained snapshot %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("refuse unsafe retained snapshot %s", path)
		}
		if err := validateSnapshot(ctx, path, expected); err != nil {
			return nil, 0, fmt.Errorf("refuse unvalidated retained snapshot %s: %w", path, err)
		}
		snapshots = append(snapshots, retainedSnapshot{path: path, suffix: suffix, info: info})
		if suffix > maxSuffix {
			maxSuffix = suffix
		}
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].suffix < snapshots[j].suffix })
	return snapshots, maxSuffix + 1, nil
}

func snapshotSuffix(base, name string) (int, bool) {
	if name == base {
		return 0, true
	}
	prefix := base + "."
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(name, prefix)
	if raw == "" || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	suffix, err := strconv.Atoi(raw)
	if err != nil || suffix <= 0 {
		return 0, false
	}
	return suffix, true
}

func vacuumSnapshot(ctx context.Context, srcPath, targetPath string) error {
	db, err := sql.Open("sqlite", "file:"+srcPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open snapshot source: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoteSQLString(targetPath)); err != nil {
		_ = db.Close()
		return fmt.Errorf("create database snapshot: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close snapshot source: %w", err)
	}
	return nil
}

func validateSnapshot(ctx context.Context, path string, expected compat.SchemaShape) (err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("snapshot mode is %s, want regular 0600", info.Mode())
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open snapshot for validation: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close snapshot validation connection: %w", closeErr))
		}
	}()

	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("run snapshot integrity_check: %w", err)
	}
	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read snapshot integrity_check: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read snapshot integrity_check: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close snapshot integrity_check: %w", err)
	}
	if len(results) != 1 || results[0] != "ok" {
		return fmt.Errorf("snapshot integrity_check failed: %s", strings.Join(results, "; "))
	}

	plan, diag, err := compat.InspectIndex(ctx, db)
	if err != nil {
		return fmt.Errorf("inspect snapshot shape: %w", err)
	}
	if diag != nil {
		return fmt.Errorf("inspect snapshot shape: %s: %s", diag.Code, diag.Summary)
	}
	if plan.From != expected {
		return fmt.Errorf("snapshot schema shape changed: got %+v want %+v", plan.From, expected)
	}
	return nil
}

func pruneSnapshots(snapshots []retainedSnapshot, keep int) error {
	removeCount := len(snapshots) - keep
	for i := 0; i < removeCount; i++ {
		current, err := os.Lstat(snapshots[i].path)
		if err != nil {
			return fmt.Errorf("recheck retained snapshot %s: %w", snapshots[i].path, err)
		}
		if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(current, snapshots[i].info) {
			return fmt.Errorf("retained snapshot changed before removal: %s", snapshots[i].path)
		}
		if err := os.Remove(snapshots[i].path); err != nil {
			return fmt.Errorf("remove retained snapshot %s: %w", snapshots[i].path, err)
		}
	}
	return nil
}

// ApplyMigrationPlan applies a checked compatibility migration plan atomically.
func (d *Database) ApplyMigrationPlan(ctx context.Context, plan compat.MigrationPlan) error {
	if len(plan.Steps) == 0 {
		return nil
	}
	if d.path == "" {
		return fmt.Errorf("database path is not bound to receiver")
	}

	if planIncludesVersion(plan, 9) {
		if err := prepareV9ToolEventDuplicates(ctx, d.db); err != nil {
			return err
		}
	}

	v6Recorded := plan.From.AppliedVersion >= 6 || planIncludesVersion(plan, 6)
	tx, err := beginMigrationTx(ctx, d.db)
	if err != nil {
		return fmt.Errorf("begin migration plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	livePlan, diag, err := compat.InspectIndex(ctx, tx)
	if err != nil {
		return fmt.Errorf("re-inspect schema in migration transaction: %w", err)
	}
	if livePlan.From.Signature != plan.From.Signature {
		return fmt.Errorf("index schema changed since inspection: got %s want %s", livePlan.From.Signature, plan.From.Signature)
	}
	if diag != nil && !planStartsFromEmptySchema(plan) {
		return fmt.Errorf("re-inspect schema in migration transaction: %s: %s", diag.Code, diag.Summary)
	}
	if planIncludesVersion(plan, 9) {
		if err := prepareV9ToolEventDuplicates(ctx, tx); err != nil {
			return err
		}
	}

	for _, step := range plan.Steps {
		if step.Version > 6 && !v6Recorded {
			if err := recordMigration(ctx, tx, 6, "V6 drop phantom source_metadata column", sqlV6Drop, "record migration v6"); err != nil {
				return err
			}
			v6Recorded = true
		}

		apply, ok := migrationPlanDispatch[step]
		if !ok {
			return fmt.Errorf("unsupported migration step %d %q", step.Version, step.Name)
		}
		if err := apply(ctx, tx, plan.From); err != nil {
			return err
		}
	}

	if err := compat.VerifyCurrentShape(ctx, tx); err != nil {
		return fmt.Errorf("verify final schema before commit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration plan: %w", err)
	}

	verify, err := OpenReadOnly(d.path)
	if err != nil {
		return fmt.Errorf("reopen migrated database read-only: %w", err)
	}
	defer func() { _ = verify.Close() }()
	if err := compat.VerifyCurrentShape(ctx, verify.DB()); err != nil {
		return fmt.Errorf("verify committed schema: %w", err)
	}
	return nil
}

type migrationApplier func(context.Context, *sql.Tx, compat.SchemaShape) error

func (d *Database) applySingleMigration(apply migrationApplier) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := apply(context.Background(), tx, compat.SchemaShape{}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

var migrationPlanDispatch = map[compat.MigrationStep]migrationApplier{
	{Version: 1, Name: "V1 core schema"}:                                                 applyV1,
	{Version: 2, Name: "V2 embedding tables"}:                                            applyV2,
	{Version: 3, Name: "V3 embedding blob column"}:                                       applyV3,
	{Version: 4, Name: "V4 tool_fts trigram index"}:                                      applyV4,
	{Version: 5, Name: "V5 drop phantom session_events"}:                                 applyV5,
	{Version: 6, Name: "V6 drop source_metadata when present"}:                           applyV6,
	{Version: 7, Name: "V7 reasoning triggers"}:                                          applyV7,
	{Version: 8, Name: "V8 perennity: extraction_version, was_interrupted, tool_events"}: applyV8,
	{Version: 9, Name: "V9 tool_events uuid uniqueness index"}:                           applyV9,
	{Version: 10, Name: "V10 template mining: message_templates, template_matches"}:      applyV10,
	{Version: 11, Name: "V11 correction detection: correction_signals"}:                  applyV11,
	{Version: 12, Name: "V12 agent classification: annotations"}:                         applyV12,
	{Version: 13, Name: "V13 backfill discovery indexes"}:                                applyV13,
	{Version: 14, Name: "V14 file metadata prefilter"}:                                   applyV14,
	{Version: 15, Name: "V15 search echo provenance"}:                                    applyV15,
	{Version: 16, Name: "V16 parser-backed message origin"}:                              applyV16,
}

func planStartsFromEmptySchema(plan compat.MigrationPlan) bool {
	return plan.From.AppliedVersion == 0 && len(plan.Steps) > 0 && plan.Steps[0].Version == 1
}

func planIncludesVersion(plan compat.MigrationPlan, version int) bool {
	for _, step := range plan.Steps {
		if step.Version == version {
			return true
		}
	}
	return false
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func fsyncPath(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open for fsync %s: %w", path, err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", path, err)
	}
	return nil
}

func applyV1(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV1Core); err != nil {
		return fmt.Errorf("create core tables: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV1FTS5); err != nil {
		return fmt.Errorf("create FTS5 virtual table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV1Triggers); err != nil {
		return fmt.Errorf("create triggers: %w", err)
	}
	return recordMigration(ctx, tx, 1, "V1 core schema", sqlV1CoreDDL, "record migration")
}

func applyV2(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV2); err != nil {
		return fmt.Errorf("create embedding tables: %w", err)
	}
	return recordMigration(ctx, tx, 2, "V2 embedding tables", sqlV2, "record migration v2")
}

func applyV3(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV3); err != nil {
		return fmt.Errorf("add embedding column: %w", err)
	}
	return recordMigration(ctx, tx, 3, "V3 embedding blob column", sqlV3, "record migration v3")
}

func applyV4(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV4ToolFTS); err != nil {
		return fmt.Errorf("create tool_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV4Triggers); err != nil {
		return fmt.Errorf("rebuild triggers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV4Repopulate); err != nil {
		return fmt.Errorf("repopulate indexes: %w", err)
	}
	return recordMigration(ctx, tx, 4, "V4 tool_fts trigram index", sqlV4ToolFTS+sqlV4Triggers, "record migration v4")
}

func applyV5(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV5Drop); err != nil {
		return fmt.Errorf("drop session_events: %w", err)
	}
	return recordMigration(ctx, tx, 5, "V5 drop phantom session_events", sqlV5Drop, "record migration v5")
}

func applyV6(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV6Drop); err != nil {
		return fmt.Errorf("drop source_metadata column: %w", err)
	}
	return recordMigration(ctx, tx, 6, "V6 drop phantom source_metadata column", sqlV6Drop, "record migration v6")
}

func applyV7(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV7Triggers); err != nil {
		return fmt.Errorf("rebuild triggers for reasoning: %w", err)
	}
	return recordMigration(ctx, tx, 7, "V7 reasoning content_type routes to messages_fts", sqlV7Triggers, "record migration v7")
}

func applyV8(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV8); err != nil {
		return fmt.Errorf("apply v8 perennity schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV8SearchItemsRebuild); err != nil {
		return fmt.Errorf("rebuild search_items for v8 shape: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV7Triggers); err != nil {
		return fmt.Errorf("restore search_items triggers after v8 rebuild: %w", err)
	}
	if _, err := tx.ExecContext(ctx, sqlV4Repopulate); err != nil {
		return fmt.Errorf("repopulate indexes after v8 rebuild: %w", err)
	}
	return recordMigration(ctx, tx, 8, "V8 perennity: extraction_version, was_interrupted, tool_events", sqlV8, "record migration v8")
}

func applyV9(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV9); err != nil {
		return fmt.Errorf("apply v9 tool_events uuid uniqueness: %w", err)
	}
	return recordMigration(ctx, tx, 9, "V9 tool_events uuid uniqueness index", sqlV9, "record migration v9")
}

type queryContexter interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func prepareV9ToolEventDuplicates(ctx context.Context, q queryContexter) error {
	existsRows, err := q.QueryContext(ctx, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'tool_events' LIMIT 1`)
	if err != nil {
		return fmt.Errorf("inspect V9 tool_events table: %w", err)
	}
	toolEventsExists := existsRows.Next()
	if err := existsRows.Err(); err != nil {
		_ = existsRows.Close()
		return fmt.Errorf("read V9 tool_events table: %w", err)
	}
	if err := existsRows.Close(); err != nil {
		return fmt.Errorf("close V9 tool_events table probe: %w", err)
	}
	if !toolEventsExists {
		return nil
	}

	rows, err := q.QueryContext(ctx, `
		SELECT
			message_uuid,
			source_path,
			ordinal,
			tool_name,
			command_head,
			is_error,
			exit_code,
			extraction_version
		FROM tool_events
		WHERE message_uuid IS NOT NULL
		ORDER BY message_uuid, id
	`)
	if err != nil {
		return fmt.Errorf("inspect V9 tool_events duplicates: %w", err)
	}
	defer rows.Close()

	seen := map[string]v9ToolEventRow{}
	for rows.Next() {
		var payload v9ToolEventRow
		if err := rows.Scan(
			&payload.MessageUUID,
			&payload.SourcePath,
			&payload.Ordinal,
			&payload.ToolName,
			&payload.CommandHead,
			&payload.IsError,
			&payload.ExitCode,
			&payload.ExtractionVersion,
		); err != nil {
			return fmt.Errorf("scan V9 tool_events duplicates: %w", err)
		}
		prior, ok := seen[payload.MessageUUID]
		if !ok {
			seen[payload.MessageUUID] = payload
			continue
		}
		if prior != payload {
			return fmt.Errorf("conflicting tool_events for message_uuid %q before V9 uniqueness migration", payload.MessageUUID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read V9 tool_events duplicates: %w", err)
	}
	return nil
}

type v9ToolEventRow struct {
	MessageUUID       string
	SourcePath        string
	Ordinal           int64
	ToolName          string
	CommandHead       sql.NullString
	IsError           sql.NullInt64
	ExitCode          sql.NullInt64
	ExtractionVersion int64
}

func applyV10(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV10); err != nil {
		return fmt.Errorf("apply v10 template-mining schema: %w", err)
	}
	return recordMigration(ctx, tx, 10, "V10 template mining: message_templates, template_matches", sqlV10, "record migration v10")
}

func applyV11(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV11); err != nil {
		return fmt.Errorf("apply v11 correction_signals schema: %w", err)
	}
	return recordMigration(ctx, tx, 11, "V11 correction detection: correction_signals", sqlV11, "record migration v11")
}

func applyV12(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV12); err != nil {
		return fmt.Errorf("apply v12 annotations schema: %w", err)
	}
	return recordMigration(ctx, tx, 12, "V12 agent classification: annotations (free-form labels; enum freeze deferred)", sqlV12, "record migration v12")
}

func applyV13(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV13); err != nil {
		return fmt.Errorf("apply v13 indexes: %w", err)
	}
	return recordMigration(ctx, tx, 13, "V13 backfill discovery indexes", sqlV13, "record migration v13")
}

func applyV14(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
	_ = from
	if _, err := tx.ExecContext(ctx, sqlV14); err != nil {
		return fmt.Errorf("apply v14 metadata prefilter columns: %w", err)
	}
	return recordMigration(ctx, tx, 14, "V14 file metadata prefilter", sqlV14, "record migration v14")
}

// NULL on existing session rows queues evidence-based re-parsing. New writes
// without reader provenance default to ordinary searchable content.
const sqlV15 = `
ALTER TABLE search_items ADD COLUMN search_echo INTEGER DEFAULT 0;
UPDATE search_items SET search_echo = NULL WHERE source = 'session';
`

func applyV15(ctx context.Context, tx *sql.Tx, _ compat.SchemaShape) error {
	if _, err := tx.ExecContext(ctx, sqlV15); err != nil {
		return fmt.Errorf("apply v15 search echo provenance: %w", err)
	}
	return recordMigration(ctx, tx, 15, "V15 search echo provenance", sqlV15, "record migration v15")
}

const sqlV16 = `
ALTER TABLE search_items ADD COLUMN origin TEXT NOT NULL DEFAULT 'unknown'
    CHECK (origin IN ('human', 'assistant', 'system', 'automation', 'unknown'));
ALTER TABLE search_items ADD COLUMN origin_version INTEGER;
`

func applyV16(ctx context.Context, tx *sql.Tx, _ compat.SchemaShape) error {
	if _, err := tx.ExecContext(ctx, sqlV16); err != nil {
		return fmt.Errorf("apply v16 message origin: %w", err)
	}
	return recordMigration(ctx, tx, 16, "V16 parser-backed message origin", sqlV16, "record migration v16")
}

func recordMigration(ctx context.Context, tx *sql.Tx, version int, name string, body string, errorPrefix string) error {
	checksum := sha256.Sum256([]byte(body))
	checksumHex := fmt.Sprintf("%x", checksum)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
	`, version, name, checksumHex); err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return nil
}

const sqlV5Drop = `
DROP INDEX IF EXISTS idx_session_events_order;
DROP INDEX IF EXISTS idx_session_events_project;
DROP TABLE IF EXISTS session_events;
`

const sqlV6Drop = `ALTER TABLE search_items DROP COLUMN source_metadata;`

const sqlV8 = `
ALTER TABLE search_items ADD COLUMN extraction_version INTEGER;
ALTER TABLE search_items ADD COLUMN was_interrupted INTEGER;
CREATE TABLE IF NOT EXISTS tool_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    tool_name TEXT NOT NULL,
    command_head TEXT,
    is_error INTEGER,
    exit_code INTEGER,
    extraction_version INTEGER NOT NULL,
    UNIQUE(source_path, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_tool_events_tool ON tool_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_events_uuid ON tool_events(message_uuid);
`

const sqlV8SearchItemsRebuild = `
DROP TRIGGER IF EXISTS search_items_ai;
DROP TRIGGER IF EXISTS search_items_ad;
DROP TRIGGER IF EXISTS search_items_au;
ALTER TABLE search_items RENAME TO search_items_v8_old;
CREATE TABLE search_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    role TEXT NOT NULL,
    text TEXT NOT NULL,
    timestamp TEXT,
    uuid TEXT UNIQUE,
    project TEXT,
    content_type TEXT NOT NULL DEFAULT 'text',
    extraction_version INTEGER,
    was_interrupted INTEGER
);
INSERT INTO search_items (id, source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
    SELECT id, source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted
    FROM search_items_v8_old;
DROP TABLE search_items_v8_old;
CREATE INDEX IF NOT EXISTS idx_search_items_source_path ON search_items(source_path);
CREATE INDEX IF NOT EXISTS idx_search_items_project ON search_items(project);
`

const sqlV9 = `
DELETE FROM tool_events WHERE message_uuid IS NOT NULL AND id NOT IN (
    SELECT MIN(id) FROM tool_events WHERE message_uuid IS NOT NULL GROUP BY message_uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_events_uuid_unique ON tool_events(message_uuid) WHERE message_uuid IS NOT NULL;
`

const sqlV10 = `
CREATE TABLE IF NOT EXISTS message_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    signature TEXT UNIQUE NOT NULL,
    normalization_version INTEGER NOT NULL,
    template_text TEXT NOT NULL,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    first_seen TEXT,
    last_seen TEXT
);
CREATE INDEX IF NOT EXISTS idx_templates_sig ON message_templates(signature);
CREATE INDEX IF NOT EXISTS idx_templates_version ON message_templates(normalization_version);

CREATE TABLE IF NOT EXISTS template_matches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    UNIQUE(source_path, ordinal, template_id),
    FOREIGN KEY(template_id) REFERENCES message_templates(id)
);
CREATE INDEX IF NOT EXISTS idx_matches_template ON template_matches(template_id);
CREATE INDEX IF NOT EXISTS idx_matches_uuid ON template_matches(item_uuid);
`

const sqlV11 = `
CREATE TABLE IF NOT EXISTS correction_signals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    detector TEXT NOT NULL,
    confidence REAL NOT NULL,
    extraction_version INTEGER NOT NULL,
    UNIQUE(source_path, ordinal, detector)
);
CREATE INDEX IF NOT EXISTS idx_correction_signals_detector ON correction_signals(detector);
CREATE INDEX IF NOT EXISTS idx_correction_signals_confidence ON correction_signals(confidence DESC);
`

const sqlV12 = `
CREATE TABLE IF NOT EXISTS annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'agent',
    created_at TEXT NOT NULL,
    UNIQUE(source_path, ordinal, kind)
);
CREATE INDEX IF NOT EXISTS idx_annotations_uuid ON annotations(item_uuid);
CREATE INDEX IF NOT EXISTS idx_annotations_kind ON annotations(kind);
`

const sqlV13 = `
CREATE INDEX IF NOT EXISTS idx_template_matches_source ON template_matches(source_path);
CREATE INDEX IF NOT EXISTS idx_correction_signals_source ON correction_signals(source_path);
`

const sqlV14 = `
ALTER TABLE indexed_files ADD COLUMN file_size INTEGER;
ALTER TABLE indexed_files ADD COLUMN file_mtime TEXT;
`
