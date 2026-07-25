package storage

import (
	"database/sql"
	"fmt"

	"github.com/pablontiv/backscroll/internal/templates"
)

// isInputSerialization reports whether text is a tool INPUT serialization rather than
// tool output. Inputs are never error lines, so both mining paths exclude them.
func isInputSerialization(text string) bool {
	toolName, _ := ParseToolFromSerialized(text)
	return toolName != ""
}

// shouldMineToolLine is the single line-selection predicate for template mining, used
// by both sync-time (mineTemplatesForFile) and backfill (backfillTemplatesForFile).
//
// It exists because the two paths drifted, and that drift is the defect this predicate
// closes: sync used to select on `ToolName != ""`, which picks the tool_USE message —
// whose text is the input serialization — while the error text lives on the tool_RESULT
// message, which carries no ToolName. Sync therefore mined inputs and never errors, and
// simply excluding inputs on top of that selection left it mining nothing at all.
//
// isError is the caller's error signal: sync reads it off the message, backfill derives
// it from an "error: " prefix or a tool_events row. Everything else is shared.
func shouldMineToolLine(contentType, text string, isError bool) bool {
	if contentType != "tool" || !isError {
		return false
	}
	return !isInputSerialization(text)
}

// mineTemplatesForFile discovers templates from messages with is_error=true
// and writes message_templates + template_matches rows inside the tx.
// Deterministic: same input → same templates + signatures.
func (d *Database) mineTemplatesForFile(tx *sql.Tx, file IndexedFile, miner *templates.Miner) error {
	// Collect error-bearing messages by tool_name.
	type errorLine struct {
		toolName string
		text     string
		ordinal  int
		uuid     string
	}
	var errorLines []errorLine

	for _, msg := range file.Messages {
		if !shouldMineToolLine(msg.ContentType, msg.Text, msg.IsError != nil && *msg.IsError) {
			continue
		}
		// The rows that survive are tool RESULTS, which carry no ToolName — the name
		// lives on the tool_use message in another record. Backfill has the same gap
		// and mines as "Unknown"; matching it keeps the two paths producing identical
		// templates for identical text. Per-tool line calibration is a follow-up.
		toolName := msg.ToolName
		if toolName == "" {
			toolName = "Unknown"
		}
		relevantLines := templates.ExtractErrorLines(toolName, msg.Text)
		for _, line := range relevantLines {
			errorLines = append(errorLines, errorLine{
				toolName: toolName,
				text:     line,
				ordinal:  msg.Ordinal,
				uuid:     msg.UUID,
			})
		}
	}

	// Mine templates and record matches.
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
			sourcePath: file.SourcePath,
			ordinal:    errLine.ordinal,
		})
	}

	// Write templates and matches to database (inside same tx).
	// Occurrence_count is derived at query time from template_matches; just insert idempotently.
	for _, rec := range templateMap {
		// INSERT OR IGNORE ensures idempotency across re-syncs
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, rec.signature, rec.normalizationVersion, rec.text, 1)
		if err != nil {
			return fmt.Errorf("insert template: %w", err)
		}

		// Get template ID (will succeed since we just inserted or it already existed)
		var tmplID int64
		err = tx.QueryRow(`SELECT id FROM message_templates WHERE signature = ?`, rec.signature).Scan(&tmplID)
		if err != nil {
			return fmt.Errorf("query template id: %w", err)
		}

		// Insert matches (UNIQUE constraint prevents duplicates across re-syncs)
		for _, m := range rec.matches {
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO template_matches (template_id, item_uuid, source_path, ordinal)
				VALUES (?, ?, ?, ?)
			`, tmplID, m.uuid, m.sourcePath, m.ordinal)
			if err != nil {
				return fmt.Errorf("insert template_match: %w", err)
			}
		}
	}

	return nil
}

type templateRecord struct {
	signature            string
	text                 string
	normalizationVersion int
	matches              []matchRecord
}

type matchRecord struct {
	uuid       string
	sourcePath string
	ordinal    int
}
