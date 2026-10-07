package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/corrections"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/templates"
)

// BackfillDerivedOpts configures BackfillDerived behavior.
type BackfillDerivedOpts struct {
	// OnProgress is called after each batch with counts: processed files,
	// templates mined, correction signals found, lossy tool_events extracted.
	OnProgress func(processed, templateCount, signalCount, eventCount int)
}

// CurrentNormalizationVersion is the target version for template normalization.
// Incremented when template mining heuristics change (e.g., v1→v2).
const CurrentNormalizationVersion = 2

type derivedBackfillFile struct {
	SourcePath string
}

// BackfillDerived mines derived data without cancellation. It is retained for
// compatibility with existing callers.
func (d *Database) BackfillDerived(opts BackfillDerivedOpts) error {
	return d.BackfillDerivedContext(context.Background(), opts)
}

// BackfillDerivedContext mines templates, corrections, and lossy tool_events
// from stored text for expired, recovered, or stale-template paths. Discovery
// and processing are deterministic by source path. Each batch is atomic:
// cancellation rolls back the active batch, preserves earlier batches, and
// prevents later batches from starting.
func (d *Database) BackfillDerivedContext(ctx context.Context, opts BackfillDerivedOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	stalePaths, err := d.StaleTemplatePathsContext(ctx, CurrentNormalizationVersion)
	if err != nil {
		return fmt.Errorf("query stale template paths: %w", err)
	}
	pathsToProcess := make(map[string]struct{}, len(stalePaths))
	for _, path := range stalePaths {
		pathsToProcess[path] = struct{}{}
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT si.source_path
		FROM search_items si
		LEFT JOIN indexed_files ifx ON si.source_path = ifx.path
		WHERE
			(ifx.path IS NULL OR ifx.hash = ?) AND
			(NOT EXISTS (SELECT 1 FROM template_matches WHERE source_path = si.source_path) OR
			 NOT EXISTS (SELECT 1 FROM correction_signals WHERE source_path = si.source_path) OR
			 NOT EXISTS (SELECT 1 FROM tool_events WHERE source_path = si.source_path AND extraction_version = 0))
		ORDER BY si.source_path ASC
	`, recoveredSourceHash)
	if err != nil {
		return fmt.Errorf("query expired files: %w", err)
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		var sourcePath string
		if err := rows.Scan(&sourcePath); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan file: %w", err)
		}
		pathsToProcess[sourcePath] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate expired files: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close expired files query: %w", err)
	}

	paths := make([]string, 0, len(pathsToProcess))
	for path := range pathsToProcess {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make([]derivedBackfillFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, derivedBackfillFile{SourcePath: path})
	}

	const batchSize = 100
	var totalTemplates, totalSignals, totalEvents int
	for batchStart := 0; batchStart < len(files); batchStart += batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		batchEnd := batchStart + batchSize
		if batchEnd > len(files) {
			batchEnd = len(files)
		}
		batchTemplates, batchSignals, batchEvents, err := d.backfillDerivedBatch(ctx, files[batchStart:batchEnd])
		if err != nil {
			return err
		}
		totalTemplates += batchTemplates
		totalSignals += batchSignals
		totalEvents += batchEvents
		if opts.OnProgress != nil {
			opts.OnProgress(batchEnd, totalTemplates, totalSignals, totalEvents)
		}
	}
	return nil
}

func (d *Database) backfillDerivedBatch(ctx context.Context, batch []derivedBackfillFile) (templatesCount, signalsCount, eventsCount int, retErr error) {
	tx, err := d.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("begin backfill batch: %w", err)
	}
	gate := newSyncTransactionGate(ctx, tx)
	defer func() {
		if rollbackErr := gate.rollback(); rollbackErr != nil && !errors.Is(retErr, rollbackErr) {
			retErr = errors.Join(retErr, fmt.Errorf("rollback backfill batch: %w", rollbackErr))
		}
		if cancelErr := gate.cancellationBeforeCommit(); cancelErr != nil && !errors.Is(retErr, cancelErr) {
			retErr = errors.Join(cancelErr, retErr)
		}
	}()

	for _, file := range batch {
		if err := ctx.Err(); err != nil {
			return 0, 0, 0, err
		}
		msgRows, err := tx.QueryContext(ctx, `
			SELECT ordinal, role, text, uuid, content_type, was_interrupted
			FROM search_items WHERE source_path = ? ORDER BY ordinal ASC
		`, file.SourcePath)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("load messages for %s: %w", file.SourcePath, err)
		}
		var messages []IndexedMessage
		for msgRows.Next() {
			if err := ctx.Err(); err != nil {
				_ = msgRows.Close()
				return 0, 0, 0, err
			}
			var message IndexedMessage
			var uuid sql.NullString
			var wasInterrupted sql.NullInt64
			if err := msgRows.Scan(&message.Ordinal, &message.Role, &message.Text, &uuid, &message.ContentType, &wasInterrupted); err != nil {
				_ = msgRows.Close()
				return 0, 0, 0, fmt.Errorf("scan message for %s: %w", file.SourcePath, err)
			}
			if uuid.Valid {
				message.UUID = uuid.String
			}
			message.WasInterrupted = wasInterrupted.Valid && wasInterrupted.Int64 != 0
			message.Timestamp = time.Now().Format(time.RFC3339)
			message.ExtractionVersion = 0
			messages = append(messages, message)
		}
		if err := ctx.Err(); err != nil {
			_ = msgRows.Close()
			return 0, 0, 0, err
		}
		if err := msgRows.Err(); err != nil {
			_ = msgRows.Close()
			return 0, 0, 0, fmt.Errorf("iterate messages for %s: %w", file.SourcePath, err)
		}
		if err := msgRows.Close(); err != nil {
			return 0, 0, 0, fmt.Errorf("close messages for %s: %w", file.SourcePath, err)
		}

		count, err := d.backfillTemplatesForFileContext(ctx, tx, file.SourcePath, messages, templates.NewMiner())
		if err != nil {
			return 0, 0, 0, fmt.Errorf("backfill templates for %s: %w", file.SourcePath, err)
		}
		templatesCount += count
		count, err = d.backfillCorrectionsForFileContext(ctx, tx, file.SourcePath, messages)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("backfill corrections for %s: %w", file.SourcePath, err)
		}
		signalsCount += count
		count, err = d.backfillToolEventsForFileContext(ctx, tx, file.SourcePath, messages)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("backfill tool_events for %s: %w", file.SourcePath, err)
		}
		eventsCount += count
	}
	if err := gate.commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("commit backfill batch: %w", err)
	}
	return templatesCount, signalsCount, eventsCount, nil
}

// backfillTemplatesForFile mines templates from tool messages in the file.
// For backfilled (expired) files, we mine only from ERROR-bearing tool text:
// - rows with "error: " prefix (case-insensitive), OR
// - rows in tool_events with is_error=1
// Input serializations (toolName detected by ParseToolFromSerialized) are always skipped.
// Returns count of unique templates inserted.
func (d *Database) backfillTemplatesForFile(tx *sql.Tx, sourcePath string, messages []IndexedMessage, miner *templates.Miner) (int, error) {
	return d.backfillTemplatesForFileContext(context.Background(), tx, sourcePath, messages, miner)
}

func (d *Database) backfillTemplatesForFileContext(ctx context.Context, tx *sql.Tx, sourcePath string, messages []IndexedMessage, miner *templates.Miner) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	type errorLine struct {
		toolName string
		text     string
		ordinal  int
		uuid     string
	}
	var errorLines []errorLine

	// Pre-load tool_events with is_error=1 for this file to avoid per-message queries.
	errorEventOrdinals := make(map[int]bool)
	errRows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT ordinal FROM tool_events
		WHERE source_path = ? AND is_error = 1
	`, sourcePath)
	if err != nil {
		return 0, fmt.Errorf("query tool_events: %w", err)
	}
	defer errRows.Close()
	for errRows.Next() {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var ordinal int
		if err := errRows.Scan(&ordinal); err != nil {
			return 0, fmt.Errorf("scan ordinal: %w", err)
		}
		errorEventOrdinals[ordinal] = true
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := errRows.Err(); err != nil {
		return 0, err
	}

	// Collect tool message text: only rows that are error-bearing.
	for _, msg := range messages {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		// Backfill reads stored text, so it recovers the error signal from an
		// "error: " prefix or a tool_events row rather than from a struct field.
		// That determination is all that differs from sync; the selection itself
		// goes through the shared predicate so the two paths cannot drift again.
		trimmed := strings.TrimSpace(msg.Text)
		hasErrorPrefix := strings.HasPrefix(strings.ToLower(trimmed), "error: ")
		hasErrorSignal := errorEventOrdinals[msg.Ordinal]

		if !shouldMineToolLine(msg.ContentType, msg.Text, hasErrorPrefix || hasErrorSignal) {
			continue
		}

		// Extract error lines using heuristic extraction (fallback Bash for unknown tools).
		relevantLines := templates.ExtractErrorLines("Bash", msg.Text)
		if len(relevantLines) == 0 {
			relevantLines = []string{msg.Text}
		}
		for _, line := range relevantLines {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			errorLines = append(errorLines, errorLine{
				toolName: "Unknown", // lossy: tool_name not available
				text:     line,
				ordinal:  msg.Ordinal,
				uuid:     msg.UUID,
			})
		}
	}

	// Mine templates and record matches (unchanged from original).
	templateMap := make(map[string]*templateRecord)
	for _, errLine := range errorLines {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		tmpl := miner.ProcessLine(errLine.text)
		if tmpl.Signature == "" {
			continue
		}

		rec, ok := templateMap[tmpl.Signature]
		if !ok {
			rec = &templateRecord{
				signature:            tmpl.Signature,
				text:                 tmpl.Text,
				normalizationVersion: tmpl.NormalizationVersion,
				matches:              []matchRecord{},
			}
			templateMap[tmpl.Signature] = rec
		}
		rec.matches = append(rec.matches, matchRecord{
			uuid:       errLine.uuid,
			sourcePath: sourcePath,
			ordinal:    errLine.ordinal,
		})
	}

	// Write templates and matches to database.
	for _, rec := range templateMap {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, rec.signature, rec.normalizationVersion, rec.text, 1)
		if err != nil {
			return 0, fmt.Errorf("insert template: %w", err)
		}

		// Upsert: if template exists with lower normalization_version, update to current version.
		// This handles the case where a v1 template is being re-mined under v2 heuristics.
		_, err = tx.ExecContext(ctx, `
			UPDATE message_templates
			SET normalization_version = ?
			WHERE signature = ? AND normalization_version < ?
		`, rec.normalizationVersion, rec.signature, rec.normalizationVersion)
		if err != nil {
			return 0, fmt.Errorf("update template normalization_version: %w", err)
		}

		var tmplID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM message_templates WHERE signature = ?`, rec.signature).Scan(&tmplID)
		if err != nil {
			return 0, fmt.Errorf("query template id: %w", err)
		}

		for _, m := range rec.matches {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			_, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO template_matches (template_id, item_uuid, source_path, ordinal)
				VALUES (?, ?, ?, ?)
			`, tmplID, m.uuid, m.sourcePath, m.ordinal)
			if err != nil {
				return 0, fmt.Errorf("insert template_match: %w", err)
			}
		}
	}

	// Delete templates that were not re-mined (stuck templates with old version).
	// First, delete all matches on this path for old-version templates.
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	result1, err := tx.ExecContext(ctx, `
		DELETE FROM template_matches
		WHERE source_path = ?
		AND template_id IN (
			SELECT id FROM message_templates
			WHERE normalization_version < ?
		)
	`, sourcePath, CurrentNormalizationVersion)
	if err != nil {
		return 0, fmt.Errorf("delete template_matches for old-version templates: %w", err)
	}
	_ = result1 // unused for now

	// Then, delete any orphaned templates (those with no matches anywhere).
	var deletedCount int
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM message_templates
		WHERE normalization_version < ?
		AND id NOT IN (
			SELECT DISTINCT template_id FROM template_matches
		)
	`, CurrentNormalizationVersion)
	if err != nil {
		return 0, fmt.Errorf("delete orphaned templates: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get deleted row count: %w", err)
	}
	deletedCount = int(rowsAffected)

	return deletedCount, nil
}

// backfillCorrectionsForFile detects corrections in prose user messages.
// Returns count of correction_signals inserted.
func (d *Database) backfillCorrectionsForFile(tx *sql.Tx, sourcePath string, messages []IndexedMessage) (int, error) {
	return d.backfillCorrectionsForFileContext(context.Background(), tx, sourcePath, messages)
}

func (d *Database) backfillCorrectionsForFileContext(ctx context.Context, tx *sql.Tx, sourcePath string, messages []IndexedMessage) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Convert to models.Message for detector input (with content-type filter)
	detectionMsgs := make([]models.Message, len(messages))
	for i, m := range messages {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		detectionMsgs[i] = models.Message{
			Role:           m.Role,
			Content:        m.Text,
			ContentType:    m.ContentType,
			UUID:           m.UUID,
			WasInterrupted: m.WasInterrupted,
		}
	}

	// Drop signals from superseded detector epochs first. These sessions have expired
	// from disk, so SyncFiles will never run for them again — this is their only route
	// to a detector fix. Without it a false positive recorded under older rules would
	// be permanent for exactly the sessions the perennial store exists to preserve.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM correction_signals
		WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
	`, sourcePath, CurrentExtractionVersion); err != nil {
		return 0, fmt.Errorf("clear superseded correction_signals: %w", err)
	}

	// Run detectors with prose filter.
	detections, err := corrections.RunDetectorsFilteredContext(ctx, detectionMsgs)
	if err != nil {
		return 0, err
	}

	// Insert signals (idempotent). Stamped with the current extraction version, not
	// the lossy 0 marker: the detector read the same stored prose either way, so the
	// signal is not lossy, and stamping it current is what lets this converge — a
	// path re-derived under today's rules is no longer discovered as stale.
	count := 0
	for ordinal, dets := range detections {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		for _, det := range dets {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			_, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO correction_signals
				(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
				VALUES (?, ?, ?, ?, ?, ?)
			`, messages[ordinal].UUID, sourcePath, ordinal, det.DetectorName, det.Confidence, CurrentExtractionVersion)
			if err != nil {
				return 0, fmt.Errorf("insert correction_signal: %w", err)
			}
			count++
		}
	}
	return count, nil
}

// backfillToolEventsForFile reverse-parses tool input text to extract lossy
// tool metadata (tool_name, command_head). Returns count of tool_events rows inserted.
// NOTE: outputs (tool_result text) cannot be attributed without tool_use_id linkage,
// so they are skipped (ParseToolFromSerialized returns empty toolName for outputs).
func (d *Database) backfillToolEventsForFile(tx *sql.Tx, sourcePath string, messages []IndexedMessage) (int, error) {
	return d.backfillToolEventsForFileContext(context.Background(), tx, sourcePath, messages)
}

func (d *Database) backfillToolEventsForFileContext(ctx context.Context, tx *sql.Tx, sourcePath string, messages []IndexedMessage) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, m := range messages {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if m.ContentType != "tool" {
			continue
		}

		// Extract tool metadata from serialized text.
		// Returns ("", "") for outputs or unmatched text → skip those rows.
		toolName, cmdHead := ParseToolFromSerialized(m.Text)
		if toolName == "" {
			// Not a recognized input structure (likely output text or garbage).
			// tool_events.tool_name is NOT NULL, so we must skip this row.
			// Consequence: tool_result outputs are NOT recoverable (lossy backfill
			// recovers tool_use rows only). This is acceptable: outputs are only
			// valuable if paired with a tool_use via tool_use_id, which requires
			// structured data (uuid pairing) unavailable in expired files.
			continue
		}

		// uuid-NULL for lossy rows (no tool_use_id linkage available)
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO tool_events
			(message_uuid, source_path, ordinal, tool_name, command_head, extraction_version)
			VALUES (?, ?, ?, ?, ?, ?)
		`, nil, sourcePath, m.Ordinal, toolName, cmdHead, 0) // extraction_version=0 (lossy)
		if err != nil {
			return 0, fmt.Errorf("insert lossy tool_event: %w", err)
		}
		count++
	}
	return count, nil
}

