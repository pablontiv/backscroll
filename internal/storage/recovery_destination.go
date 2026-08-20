package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

var recoveryFTSTokenRE = regexp.MustCompile(`[A-Za-z0-9_]{3,}`)

// RecoveryDestinationError describes a failed destination construction attempt.
// Path is populated when a temporary destination path may have leaked; CleanupErr
// is populated when cleanup of that exact path failed.
type RecoveryDestinationError struct {
	Path       string
	Cause      error
	CleanupErr error
}

func (e *RecoveryDestinationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("recovery destination %s failed: %v", e.Path, e.Cause)
	}
	return fmt.Sprintf("recovery destination failed: %v", e.Cause)
}

func (e *RecoveryDestinationError) Unwrap() []error {
	var errs []error
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	if e.CleanupErr != nil {
		errs = append(errs, e.CleanupErr)
	}
	return errs
}

func recoveryDestinationError(path string, cause, cleanupErr error) error {
	if cause == nil {
		cause = cleanupErr
	}
	return &RecoveryDestinationError{Path: path, Cause: cause, CleanupErr: cleanupErr}
}

// CreateRecoveryDestination creates a fresh current-schema recovery database in
// dir, imports the applicable canonical plan in one transaction, verifies the
// uncommitted transaction, commits it, then independently reopens the committed
// bytes read-only and verifies them again. The returned path is safe to expose
// only after that independent verification succeeds.
func CreateRecoveryDestination(ctx context.Context, dir string, plan compat.RecoveryPlan) (path string, err error) {
	if plan.Records == nil {
		return "", fmt.Errorf("recovery plan is inapplicable or contains diagnostics")
	}

	temp, err := os.CreateTemp(dir, ".backscroll-recover-*.db")
	if err != nil {
		return "", fmt.Errorf("create recovery destination temp: %w", err)
	}
	path = temp.Name()
	if closeErr := temp.Close(); closeErr != nil {
		cause := fmt.Errorf("close recovery destination temp: %w", closeErr)
		cleanupErr := recoveryDestinationRemoveFiles(path)
		if cleanupErr != nil {
			return path, recoveryDestinationError(path, cause, fmt.Errorf("cleanup leaked recovery destination temp %s: %w", path, cleanupErr))
		}
		return "", cause
	}
	if err := os.Chmod(path, 0o600); err != nil {
		cause := fmt.Errorf("set recovery destination permissions: %w", err)
		cleanupErr := recoveryDestinationRemoveFiles(path)
		if cleanupErr != nil {
			return path, recoveryDestinationError(path, cause, fmt.Errorf("cleanup leaked recovery destination temp %s: %w", path, cleanupErr))
		}
		return "", cause
	}

	tempPath := path
	cleanup := true
	defer func() {
		if cleanup {
			if cleanupErr := recoveryDestinationRemoveFiles(tempPath); cleanupErr != nil {
				path = tempPath
				err = recoveryDestinationError(tempPath, err, fmt.Errorf("cleanup leaked recovery destination temp %s: %w", tempPath, cleanupErr))
			} else {
				path = ""
			}
		}
	}()

	db, err := Open(path)
	if err != nil {
		return "", fmt.Errorf("initialize fresh recovery destination: %w", err)
	}
	closeDB := true
	defer func() {
		if closeDB {
			if closeErr := db.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close recovery destination database: %w", closeErr))
			}
		}
	}()

	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin recovery import transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback recovery import transaction: %w", rollbackErr))
			}
		}
	}()

	for i, planned := range plan.Records {
		if err := insertRecoveryDestinationRecord(ctx, tx, planned.Record); err != nil {
			return "", fmt.Errorf("insert recovery record %d: %w", i, err)
		}
	}
	if err := insertRecoveryDestinationAccounting(ctx, tx, plan); err != nil {
		return "", fmt.Errorf("insert recovery source accounting: %w", err)
	}
	if err := verifyRecoveryDestinationQueryer(ctx, tx, plan); err != nil {
		return "", fmt.Errorf("verify recovery destination before commit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit recovery import transaction: %w", err)
	}
	committed = true
	if _, err := db.DB().ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return "", fmt.Errorf("checkpoint recovery destination into main database: %w", err)
	}

	if err := db.Close(); err != nil {
		closeDB = false
		return "", fmt.Errorf("close recovery destination before independent verification: %w", err)
	}
	closeDB = false
	if err := removeEmptyRecoveryDestinationSidecars(path); err != nil {
		return "", err
	}

	if err := VerifyRecoveryDestination(ctx, path, plan); err != nil {
		return "", fmt.Errorf("verify committed recovery destination: %w", err)
	}
	cleanup = false
	return path, nil
}

