package storage

import (
	"encoding/json"
	"fmt"
)

// IndexedMessage represents a message to be indexed.
type IndexedMessage struct {
	Ordinal     int
	Role        string
	Text        string
	UUID        string
	Timestamp   string
	ContentType string
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
		// Delete old search_items for this source_path
		if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ?", file.SourcePath); err != nil {
			return fmt.Errorf("delete old search_items for %s: %w", file.SourcePath, err)
		}

		// Delete old session_events for this source_path
		if _, err := tx.Exec("DELETE FROM session_events WHERE source_path = ?", file.SourcePath); err != nil {
			return fmt.Errorf("delete old session_events for %s: %w", file.SourcePath, err)
		}

		// Insert new search_items. Use OR IGNORE so that cross-file UUID
		// collisions (rare) are silently skipped rather than aborting the
		// transaction. Use nil (SQL NULL) when uuid is absent — SQLite's
		// UNIQUE constraint allows multiple NULLs but not multiple "".
		for _, msg := range file.Messages {
			var uuidVal interface{}
			if msg.UUID != "" {
				uuidVal = msg.UUID
			}
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO search_items
				(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			)
			if err != nil {
				return fmt.Errorf("insert search_item for %s: %w", file.SourcePath, err)
			}
		}

		// Insert session_events (one per message, with event_type='message')
		for _, msg := range file.Messages {
			_, err := tx.Exec(`
				INSERT INTO session_events
				(source, source_path, project, ordinal, timestamp, event_type, role, snippet)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`,
				file.Source,
				file.SourcePath,
				file.Project,
				msg.Ordinal,
				msg.Timestamp,
				"message",
				msg.Role,
				msg.Text,
			)
			if err != nil {
				return fmt.Errorf("insert session_event for %s: %w", file.SourcePath, err)
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
	defer rows.Close()

	var stopwords []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			continue
		}
		stopwords = append(stopwords, term)
	}

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
	defer rows.Close()

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

// SessionSourceMetadata represents metadata stored in session source_metadata JSON column.
type SessionSourceMetadata struct {
	UUID        string `json:"uuid,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	CWD         string `json:"cwd,omitempty"`
}

// SetSessionSourceMetadata sets the source_metadata JSON for a session.
// This is called during sync for sessions to store extra context.
func (d *Database) SetSessionSourceMetadata(sourcePath string, metadata SessionSourceMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Update all search_items for this source_path
	_, err = d.db.Exec(
		"UPDATE search_items SET source_metadata = ? WHERE source_path = ?",
		string(data),
		sourcePath,
	)
	if err != nil {
		return fmt.Errorf("update source_metadata: %w", err)
	}

	return nil
}