// StaleTemplatePaths returns source_paths with templates whose normalization_version is older
// than currentVersion. Results are ordered by ascending source_path (deterministic).
// Used to discover which files need re-mining under newer template heuristics.
func (d *Database) StaleTemplatePaths(currentVersion int) ([]string, error) {
	return d.StaleTemplatePathsContext(context.Background(), currentVersion)
}

// StaleTemplatePathsContext is StaleTemplatePaths with cancellation propagated
// through the query and row iteration.
func (d *Database) StaleTemplatePathsContext(ctx context.Context, currentVersion int) ([]string, error) {
	paths, err := d.queryPathsContext(ctx, `
		SELECT DISTINCT tm.source_path
		FROM template_matches tm
		JOIN message_templates mt ON tm.template_id = mt.id
		WHERE mt.normalization_version < ?
		ORDER BY tm.source_path ASC
	`, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("query stale template paths: %w", err)
	}
	return paths, nil
}

// LoadMessagesForPath loads all IndexedMessage rows from search_items for a given source path,
// ordered by ordinal. Used by incremental template re-mining in sync_helpers.go.
func (d *Database) LoadMessagesForPath(sourcePath string) ([]IndexedMessage, error) {
	return d.LoadMessagesForPathContext(context.Background(), sourcePath)
}

