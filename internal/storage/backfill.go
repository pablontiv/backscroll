package storage

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/pablontiv/backscroll/internal/corrections"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/templates"
	"time"
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

// BackfillDerived mines templates, corrections, and lossy tool_events from
// stored text for files that are EXPIRED (absent from disk) or have STALE TEMPLATES
// (with normalization_version < current). Results are inserted idempotently (INSERT OR IGNORE).
// Extraction_version=0 marks lossy (reverse-parsed) rows. On-disk files are handled by B1's
// rich re-parse path; this path avoids duplicate lossy rows.
func (d *Database) BackfillDerived(opts BackfillDerivedOpts) error {
	type fileToBackfill struct {
		SourcePath string
		Source     string
	}

	// First, discover and process stale templates (v1 → v2 epoch upgrade).
	// These are templates with normalization_version < CurrentNormalizationVersion.
	stalePaths, err := d.StaleTemplatePaths(CurrentNormalizationVersion)
	if err != nil {
		return fmt.Errorf("query stale template paths: %w", err)
	}

	// Deduplicate stale paths and expired files using a map.
	pathsToProcess := make(map[string]string)
	for _, p := range stalePaths {
		pathsToProcess[p] = "session"
	}

	// Find files in search_items that are EXPIRED: absent from indexed_files.
	// Within expired files, process only those missing at least one of the three derivations:
	// - template_matches (templates mined), OR
	// - correction_signals (corrections detected), OR
	// - tool_events with extraction_version = 0 (lossy tool metadata extracted)
	rows, err := d.db.Query(`
		SELECT DISTINCT si.source_path, si.source
		FROM search_items si
		LEFT JOIN indexed_files ifx ON si.source_path = ifx.path
		WHERE
			ifx.path IS NULL AND
			(NOT EXISTS (SELECT 1 FROM template_matches WHERE source_path = si.source_path) OR
			 NOT EXISTS (SELECT 1 FROM correction_signals WHERE source_path = si.source_path) OR
			 NOT EXISTS (SELECT 1 FROM tool_events WHERE source_path = si.source_path AND extraction_version = 0))
		ORDER BY si.source_path
	`)
	if err != nil {
		return fmt.Errorf("query expired files: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sourcePath, source string
		if err := rows.Scan(&sourcePath, &source); err != nil {
			return fmt.Errorf("scan file: %w", err)
		}
		pathsToProcess[sourcePath] = source
	}

	// Convert map to list of files to backfill
	var filesToBackfill []fileToBackfill
	for path, source := range pathsToProcess {
		filesToBackfill = append(filesToBackfill, fileToBackfill{path, source})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate expired files: %w", err)
	}

	if len(filesToBackfill) == 0 {
		return nil // nothing to backfill
	}

	const batchSize = 100
	totalTemplates := 0
	totalSignals := 0
	totalEvents := 0

	for batchStart := 0; batchStart < len(filesToBackfill); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(filesToBackfill) {
			batchEnd = len(filesToBackfill)
		}
		batch := filesToBackfill[batchStart:batchEnd]

		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}

		batchTemplates := 0
		batchSignals := 0
		batchEvents := 0

		for _, file := range batch {
			// Load messages for this file from search_items
			msgRows, err := tx.Query(`
				SELECT ordinal, role, text, uuid, content_type, was_interrupted
				FROM search_items
				WHERE source_path = ?
				ORDER BY ordinal
			`, file.SourcePath)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("load messages for %s: %w", file.SourcePath, err)
			}

			var messages []IndexedMessage
			for msgRows.Next() {
				var m IndexedMessage
				var uuid sql.NullString
				var wasInterrupted sql.NullInt64
				if err := msgRows.Scan(&m.Ordinal, &m.Role, &m.Text, &uuid,
					&m.ContentType, &wasInterrupted); err != nil {
					msgRows.Close()
					_ = tx.Rollback()
					return fmt.Errorf("scan message: %w", err)
				}
				if uuid.Valid {
					m.UUID = uuid.String
				}
				m.WasInterrupted = wasInterrupted.Valid && wasInterrupted.Int64 != 0
				m.Timestamp = time.Now().Format(time.RFC3339) // not needed for backfill
				m.ExtractionVersion = 0                       // lossy marker
				messages = append(messages, m)
			}
			msgRows.Close()

			// Mine templates from tool messages with is_error
			miner := templates.NewMiner()
			templateCount, err := d.backfillTemplatesForFile(tx, file.SourcePath, messages, miner)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("backfill templates for %s: %w", file.SourcePath, err)
			}
			batchTemplates += templateCount

			// Mine corrections from user prose messages (content_type='text'|'code')
			signalCount, err := d.backfillCorrectionsForFile(tx, file.SourcePath, messages)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("backfill corrections for %s: %w", file.SourcePath, err)
			}
			batchSignals += signalCount

			// Extract lossy tool_events (uuid-NULL rows)
			eventCount, err := d.backfillToolEventsForFile(tx, file.SourcePath, messages)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("backfill tool_events for %s: %w", file.SourcePath, err)
			}
			batchEvents += eventCount
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit backfill batch: %w", err)
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

