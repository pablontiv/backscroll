package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/pablontiv/backscroll/internal/corrections"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/templates"
)

// CurrentExtractionVersion identifies the reader-extraction logic that
// produced a row. Bump when extraction semantics change; rows keep the
// version that actually produced them (perennity: old rows are never
// silently reinterpreted).
const CurrentExtractionVersion = 3

// IndexedMessage represents a message to be indexed.
type IndexedMessage struct {
	Ordinal     int
	Role        string
	Origin      models.MessageOrigin
	Text        string
	UUID        string
	Timestamp   string
	ContentType string
	// F0a rich capture
	ToolName          string
	CommandHead       string
	IsError           *bool
	WasInterrupted    bool
	ExitCode          *int // extracted by the reader from FULL tool output, before truncation
	ExtractionVersion int
	SearchEcho        bool // proven direct search call/result, never guessed from output text
}

// IndexedFile represents a file to be synced into the database.
type IndexedFile struct {
	SourcePath string
	Source     string // "session", "plan", "ke", "decision", etc.
	Hash       string
	Project    string
	Messages   []IndexedMessage
	Tags       []string // only used for sessions
	// Metadata for v14 prefilter
	FileSize  *int64  // populated by sync_helpers.go if available
	FileMtime *string // populated by sync_helpers.go if available; RFC3339 format
}

