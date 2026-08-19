package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablontiv/backscroll/internal/compat"
)

var (
	snapshotDatabase = SnapshotDatabase
	beginMigrationTx = func(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
		return db.BeginTx(ctx, nil)
	}
)

// SnapshotDatabase creates a read-only reopenable sibling snapshot of srcPath.
func SnapshotDatabase(ctx context.Context, srcPath string) (string, error) {
	targetPath, err := nextSnapshotPath(srcPath)
	if err != nil {
		return "", err
	}

	db, err := sql.Open("sqlite", "file:"+srcPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return "", fmt.Errorf("open snapshot source: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoteSQLString(targetPath)); err != nil {
		_ = os.Remove(targetPath)
		return "", fmt.Errorf("create database snapshot: %w", err)
	}
	if err := fsyncPath(targetPath); err != nil {
		return "", err
	}
	if err := fsyncPath(filepath.Dir(targetPath)); err != nil {
		return "", err
	}
	return targetPath, nil
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

	if planHasDestructiveMigration(plan) {
		if _, err := snapshotDatabase(ctx, d.path); err != nil {
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
}

func isDestructiveMigration(step compat.MigrationStep) bool {
	return step.Version == 5 || step.Version == 6 || step.Version == 8 || step.Version == 9
}

func planHasDestructiveMigration(plan compat.MigrationPlan) bool {
	for _, step := range plan.Steps {
		if isDestructiveMigration(step) {
			return true
		}
	}
	return false
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

func nextSnapshotPath(srcPath string) (string, error) {
	for i := 0; ; i++ {
		candidate := srcPath + ".snapshot"
		if i > 0 {
			candidate = fmt.Sprintf("%s.snapshot.%d", srcPath, i)
		}
		_, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("stat snapshot target: %w", err)
		}
	}
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