// LoadMessagesForPathContext is LoadMessagesForPath with cancellation propagated
// through the query and row iteration.
func (d *Database) LoadMessagesForPathContext(ctx context.Context, sourcePath string) ([]IndexedMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// uuid, timestamp and extraction_version are the nullable columns here, and legacy
	// and Pi/OpenCode rows do carry NULLs in them; a plain Scan fails on those and
	// would abort the whole caller. Same reason AggregateCorrections coalesces uuid.
	rows, err := d.db.QueryContext(ctx, `
		SELECT
			ordinal,
			COALESCE(uuid, ''),
			role,
			text,
			COALESCE(timestamp, ''),
			content_type,
			COALESCE(extraction_version, 0),
			was_interrupted
		FROM search_items
		WHERE source_path = ?
		ORDER BY ordinal ASC
	`, sourcePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []IndexedMessage
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var msg IndexedMessage
		var wasInterrupted *int
		if err := rows.Scan(
			&msg.Ordinal, &msg.UUID, &msg.Role, &msg.Text, &msg.Timestamp,
			&msg.ContentType, &msg.ExtractionVersion, &wasInterrupted,
		); err != nil {
			return nil, err
		}
		if wasInterrupted != nil {
			msg.WasInterrupted = *wasInterrupted != 0
		}
		msgs = append(msgs, msg)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return msgs, rows.Err()
}

// BackfillTemplatesForFile re-mines templates for a single source path under current normalization heuristics.
// Used by incremental template re-mining in sync_helpers.go (Q3).
// The sourcePath is needed to update database records and should be provided by the caller
// (obtained from LoadMessagesForPath or StaleTemplatePaths).
// Returns count of deleted stuck templates (those with old normalization_version not reproduced in re-mining).
func (d *Database) BackfillTemplatesForFile(miner *templates.Miner, sourcePath string, msgs []IndexedMessage) (int, error) {
	return d.BackfillTemplatesForFileContext(context.Background(), miner, sourcePath, msgs)
}

// BackfillTemplatesForFileContext is BackfillTemplatesForFile with cancellation
// propagated through its per-path transaction. Cancellation rolls back this path.
func (d *Database) BackfillTemplatesForFileContext(ctx context.Context, miner *templates.Miner, sourcePath string, msgs []IndexedMessage) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Re-mine templates using internal backfillTemplatesForFileContext.
	// This handles all cases including empty messages (which allows stuck template deletion).
	deletedCount, err := d.backfillTemplatesForFileContext(ctx, tx, sourcePath, msgs, miner)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return 0, err
	}
	return deletedCount, nil
}

