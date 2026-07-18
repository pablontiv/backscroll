package storage

import (
	"fmt"

	"github.com/pablontiv/backscroll/internal/sync"
	"github.com/pablontiv/backscroll/internal/templates"
)

// CurrentExtractionVersion identifies the reader-extraction logic that
// produced a row. Bump when extraction semantics change; rows keep the
// version that actually produced them (perennity: old rows are never
// silently reinterpreted).
const CurrentExtractionVersion = 1

// IndexedMessage represents a message to be indexed.
type IndexedMessage struct {
	Ordinal     int
	Role        string
	Text        string
	UUID        string
	Timestamp   string
	ContentType string
	// F0a rich capture
	ToolName          string
	CommandHead       string
	IsError           *bool
	WasInterrupted    bool
	ExtractionVersion int
}

// IndexedFile represents a file to be synced into the database.
type IndexedFile struct {
	SourcePath string
	Source     string // "session", "plan", "ke", "decision", etc.
	Hash       string
	Project    string
	Messages   []IndexedMessage
	Tags       []string // only used for sessions
}

// SyncFiles syncs a batch of files into the database.
// It uses a transaction to atomically insert all records.
// For each file, it deletes old records and inserts new ones.
func (d *Database) SyncFiles(files []IndexedFile) error {
	if len(files) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, file := range files {
		// Perennial path: session files where every message carries a UUID
		// sync append-only — existing rows (and their ids) are never touched.
		// Flap guard: if the file was previously perennial (DB has uuid-bearing rows),
		// keep it perennial even if this sync has some uuid-less messages (prevents
		// wiping rows during temporary parsing drift).
		// Anything else keeps wipe-and-reload (correct for mutable sources).
		isSession := file.Source == "session" && len(file.Messages) > 0
		allCurrentHaveUUIDs := true
		for _, m := range file.Messages {
			if m.UUID == "" {
				allCurrentHaveUUIDs = false
				break
			}
		}
		perennial := isSession && allCurrentHaveUUIDs

		// Flap guard: a file that was perennial in an earlier sync (DB has
		// uuid-bearing rows) stays perennial even if this parse has uuid-less
		// messages — it must never be wiped.
		if isSession && !perennial {
			var uuidCount int
			err := tx.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ? AND uuid IS NOT NULL", file.SourcePath).Scan(&uuidCount)
			if err != nil {
				return fmt.Errorf("check perennial status for %s: %w", file.SourcePath, err)
			}
			perennial = uuidCount > 0
		}

		if !perennial {
			if _, err := tx.Exec("DELETE FROM template_matches WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old template_matches for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old search_items for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old tool_events for %s: %w", file.SourcePath, err)
			}
		} else {
			// Transition cleanup: rows indexed BEFORE v8 for this same file
			// have uuid NULL; without this one-time delete the uuid-carrying
			// re-parse would duplicate the whole file. Expired files never
			// re-sync, so their legacy rows persist untouched (perennity).
			if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ? AND uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy rows for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ? AND message_uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy tool_events for %s: %w", file.SourcePath, err)
			}
		}

		// Insert new search_items. Use OR IGNORE so that cross-file UUID
		// collisions (rare) are silently skipped rather than aborting the
		// transaction. Use nil (SQL NULL) when uuid is absent — SQLite's
		// UNIQUE constraint allows multiple NULLs but not multiple "".
		// For perennial files, skip messages without UUIDs (flap guard: don't
		// introduce uuid-less rows into a perennial file).
		for _, msg := range file.Messages {
			// Skip uuid-less messages if file is perennial
			if perennial && msg.UUID == "" {
				continue
			}

			var uuidVal interface{}
			if msg.UUID != "" {
				uuidVal = msg.UUID
			}
			var isErrVal interface{}
			if msg.IsError != nil {
				isErrVal = *msg.IsError
			}
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO search_items
				(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				file.Source,
				file.SourcePath,
				msg.Ordinal,
				msg.Role,
				msg.Text,
				msg.Timestamp,
				uuidVal,
				file.Project,
				msg.ContentType,
				msg.ExtractionVersion,
				msg.WasInterrupted,
			)
			if err != nil {
				return fmt.Errorf("insert search_item for %s: %w", file.SourcePath, err)
			}

			if msg.ToolName != "" {
				exitCode := sync.ExtractExitCode(msg.Text, msg.ToolName)
				exitCodeVal := interface{}(nil)
				if exitCode != nil {
					exitCodeVal = *exitCode
				}
				if _, err := tx.Exec(`
					INSERT OR IGNORE INTO tool_events
					(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, uuidVal, file.SourcePath, msg.Ordinal, msg.ToolName, msg.CommandHead, isErrVal, exitCodeVal, msg.ExtractionVersion); err != nil {
					return fmt.Errorf("insert tool_event for %s: %w", file.SourcePath, err)
				}
			}
		}

		// If this is a session (source == "session"), upsert session_tags
		if file.Source == "session" {
			// Delete old tags for this source_path
			if _, err := tx.Exec("DELETE FROM session_tags WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old session_tags for %s: %w", file.SourcePath, err)
			}

			// Insert new tags
			for _, tag := range file.Tags {
				_, err := tx.Exec(`
					INSERT INTO session_tags (source_path, tag)
					VALUES (?, ?)
				`,
					file.SourcePath,
					tag,
				)
				if err != nil {
					return fmt.Errorf("insert session_tag for %s: %w", file.SourcePath, err)
				}
			}
		}

		// Insert or replace in indexed_files
		_, err = tx.Exec(`
			INSERT OR REPLACE INTO indexed_files (path, hash, last_indexed)
			VALUES (?, ?, CURRENT_TIMESTAMP)
		`,
			file.SourcePath,
			file.Hash,
		)
		if err != nil {
			return fmt.Errorf("upsert indexed_files for %s: %w", file.SourcePath, err)
		}

		// Mine templates from error-bearing tool outputs (inside same tx).
		miner := templates.NewMiner()
		if err := d.mineTemplatesForFile(tx, file, miner); err != nil {
			return fmt.Errorf("mine templates for %s: %w", file.SourcePath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Refresh dynamic stopwords (load top stopwords from messages_vocab)
	if err := d.refreshStopwords(); err != nil {
		return fmt.Errorf("refresh stopwords: %w", err)
	}

	return nil
}

// refreshStopwords updates the dynamic_stopwords table with frequently occurring terms.
// It loads the top 1000 terms from the FTS5 vocab and inserts them.
// This helps the search sanitizer filter common words.
func (d *Database) refreshStopwords() error {
	// Clear existing stopwords
	if _, err := d.db.Exec("DELETE FROM dynamic_stopwords"); err != nil {
		return fmt.Errorf("clear stopwords: %w", err)
	}

	// Load top 1000 terms from messages_vocab
	rows, err := d.db.Query(`
		SELECT term FROM messages_vocab
		ORDER BY doc DESC
		LIMIT 1000
	`)
	if err != nil {
		// If messages_vocab doesn't exist yet or is empty, just return
		return nil
	}
	defer func() { _ = rows.Close() }()

	var stopwords []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			continue
		}
		stopwords = append(stopwords, term)
	}
	// Close rows before issuing INSERT; SetMaxOpenConns(1) would deadlock if
	// we held the rows cursor open while acquiring a second connection.
	_ = rows.Close()

	// Insert stopwords
	for _, term := range stopwords {
		if _, err := d.db.Exec("INSERT OR IGNORE INTO dynamic_stopwords (term) VALUES (?)", term); err != nil {
			return fmt.Errorf("insert stopword: %w", err)
		}
	}

	return nil
}

// GetFileHashes returns a map of file paths to their stored hashes.
func (d *Database) GetFileHashes() (map[string]string, error) {
	rows, err := d.db.Query("SELECT path, hash FROM indexed_files")
	if err != nil {
		return nil, fmt.Errorf("query file hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	hashes := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, fmt.Errorf("scan file hash: %w", err)
		}
		hashes[path] = hash
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file hashes: %w", err)
	}

	return hashes, nil
}