// validateSyncUUIDs checks every non-empty UUID in the full synchronization
// batch before any file is mutated. It returns the identities already retained
// by their incoming source path so perennial replays can preserve those rows
// without relying on INSERT conflict handling.
func validateSyncUUIDs(tx *sql.Tx, files []IndexedFile) (map[string]bool, error) {
	seen := make(map[string]string)
	for _, file := range files {
		for _, message := range file.Messages {
			if message.UUID == "" {
				continue
			}
			if firstPath, duplicate := seen[message.UUID]; duplicate {
				return nil, fmt.Errorf("duplicate UUID %q in sync batch (source paths %q and %q)", message.UUID, firstPath, file.SourcePath)
			}
			seen[message.UUID] = file.SourcePath
		}
	}

	existing := make(map[string]bool, len(seen))
	for uuid, sourcePath := range seen {
		var owner string
		err := tx.QueryRow(`SELECT source_path FROM search_items WHERE uuid = ?`, uuid).Scan(&owner)
		switch {
		case err == sql.ErrNoRows:
		case err != nil:
			return nil, fmt.Errorf("inspect UUID %q: %w", uuid, err)
		case owner != sourcePath:
			return nil, fmt.Errorf("UUID %q belongs to source_path %q, not %q", uuid, owner, sourcePath)
		default:
			existing[uuid] = true
		}

		var eventOwner string
		err = tx.QueryRow(`SELECT source_path FROM tool_events WHERE message_uuid = ? LIMIT 1`, uuid).Scan(&eventOwner)
		switch {
		case err == sql.ErrNoRows:
		case err != nil:
			return nil, fmt.Errorf("inspect tool event UUID %q: %w", uuid, err)
		case eventOwner != sourcePath:
			return nil, fmt.Errorf("UUID %q belongs to tool_event source_path %q, not %q", uuid, eventOwner, sourcePath)
		}
	}
	return existing, nil
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

	existingUUIDs, err := validateSyncUUIDs(tx, files)
	if err != nil {
		return fmt.Errorf("validate sync UUIDs: %w", err)
	}

	for _, file := range files {
		// A non-empty session parse in which every message carries a UUID starts
		// the perennial path. Once the database has stored identity for a path,
		// that stored fact remains authoritative even when a later parse is empty
		// or mixed; transient parser output must not wipe retained history.
		isSession := file.Source == "session"
		allCurrentHaveUUIDs := true
		for _, m := range file.Messages {
			if m.UUID == "" {
				allCurrentHaveUUIDs = false
				break
			}
		}
		completeUUIDParse := len(file.Messages) > 0 && allCurrentHaveUUIDs

		storedIdentity := storedPathIdentity{}
		if isSession {
			storedIdentity, err = inspectStoredPathIdentity(tx, file.SourcePath)
			if err != nil {
				return fmt.Errorf("check perennial status for %s: %w", file.SourcePath, err)
			}
		}
		perennial := isSession && (completeUUIDParse || storedIdentity.identified())

		// UUID-less legacy history is deleted only for the one-way transition
		// from a purely legacy path to a complete UUID parse. The batch-wide
		// preflight above guarantees that every replacement can be installed.
		// Empty/mixed replays of an identified path retain every UUID-less search
		// row and tool event.
		legacyTransition := perennial && storedIdentity.pureLegacy() && completeUUIDParse

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
		} else if legacyTransition {
			if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ? AND uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy rows for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ? AND message_uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy tool_events for %s: %w", file.SourcePath, err)
			}
		}

		// Insert new search_items. UUID ownership and batch uniqueness were
		// validated before any mutation. Existing UUID rows owned by this path
		// are retained explicitly; INSERT conflict handling is not used to
		// condense invalid input. Use nil (SQL NULL) when uuid is absent —
		// SQLite's UNIQUE constraint allows multiple NULLs but not multiple "".
		// For perennial files, skip messages without UUIDs (flap guard: don't
		// introduce uuid-less rows into a perennial file).
		for _, msg := range file.Messages {
			// Skip uuid-less messages if file is perennial
			if perennial && msg.UUID == "" {
				continue
			}

			origin := normalizedOrigin(msg.Origin)
			if err := rejectConflictingOrigin(tx, msg.UUID, origin); err != nil {
				return fmt.Errorf("validate message origin for %s: %w", file.SourcePath, err)
			}

			var uuidVal interface{}
			if msg.UUID != "" {
				uuidVal = msg.UUID
			}
			var isErrVal interface{}
			if msg.IsError != nil {
				isErrVal = *msg.IsError
			}
			if !perennial || msg.UUID == "" || !existingUUIDs[msg.UUID] {
				_, err := tx.Exec(`
					INSERT INTO search_items
					(source, source_path, ordinal, role, origin, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted, search_echo, origin_version)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`,
					file.Source,
					file.SourcePath,
					msg.Ordinal,
					msg.Role,
					origin,
					msg.Text,
					msg.Timestamp,
					uuidVal,
					file.Project,
					msg.ContentType,
					msg.ExtractionVersion,
					msg.WasInterrupted,
					msg.SearchEcho,
					CurrentOriginVersion,
				)
				if err != nil {
					return fmt.Errorf("insert search_item for %s: %w", file.SourcePath, err)
				}
			}

			// Re-parsing a perennial row must enrich provenance without replacing
			// its ID, original text, or extraction epoch. NULL is migration backlog;
			// a later paired result may also supply positive evidence. Never erase
			// previously proven linkage when a partial source no longer has the call.
			if perennial {
				if _, err := tx.Exec(`UPDATE search_items SET search_echo = ?
					WHERE source_path = ? AND uuid = ? AND text = ? AND content_type = ?
					AND (search_echo IS NULL OR (search_echo = 0 AND ? = 1))`,
					msg.SearchEcho, file.SourcePath, msg.UUID, msg.Text, msg.ContentType, msg.SearchEcho); err != nil {
					return fmt.Errorf("update search echo provenance for %s: %w", file.SourcePath, err)
				}
				if err := enrichOrigin(tx, msg.UUID, origin); err != nil {
					return fmt.Errorf("update message origin provenance for %s: %w", file.SourcePath, err)
				}
			}

			if msg.ToolName != "" {
				// Q2: use the code the reader extracted from the untruncated output.
				// Re-extracting from msg.Text here would miss any code beyond the
				// toolfmt truncation cap — the exact bug Q2 exists to fix.
				exitCode := msg.ExitCode
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

		// Close omitted-row provenance in the same transaction as every other
		// sync mutation. Closure advances epochs and resolves echo unknowns, but
		// never rewrites actor proof, payload, identity, or positive echo evidence.
		if perennial {
			if err := closePerennialProvenance(tx, file.SourcePath, file.Messages); err != nil {
				return fmt.Errorf("close provenance backlog for %s: %w", file.SourcePath, err)
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
			INSERT OR REPLACE INTO indexed_files (path, hash, last_indexed, file_size, file_mtime)
			VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?)
		`,
			file.SourcePath,
			file.Hash,
			file.FileSize,
			file.FileMtime,
		)
		if err != nil {
			return fmt.Errorf("upsert indexed_files for %s: %w", file.SourcePath, err)
		}

		// Mine templates from error-bearing tool outputs (inside same tx).
		miner := templates.NewMiner()
		if err := d.mineTemplatesForFile(tx, file, miner); err != nil {
			return fmt.Errorf("mine templates for %s: %w", file.SourcePath, err)
		}

		// F3: Run detectors and write correction_signals
		if file.Source == "session" && !perennial {
			// Delete old corrections for non-perennial files
			if _, err := tx.Exec("DELETE FROM correction_signals WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old corrections for %s: %w", file.SourcePath, err)
			}
		}

		// Convert IndexedMessage to models.Message for detector input
		detectionMsgs := make([]models.Message, len(file.Messages))
		for i, im := range file.Messages {
			detectionMsgs[i] = models.Message{
				Role:           im.Role,
				Origin:         normalizedOrigin(im.Origin),
				Content:        im.Text,
				ContentType:    im.ContentType,
				Timestamp:      time.Time{}, // not needed for detection
				UUID:           im.UUID,
				ToolName:       im.ToolName,
				IsError:        im.IsError,
				WasInterrupted: im.WasInterrupted,
			}
		}

		// Drop this file's signals from superseded detector epochs before re-detecting.
		// correction_signals is append-only with INSERT OR IGNORE, so without this a
		// false positive recorded under older detector rules would survive forever —
		// a detector fix would stop producing NEW bad candidates while the old ones
		// kept topping the census. Signals at the current version are left alone, so
		// re-syncing an up-to-date file is still a no-op.
		if _, err := tx.Exec(`
			DELETE FROM correction_signals
			WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
		`, file.SourcePath, CurrentExtractionVersion); err != nil {
			return fmt.Errorf("clear superseded correction_signals for %s: %w", file.SourcePath, err)
		}

		// Run detectors with prose-only filter: lexicon, rephrase, denial on
		// content_type='text'|'code' + role='user'; interrupt on all user messages.
		detections := corrections.RunDetectorsFiltered(detectionMsgs)
		for ordinal, dets := range detections {
			for _, det := range dets {
				_, err := tx.Exec(`
					INSERT OR IGNORE INTO correction_signals
					(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
					VALUES (?, ?, ?, ?, ?, ?)
				`, file.Messages[ordinal].UUID, file.SourcePath, ordinal, det.DetectorName, det.Confidence, CurrentExtractionVersion)
				if err != nil {
					return fmt.Errorf("insert correction_signal for %s: %w", file.SourcePath, err)
				}
			}
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
	rows, err := d.db.Query(`
		SELECT path, hash
		FROM indexed_files
		WHERE hash <> ?
	`, recoveredSourceHash)
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

// FileMetadata represents the persisted metadata for a file.
type FileMetadata struct {
	Path        string
	Hash        string
	Size        *int64  // NULL for pre-v14 rows
	Mtime       *string // NULL for pre-v14 rows
	LastIndexed *string // timestamp of the indexing, used for racy-clean detection
}

// GetFileMetadata returns all indexed file metadata (path, hash, size, mtime, last_indexed).
// Pre-v14 rows have NULL size and mtime. LastIndexed is populated from indexed_files.last_indexed.
func (d *Database) GetFileMetadata() (map[string]FileMetadata, error) {
	rows, err := d.db.Query(`
		SELECT path, hash, file_size, file_mtime, last_indexed
		FROM indexed_files
		WHERE hash <> ?
	`, recoveredSourceHash)
	if err != nil {
		return nil, fmt.Errorf("query file metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()

	metadata := make(map[string]FileMetadata)
	for rows.Next() {
		var path, hash string
		var size *int64
		var mtime *string
		var lastIndexed *string
		if err := rows.Scan(&path, &hash, &size, &mtime, &lastIndexed); err != nil {
			return nil, fmt.Errorf("scan file metadata: %w", err)
		}
		metadata[path] = FileMetadata{
			Path:        path,
			Hash:        hash,
			Size:        size,
			Mtime:       mtime,
			LastIndexed: lastIndexed,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file metadata: %w", err)
	}

	return metadata, nil
}