// backfillTemplatesForFile mines templates from tool messages in the file.
// For backfilled (expired) files, we mine only from ERROR-bearing tool text:
// - rows with "error: " prefix (case-insensitive), OR
// - rows in tool_events with is_error=1
// Input serializations (toolName detected by ParseToolFromSerialized) are always skipped.
// Returns count of unique templates inserted.
func (d *Database) backfillTemplatesForFile(tx *sql.Tx, sourcePath string, messages []IndexedMessage, miner *templates.Miner) (int, error) {
	type errorLine struct {
		toolName string
		text     string
		ordinal  int
		uuid     string
	}
	var errorLines []errorLine

	// Pre-load tool_events with is_error=1 for this file to avoid per-message queries.
	errorEventOrdinals := make(map[int]bool)
	errRows, err := tx.Query(`
		SELECT DISTINCT ordinal FROM tool_events
		WHERE source_path = ? AND is_error = 1
	`, sourcePath)
	if err != nil {
		return 0, fmt.Errorf("query tool_events: %w", err)
	}
	defer errRows.Close()
	for errRows.Next() {
		var ordinal int
		if err := errRows.Scan(&ordinal); err != nil {
			return 0, fmt.Errorf("scan ordinal: %w", err)
		}
		errorEventOrdinals[ordinal] = true
	}
	if err := errRows.Err(); err != nil {
		return 0, err
	}

	// Collect tool message text: only rows that are error-bearing.
	for _, msg := range messages {
		if msg.ContentType != "tool" {
			continue
		}

		// Determine if this row is error-bearing: has error prefix OR error signal.
		trimmed := strings.TrimSpace(msg.Text)
		hasErrorPrefix := strings.HasPrefix(strings.ToLower(trimmed), "error: ")
		hasErrorSignal := errorEventOrdinals[msg.Ordinal]

		// Determine if this row is an input serialization (should always be excluded).
		toolName, _ := ParseToolFromSerialized(msg.Text)
		isInputSerialization := toolName != ""

		// Include only if (error prefix OR error signal) AND NOT input serialization.
		if (!hasErrorPrefix && !hasErrorSignal) || isInputSerialization {
			continue
		}

		// Extract error lines using heuristic extraction (fallback Bash for unknown tools).
		relevantLines := templates.ExtractErrorLines("Bash", msg.Text)
		if len(relevantLines) == 0 {
			relevantLines = []string{msg.Text}
		}
		for _, line := range relevantLines {
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
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, rec.signature, rec.normalizationVersion, rec.text, 1)
		if err != nil {
			return 0, fmt.Errorf("insert template: %w", err)
		}

		// Upsert: if template exists with lower normalization_version, update to current version.
		// This handles the case where a v1 template is being re-mined under v2 heuristics.
		_, err = tx.Exec(`
			UPDATE message_templates
			SET normalization_version = ?
			WHERE signature = ? AND normalization_version < ?
		`, rec.normalizationVersion, rec.signature, rec.normalizationVersion)
		if err != nil {
			return 0, fmt.Errorf("update template normalization_version: %w", err)
		}

		var tmplID int64
		err = tx.QueryRow(`SELECT id FROM message_templates WHERE signature = ?`, rec.signature).Scan(&tmplID)
		if err != nil {
			return 0, fmt.Errorf("query template id: %w", err)
		}

		for _, m := range rec.matches {
			_, err := tx.Exec(`
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
	result1, err := tx.Exec(`
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
	result, err := tx.Exec(`
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
	// Convert to models.Message for detector input (with content-type filter)
	detectionMsgs := make([]models.Message, len(messages))
	for i, m := range messages {
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
	if _, err := tx.Exec(`
		DELETE FROM correction_signals
		WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
	`, sourcePath, CurrentExtractionVersion); err != nil {
		return 0, fmt.Errorf("clear superseded correction_signals: %w", err)
	}

	// Run detectors with prose filter
	detections := backfillDetectCorrections(detectionMsgs)

	// Insert signals (idempotent). Stamped with the current extraction version, not
	// the lossy 0 marker: the detector read the same stored prose either way, so the
	// signal is not lossy, and stamping it current is what lets this converge — a
	// path re-derived under today's rules is no longer discovered as stale.
	count := 0
	for ordinal, dets := range detections {
		for _, det := range dets {
			_, err := tx.Exec(`
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
	count := 0
	for _, m := range messages {
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
		_, err := tx.Exec(`
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
	paths, err := d.queryPaths(`
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
	// uuid, timestamp and extraction_version are the nullable columns here, and legacy
	// and Pi/OpenCode rows do carry NULLs in them; a plain Scan fails on those and
	// would abort the whole caller. Same reason AggregateCorrections coalesces uuid.
	rows, err := d.db.Query(`
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
	return msgs, rows.Err()
}

// BackfillTemplatesForFile re-mines templates for a single source path under current normalization heuristics.
// Used by incremental template re-mining in sync_helpers.go (Q3).
// The sourcePath is needed to update database records and should be provided by the caller
// (obtained from LoadMessagesForPath or StaleTemplatePaths).
// Returns count of deleted stuck templates (those with old normalization_version not reproduced in re-mining).
func (d *Database) BackfillTemplatesForFile(miner *templates.Miner, sourcePath string, msgs []IndexedMessage) (int, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Re-mine templates using internal backfillTemplatesForFile
	// This handles all cases including empty messages (which allows stuck template deletion)
	deletedCount, err := d.backfillTemplatesForFile(tx, sourcePath, msgs, miner)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
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
	paths, err := d.queryPaths(`
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
	// both harmless and convergent. A shared batch transaction would be worse than
	// useless here: a path failing midway would leave its DELETE staged, and skipping
	// to the next path would still commit that DELETE at the end, wiping the signals
	// of the very path the loop claims to have skipped.
	//
	// A path that fails is skipped rather than aborting the run. Discovery order is
	// deterministic, so failing fast would park the same path at the head of every
	// run and nothing behind it would ever be re-derived.
	processed := 0
	var firstErr error
	recordErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, sourcePath := range paths {
		msgs, err := d.LoadMessagesForPath(sourcePath)
		if err != nil {
			recordErr(fmt.Errorf("load messages for %s: %w", sourcePath, err))
			continue
		}
		tx, err := d.db.Begin()
		if err != nil {
			return processed, fmt.Errorf("begin transaction for %s: %w", sourcePath, err)
		}
		if _, err := d.backfillCorrectionsForFile(tx, sourcePath, msgs); err != nil {
			_ = tx.Rollback()
			recordErr(fmt.Errorf("re-derive corrections for %s: %w", sourcePath, err))
			continue
		}
		if err := tx.Commit(); err != nil {
			recordErr(fmt.Errorf("commit re-derivation for %s: %w", sourcePath, err))
			continue
		}
		processed++
	}
	return processed, firstErr
}

// queryPaths runs a query returning a single source_path column and collects it.
// Shared by the stale-template and superseded-correction discovery queries, which
// otherwise repeat the same scan-and-close boilerplate.
func (d *Database) queryPaths(query string, args ...any) ([]string, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan path: %w", err)
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}