// VerifyRecoveryDestination independently opens path read-only and verifies that
// its committed bytes exactly match the current-schema canonical recovery plan.
func removeEmptyRecoveryDestinationSidecars(path string) error {
	var errs []error
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecar := path + suffix
		info, err := os.Lstat(sidecar)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("lstat recovery destination sidecar %s: %w", sidecar, err))
			continue
		}
		if !info.Mode().IsRegular() || info.Size() != 0 {
			errs = append(errs, fmt.Errorf("recovery destination has unexpected SQLite sidecar %s", sidecar))
			continue
		}
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove empty recovery destination sidecar %s: %w", sidecar, err))
		}
	}
	return errors.Join(errs...)
}

func VerifyRecoveryDestination(ctx context.Context, path string, plan compat.RecoveryPlan) (err error) {
	if plan.Records == nil {
		return fmt.Errorf("recovery plan is inapplicable or contains diagnostics")
	}
	db, err := OpenImmutableReadOnly(path)
	if err != nil {
		return fmt.Errorf("open recovery destination immutable read-only: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close recovery destination immutable read-only %s: %w", path, closeErr))
		}
	}()
	return verifyRecoveryDestinationQueryer(ctx, db.DB(), plan)
}

func insertRecoveryDestinationRecord(ctx context.Context, tx *sql.Tx, r models.IndexedRecord) error {
	// extraction_version and was_interrupted are intentionally NULL here: the
	// canonical recovery record does not carry that lossy derived metadata, and
	// recovered rows remain eligible for safe rederivation by future sync/mining.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
	`, r.Source, r.SourcePath, r.Ordinal, r.Role, r.Text, recoveryDestinationNullableString(r.Timestamp), recoveryDestinationUUIDValue(r.UUID), recoveryDestinationNullableString(r.Project), r.ContentType)
	if err != nil {
		return fmt.Errorf("insert search_items: %w", err)
	}
	return nil
}

func recoveryDestinationSourcePaths(plan compat.RecoveryPlan) []string {
	seen := make(map[string]struct{}, len(plan.Records))
	for _, planned := range plan.Records {
		seen[planned.Record.SourcePath] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func insertRecoveryDestinationAccounting(ctx context.Context, tx *sql.Tx, plan compat.RecoveryPlan) error {
	for _, path := range recoveryDestinationSourcePaths(plan) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO indexed_files (path, hash, last_indexed)
			VALUES (?, ?, NULL)
		`, path, recoveredSourceHash); err != nil {
			return fmt.Errorf("insert recovered source accounting for %s: %w", path, err)
		}
	}
	return nil
}

func verifyRecoveryDestinationQueryer(ctx context.Context, q compat.Queryer, plan compat.RecoveryPlan) error {
	if err := compat.VerifyCurrentShape(ctx, q); err != nil {
		return fmt.Errorf("current schema signature: %w", err)
	}
	if err := verifyRecoveryDestinationForeignKeys(ctx, q); err != nil {
		return err
	}
	if err := verifyRecoveryDestinationRows(ctx, q, plan); err != nil {
		return err
	}
	if err := verifyRecoveryDestinationFTS(ctx, q, plan); err != nil {
		return err
	}
	return nil
}

