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

// BackfillDerived mines templates, corrections, and lossy tool_events from
// stored text for files that are EXPIRED (absent from disk). Results are inserted
// idempotently (INSERT OR IGNORE). Extraction_version=0 marks lossy (reverse-parsed) rows.
// On-disk files are handled by B1's rich re-parse path; this path avoids duplicate lossy rows.
func (d *Database) BackfillDerived(opts BackfillDerivedOpts) error {
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

	type fileToBackfill struct {
		SourcePath string
		Source     string
	}
	var filesToBackfill []fileToBackfill
	for rows.Next() {
		var sourcePath, source string
		if err := rows.Scan(&sourcePath, &source); err != nil {
			return fmt.Errorf("scan file: %w", err)
		}
		filesToBackfill = append(filesToBackfill, fileToBackfill{sourcePath, source})
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

	var count int
	err = tx.QueryRow(`
		SELECT COUNT(DISTINCT template_id) FROM template_matches WHERE source_path = ?
	`, sourcePath).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
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

	// Run detectors with prose filter
	detections := backfillDetectCorrections(detectionMsgs)

	// Insert signals (idempotent)
	count := 0
	for ordinal, dets := range detections {
		for _, det := range dets {
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO correction_signals
				(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
				VALUES (?, ?, ?, ?, ?, ?)
			`, messages[ordinal].UUID, sourcePath, ordinal, det.DetectorName, det.Confidence, 0) // extraction_version=0 (lossy)
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

// backfillDetectCorrections runs detectors with prose-only filter.
// Replicates the SyncFiles filtering logic: lexicon, rephrase, denial on
// content_type='text'|'code' + role='user' only; interrupt on all user.
func backfillDetectCorrections(msgs []models.Message) map[int][]corrections.Detection {
	return corrections.RunDetectorsFiltered(msgs)
}