// backfillDetectCorrections runs detectors with prose-only filter.
// Replicates the SyncFiles filtering logic: lexicon, rephrase, denial on
// content_type='text'|'code' + role='user' only; interrupt on all user.
func backfillDetectCorrections(msgs []models.Message) map[int][]corrections.Detection {
	return corrections.RunDetectorsFiltered(msgs)
}

// RederiveSupersededCorrections re-runs correction detection for paths whose signals
// were recorded under a superseded detector epoch, and returns how many paths it
// processed.
//
// It exists because neither existing path reaches those rows. SyncFiles only sees
// files still on disk; BackfillDerived only sees paths absent from indexed_files. A
// session whose JSONL expired but whose indexed_files row remains falls between the
// two, and its stale signals would otherwise be permanent — which is the common case,
// since nothing prunes indexed_files when a file disappears.
//
// Discovery is epoch-based only, so it converges in both directions: a path whose
// signals are re-derived is stamped current and drops out, and a path that re-derives
// to no signals at all has no stale rows left to match. limit bounds the work per run;
// remaining paths drain on later runs in deterministic path order.
func (d *Database) RederiveSupersededCorrections(limit int) (int, error) {
	return d.RederiveSupersededCorrectionsContext(context.Background(), limit)
}

// RederiveSupersededCorrectionsContext is RederiveSupersededCorrections with
// cancellation propagated through discovery and each per-path transaction.
// A canceled active path is rolled back and processing stops immediately; paths
// committed earlier remain committed under the existing convergence semantics.
func (d *Database) RederiveSupersededCorrectionsContext(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	paths, err := d.queryPathsContext(ctx, `
		SELECT DISTINCT source_path FROM correction_signals
		WHERE extraction_version IS NULL OR extraction_version < ?
		ORDER BY source_path
		LIMIT ?
	`, CurrentExtractionVersion, limit)
	if err != nil {
		return 0, fmt.Errorf("query superseded correction paths: %w", err)
	}
	if len(paths) == 0 {
		return 0, nil
	}

	// One transaction PER PATH, not one for the batch. Re-deriving a path deletes its
	// superseded signals before writing the new ones, so that pair must be atomic —
	// but only per path, since paths are independent and partial batch progress is
	// both harmless and convergent. Cancellation therefore preserves earlier commits,
	// rolls back the active path, and stops before any later path begins.
	//
	// Ordinary path failures are skipped rather than aborting the run. Discovery order
	// is deterministic, so failing fast would park the same path at the head forever.
	processed := 0
	var firstErr error
	recordErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, sourcePath := range paths {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		msgs, err := d.LoadMessagesForPathContext(ctx, sourcePath)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return processed, ctxErr
			}
			recordErr(fmt.Errorf("load messages for %s: %w", sourcePath, err))
			continue
		}
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return processed, ctxErr
			}
			return processed, fmt.Errorf("begin transaction for %s: %w", sourcePath, err)
		}
		if _, err := d.backfillCorrectionsForFileContext(ctx, tx, sourcePath, msgs); err != nil {
			_ = tx.Rollback()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return processed, ctxErr
			}
			recordErr(fmt.Errorf("re-derive corrections for %s: %w", sourcePath, err))
			continue
		}
		if err := ctx.Err(); err != nil {
			_ = tx.Rollback()
			return processed, err
		}
		if err := tx.Commit(); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return processed, ctxErr
			}
			recordErr(fmt.Errorf("commit re-derivation for %s: %w", sourcePath, err))
			continue
		}
		processed++
	}
	return processed, firstErr
}

// queryPathsContext runs a query returning one source_path column and collects
// it while propagating cancellation through query execution and row iteration.
func (d *Database) queryPathsContext(ctx context.Context, query string, args ...any) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan path: %w", err)
		}
		paths = append(paths, p)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return paths, rows.Err()
}