func verifyRecoveryDestinationRows(ctx context.Context, q compat.Queryer, plan compat.RecoveryPlan) error {
	var rowCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_items`).Scan(&rowCount); err != nil {
		return fmt.Errorf("count search_items: %w", err)
	}
	if rowCount != len(plan.Records) {
		return fmt.Errorf("search_items count = %d, want %d", rowCount, len(plan.Records))
	}
	rows, err := q.QueryContext(ctx, `
		SELECT path, hash, last_indexed
		FROM indexed_files
		ORDER BY path
	`)
	if err != nil {
		return fmt.Errorf("query recovered source accounting: %w", err)
	}

	var gotPaths []string
	for rows.Next() {
		var path, hash string
		var lastIndexed sql.NullString
		if err := rows.Scan(&path, &hash, &lastIndexed); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan recovered source accounting: %w", err)
		}
		if !isRecoveredSourceHash(hash) {
			_ = rows.Close()
			return fmt.Errorf("recovered source accounting for %s has hash %q", path, hash)
		}
		if lastIndexed.Valid {
			_ = rows.Close()
			return fmt.Errorf("recovered source accounting for %s has last_indexed %q", path, lastIndexed.String)
		}
		gotPaths = append(gotPaths, path)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read recovered source accounting: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close recovered source accounting: %w", err)
	}
	wantPaths := recoveryDestinationSourcePaths(plan)
	if !recoveryDestinationStringSlicesEqual(gotPaths, wantPaths) {
		return fmt.Errorf("recovered source accounting paths do not match plan")
	}

	records, diag, err := readCanonicalSearchItems(ctx, q)
	if err != nil {
		return err
	}
	if diag != nil {
		return fmt.Errorf("%s: %s", diag.Code, diag.Summary)
	}
	if len(records) != len(plan.Records) {
		return fmt.Errorf("canonical record count = %d, want %d", len(records), len(plan.Records))
	}

	wantIdentities := map[string]bool{}
	wantPayloads := map[string]bool{}
	wantSources := map[string]int{}
	for i, planned := range plan.Records {
		identity, err := recoveryDestinationIdentity(planned.Record)
		if err != nil {
			return fmt.Errorf("planned record %d identity: %w", i, err)
		}
		if wantIdentities[identity] {
			return fmt.Errorf("planned record %d repeats identity %s", i, identity)
		}
		wantIdentities[identity] = true
		payload, err := recoveryDestinationPayload(planned.Record)
		if err != nil {
			return err
		}
		wantPayloads[identity+"\x00"+payload] = true
		wantSources[planned.Record.Source]++
	}

	gotIdentities := map[string]bool{}
	gotPayloads := map[string]bool{}
	gotSources := map[string]int{}
	for i, record := range records {
		identity, err := recoveryDestinationIdentity(record)
		if err != nil {
			return fmt.Errorf("destination record %d identity: %w", i, err)
		}
		if gotIdentities[identity] {
			return fmt.Errorf("destination repeats identity %s", identity)
		}
		gotIdentities[identity] = true
		payload, err := recoveryDestinationPayload(record)
		if err != nil {
			return err
		}
		gotPayloads[identity+"\x00"+payload] = true
		gotSources[record.Source]++
	}
	if !recoveryDestinationStringBoolMapsEqual(gotIdentities, wantIdentities) {
		return fmt.Errorf("destination identities do not match plan")
	}
	if !recoveryDestinationStringBoolMapsEqual(gotPayloads, wantPayloads) {
		return fmt.Errorf("destination payloads do not match plan")
	}
	if !recoveryDestinationStringIntMapsEqual(gotSources, wantSources) {
		return fmt.Errorf("destination source accounting does not match plan")
	}
	return nil
}

func verifyRecoveryDestinationForeignKeys(ctx context.Context, q compat.Queryer) (err error) {
	rows, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close foreign_key_check rows: %w", closeErr))
		}
	}()
	if rows.Next() {
		return fmt.Errorf("foreign_key_check reported violations")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read foreign_key_check: %w", err)
	}
	return nil
}

func verifyRecoveryDestinationFTS(ctx context.Context, q compat.Queryer, plan compat.RecoveryPlan) error {
	checkedMessages, checkedTools := 0, 0
	for i, planned := range plan.Records {
		record := planned.Record
		switch record.ContentType {
		case "tool":
			checked, err := verifyRecoveryDestinationFTSRecord(ctx, q, "tool_fts", "messages_fts", record)
			if err != nil {
				return fmt.Errorf("verify tool_fts record %d: %w", i, err)
			}
			if checked {
				checkedTools++
			}
		case "text", "code", "reasoning":
			checked, err := verifyRecoveryDestinationFTSRecord(ctx, q, "messages_fts", "tool_fts", record)
			if err != nil {
				return fmt.Errorf("verify messages_fts record %d: %w", i, err)
			}
			if checked {
				checkedMessages++
			}
		}
	}
	if want := recoveryDestinationTokenedCount(plan, "messages_fts"); checkedMessages != want {
		return fmt.Errorf("messages_fts verified count = %d, want %d", checkedMessages, want)
	}
	if want := recoveryDestinationTokenedCount(plan, "tool_fts"); checkedTools != want {
		return fmt.Errorf("tool_fts verified count = %d, want %d", checkedTools, want)
	}
	return nil
}

func verifyRecoveryDestinationFTSRecord(ctx context.Context, q compat.Queryer, table, wrongTable string, record models.IndexedRecord) (bool, error) {
	match := recoveryDestinationFTSToken(record.Text)
	if match == "" {
		return false, nil
	}
	id, err := recoveryDestinationRecordID(ctx, q, record)
	if err != nil {
		return false, fmt.Errorf("locate %s row: %w", table, err)
	}
	if err := verifyRecoveryDestinationFTSCount(ctx, q, table, match, id, 1); err != nil {
		return false, err
	}
	if err := verifyRecoveryDestinationFTSCount(ctx, q, wrongTable, match, id, 0); err != nil {
		return false, err
	}
	return true, nil
}

func verifyRecoveryDestinationFTSCount(ctx context.Context, q compat.Queryer, table, match string, id int64, want int) error {
	query, err := recoveryDestinationFTSCountSQL(table)
	if err != nil {
		return err
	}
	var got int
	if err := q.QueryRowContext(ctx, query, match, id).Scan(&got); err != nil {
		return fmt.Errorf("count %s virtual table hits for rowid %d: %w", table, id, err)
	}
	if got != want {
		return fmt.Errorf("%s virtual table hits for rowid %d token %q = %d, want %d", table, id, match, got, want)
	}
	return nil
}

func recoveryDestinationFTSCountSQL(table string) (string, error) {
	switch table {
	case "messages_fts":
		return `SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH ? AND rowid = ?`, nil
	case "tool_fts":
		return `SELECT COUNT(*) FROM tool_fts WHERE tool_fts MATCH ? AND rowid = ?`, nil
	default:
		return "", fmt.Errorf("unknown FTS table %q", table)
	}
}

func recoveryDestinationRecordID(ctx context.Context, q compat.Queryer, record models.IndexedRecord) (int64, error) {
	var id int64
	if err := q.QueryRowContext(ctx, `
		SELECT id FROM search_items
		WHERE source_path = ? AND ordinal = ? AND COALESCE(uuid, '') = ? AND text = ?
	`, record.SourcePath, record.Ordinal, recoveryDestinationStringValue(record.UUID), record.Text).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func recoveryDestinationTokenedCount(plan compat.RecoveryPlan, table string) int {
	count := 0
	for _, planned := range plan.Records {
		record := planned.Record
		if recoveryDestinationFTSToken(record.Text) == "" {
			continue
		}
		if table == "tool_fts" && record.ContentType == "tool" {
			count++
		}
		if table == "messages_fts" && (record.ContentType == "text" || record.ContentType == "code" || record.ContentType == "reasoning") {
			count++
		}
	}
	return count
}

func recoveryDestinationIdentity(r models.IndexedRecord) (string, error) {
	if r.UUID != nil && *r.UUID != "" {
		if _, err := uuid.Parse(*r.UUID); err != nil {
			return "", err
		}
		return "uuid\x00" + *r.UUID, nil
	}
	if r.SourcePath == "" || r.Ordinal < 0 {
		return "", fmt.Errorf("unsafe path/ordinal identity source_path=%q ordinal=%d", r.SourcePath, r.Ordinal)
	}
	return "path_ordinal\x00" + r.SourcePath + "\x00" + strconv.FormatInt(r.Ordinal, 10), nil
}

func recoveryDestinationPayload(r models.IndexedRecord) (string, error) {
	if r.UUID != nil && *r.UUID == "" {
		r.UUID = nil
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("encode recovery payload: %w", err)
	}
	return string(encoded), nil
}

func recoveryDestinationNullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func recoveryDestinationUUIDValue(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func recoveryDestinationStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func recoveryDestinationFTSToken(text string) string {
	tokens := recoveryFTSTokenRE.FindAllString(text, -1)
	if len(tokens) == 0 {
		return ""
	}
	sort.SliceStable(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })
	return strings.ToLower(tokens[0])
}

func recoveryDestinationStringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i, leftValue := range left {
		if right[i] != leftValue {
			return false
		}
	}
	return true
}

func recoveryDestinationStringBoolMapsEqual(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}

func recoveryDestinationStringIntMapsEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}

func recoveryDestinationRemoveFiles(path string) error {
	if path == "" {
		return nil
	}
	var errs []error
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		candidate := path + suffix
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove recovery destination temp %s: %w", candidate, err))
		}
	}
	return errors.Join(errs...)
}
