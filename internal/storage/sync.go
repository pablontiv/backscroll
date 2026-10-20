package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pablontiv/backscroll/internal/corrections"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/tagging"
	"github.com/pablontiv/backscroll/internal/templates"
)

// CurrentExtractionVersion identifies the reader-extraction logic that
// produced a row. Bump when extraction semantics change; rows keep the
// version that actually produced them (perennity: old rows are never
// silently reinterpreted).
const CurrentExtractionVersion = 3

// IndexedMessage represents a message to be indexed.
type IndexedMessage struct {
	SessionID       string
	Project         string
	ProjectEvidence string

	Ordinal     int
	Role        string
	Origin      models.MessageOrigin
	Text        string
	UUID        string
	Timestamp   string
	ContentType string
	// F0a rich capture
	ToolName          string
	ToolUseID         string // native call identity; independent of the canonical message UUID
	CommandHead       string
	IsError           *bool
	WasInterrupted    bool
	ExitCode          *int // extracted by the reader from FULL tool output, before truncation
	ExtractionVersion int
	SearchEcho        bool // proven direct search call/result, never guessed from output text
}

// IndexedFile represents a file to be synced into the database.
type IndexedFile struct {
	SourcePath           string
	Source               string // "session", "plan", "ke", "decision", etc.
	Hash                 string
	IngestionFingerprint string
	Project              string
	ProjectEvidence      string
	Messages             []IndexedMessage
	Tags                 []string // only used for sessions
	// Metadata for v14 prefilter
	FileSize  *int64  // populated by sync_helpers.go if available
	FileMtime *string // populated by sync_helpers.go if available; RFC3339 format
}

// validateSyncUUIDs checks every non-empty UUID in the full synchronization
// batch before any file is mutated. It returns the identities already retained
// by their incoming source path so perennial replays can preserve those rows
// without relying on INSERT conflict handling.
func validateSyncUUIDs(ctx context.Context, tx *sql.Tx, files []IndexedFile) (map[string]bool, error) {
	seen := make(map[string]string)
	type incomingOwnership struct{ session, project, evidence string }
	incoming := make(map[string]incomingOwnership)
	type sessionKey struct{ path, session string }
	nativeProjects := make(map[sessionKey]string)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, message := range file.Messages {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if message.SessionID != "" && message.ProjectEvidence == "native_cwd" {
				key := sessionKey{file.SourcePath, message.SessionID}
				if project, seen := nativeProjects[key]; seen {
					if project != message.Project {
						return nil, fmt.Errorf("session %q contradicts incoming native project ownership", message.SessionID)
					}
				} else {
					var conflict bool
					if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM search_items WHERE source_path=? AND session_id=? AND project_evidence='native_cwd' AND project IS NOT ?)`, file.SourcePath, message.SessionID, message.Project).Scan(&conflict); err != nil {
						return nil, err
					}
					if conflict {
						return nil, fmt.Errorf("session %q contradicts retained native project ownership", message.SessionID)
					}
					nativeProjects[key] = message.Project
				}
			}
			if message.UUID == "" {
				continue
			}
			if firstPath, duplicate := seen[message.UUID]; duplicate {
				return nil, fmt.Errorf("duplicate UUID %q in sync batch (source paths %q and %q)", message.UUID, firstPath, file.SourcePath)
			}
			seen[message.UUID] = file.SourcePath
			incoming[message.UUID] = incomingOwnership{message.SessionID, message.Project, message.ProjectEvidence}
		}
	}

	existing := make(map[string]bool, len(seen))
	for uuid, sourcePath := range seen {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var owner, session, project, evidence string
		err := tx.QueryRowContext(ctx, `SELECT source_path,COALESCE(session_id,''),COALESCE(project,''),COALESCE(project_evidence,'') FROM search_items WHERE uuid = ?`, uuid).Scan(&owner, &session, &project, &evidence)
		switch {
		case err == sql.ErrNoRows:
		case err != nil:
			return nil, fmt.Errorf("inspect UUID %q: %w", uuid, err)
		case owner != sourcePath:
			return nil, fmt.Errorf("UUID %q belongs to source_path %q, not %q", uuid, owner, sourcePath)
		default:
			m := incoming[uuid]
			if session != "" && m.session != "" && session != m.session {
				return nil, fmt.Errorf("UUID %q contradicts retained session ownership", uuid)
			}
			if m.session != "" && m.evidence == "native_cwd" && evidence == "native_cwd" && project != m.project {
				return nil, fmt.Errorf("UUID %q contradicts retained native project ownership", uuid)
			}
			existing[uuid] = true
		}

		var eventOwner string
		err = tx.QueryRowContext(ctx, `SELECT source_path FROM tool_events WHERE message_uuid = ? LIMIT 1`, uuid).Scan(&eventOwner)
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

// reconcilePerennialOrdinals reserves the entire persisted path before allocating
// any incoming identity, including unknown identities and historical shared
// search ordinals. Existing UUIDs keep their anchors; new collisions use the
// lowest free non-negative ordinal, in incoming order. The caller's slice is not
// mutated, and every downstream consumer receives the same reconciled copy.
func reconcilePerennialOrdinals(ctx context.Context, tx *sql.Tx, file IndexedFile) ([]IndexedMessage, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ordinal, COALESCE(uuid, '') FROM search_items WHERE source_path = ?
 UNION ALL SELECT ordinal, COALESCE(message_uuid, '') FROM tool_events WHERE source_path = ?`, file.SourcePath, file.SourcePath)
	if err != nil {
		return nil, err
	}
	reserved := make(map[int]struct{})
	identities := make(map[string]int)
	for rows.Next() {
		var ordinal int
		var uuid string
		if err := rows.Scan(&ordinal, &uuid); err != nil {
			rows.Close()
			return nil, err
		}
		if ordinal < 0 {
			rows.Close()
			return nil, fmt.Errorf("negative persisted ordinal %d", ordinal)
		}
		if uuid != "" {
			if previous, ok := identities[uuid]; ok && previous != ordinal {
				rows.Close()
				return nil, fmt.Errorf("UUID %q has conflicting persisted ordinals %d and %d", uuid, previous, ordinal)
			}
			identities[uuid] = ordinal
		}
		reserved[ordinal] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	messages := append([]IndexedMessage(nil), file.Messages...)
	nextFree := 0
	for i := range messages {
		msg := &messages[i]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if msg.UUID == "" {
			continue
		}
		if msg.Ordinal < 0 {
			return nil, fmt.Errorf("negative incoming ordinal %d for UUID %q", msg.Ordinal, msg.UUID)
		}
		if ordinal, ok := identities[msg.UUID]; ok {
			msg.Ordinal = ordinal
			continue
		}
		if _, occupied := reserved[msg.Ordinal]; occupied {
			for {
				if _, occupied := reserved[nextFree]; !occupied {
					break
				}
				nextFree++
			}
			msg.Ordinal = nextFree
		}
		reserved[msg.Ordinal] = struct{}{}
		identities[msg.UUID] = msg.Ordinal
	}
	return messages, nil
}

// SyncFiles syncs a batch of files into the database without cancellation.
// It is retained for compatibility with existing callers.
func (d *Database) SyncFiles(files []IndexedFile) error {
	return d.SyncFilesContext(context.Background(), files)
}

// SyncFilesContext syncs a batch of files into the database. All records and
// derived stopwords are updated in one transaction, so cancellation or any
// error leaves no partial rows or file metadata from the call.
func (d *Database) SyncFilesContext(ctx context.Context, files []IndexedFile) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	// The transaction lifetime is deliberately detached from cancellation.
	// Statements use the original ctx; syncTransactionGate alone owns the
	// synchronized rollback-versus-commit decision.
	tx, err := d.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	gate := newSyncTransactionGate(ctx, tx)
	defer func() {
		if rollbackErr := gate.rollback(); rollbackErr != nil && !errors.Is(retErr, rollbackErr) {
			wrapped := fmt.Errorf("rollback transaction: %w", rollbackErr)
			retErr = errors.Join(retErr, wrapped)
		}
		if cancelErr := gate.cancellationBeforeCommit(); cancelErr != nil && !errors.Is(retErr, cancelErr) {
			retErr = errors.Join(cancelErr, retErr)
		}
	}()

	existingUUIDs, err := validateSyncUUIDs(ctx, tx, files)
	if err != nil {
		return fmt.Errorf("validate sync UUIDs: %w", err)
	}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A non-empty session parse in which every message carries a UUID starts
		// the perennial path. Once the database has stored identity for a path,
		// that stored fact remains authoritative even when a later parse is empty
		// or mixed; transient parser output must not wipe retained history.
		isSession := file.Source == "session"
		allCurrentHaveUUIDs := true
		for _, m := range file.Messages {
			if err := ctx.Err(); err != nil {
				return err
			}
			if m.UUID == "" {
				allCurrentHaveUUIDs = false
				break
			}
		}
		completeUUIDParse := len(file.Messages) > 0 && allCurrentHaveUUIDs

		storedIdentity := storedPathIdentity{}
		if isSession {
			storedIdentity, err = inspectStoredPathIdentity(ctx, tx, file.SourcePath)
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
			if _, err := tx.ExecContext(ctx, "DELETE FROM template_matches WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old template_matches for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM search_items WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old search_items for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM tool_events WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old tool_events for %s: %w", file.SourcePath, err)
			}
		} else if legacyTransition {
			if _, err := tx.ExecContext(ctx, "DELETE FROM search_items WHERE source_path = ? AND uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy rows for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM tool_events WHERE source_path = ? AND message_uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy tool_events for %s: %w", file.SourcePath, err)
			}
		}

		if perennial {
			file.Messages, err = reconcilePerennialOrdinals(ctx, tx, file)
			if err != nil {
				return fmt.Errorf("reconcile ordinals for %s: %w", file.SourcePath, err)
			}
		}

		// Insert new search_items. UUID ownership and batch uniqueness were
		// validated before any mutation. Existing UUID rows owned by this path
		// are retained explicitly; INSERT conflict handling is not used to
		// condense invalid input. Use nil (SQL NULL) when uuid is absent —
		// SQLite's UNIQUE constraint allows multiple NULLs but not multiple "".
		// For perennial files, skip messages without UUIDs (flap guard: don't
		// introduce uuid-less rows into a perennial file).
		hasNativeSession := false
		for _, msg := range file.Messages {
			hasNativeSession = hasNativeSession || msg.SessionID != ""
			if err := ctx.Err(); err != nil {
				return err
			}
			// Skip uuid-less messages if file is perennial
			if perennial && msg.UUID == "" {
				continue
			}

			project, evidence := file.Project, file.ProjectEvidence
			if msg.SessionID != "" {
				project, evidence = msg.Project, msg.ProjectEvidence
			}
			origin := normalizedOrigin(msg.Origin)
			if err := rejectConflictingOrigin(ctx, tx, msg.UUID, origin); err != nil {
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
			var timestampVal interface{}
			if msg.Timestamp != "" {
				timestampVal = msg.Timestamp
			}
			if !perennial || msg.UUID == "" || !existingUUIDs[msg.UUID] {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO search_items
					(source, source_path, ordinal, role, origin, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted, search_echo, origin_version, project_evidence, session_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`,
					file.Source,
					file.SourcePath,
					msg.Ordinal,
					msg.Role,
					origin,
					msg.Text,
					timestampVal,
					uuidVal,
					project,
					msg.ContentType,
					msg.ExtractionVersion,
					msg.WasInterrupted,
					msg.SearchEcho,
					CurrentOriginVersion,
					projectEvidence(evidence),
					nullableSessionID(msg.SessionID),
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
				if msg.SessionID != "" {
					if _, err := tx.ExecContext(ctx, `UPDATE search_items SET session_id=? WHERE uuid=? AND session_id IS NULL`, msg.SessionID, msg.UUID); err != nil {
						return err
					}
				}
				if msg.SessionID != "" && evidence == "native_cwd" {
					if _, err := tx.ExecContext(ctx, `UPDATE search_items SET project=?,project_evidence='native_cwd' WHERE source_path=? AND uuid=? AND session_id=? AND COALESCE(project_evidence,'unknown') IN ('unknown','path_fallback')`, project, file.SourcePath, msg.UUID, msg.SessionID); err != nil {
						return fmt.Errorf("enrich native project proof for %s: %w", file.SourcePath, err)
					}
				}
				if _, err := tx.ExecContext(ctx, `UPDATE search_items SET project = ?, project_evidence = ?
     WHERE source_path = ? AND uuid = ? AND text = ? AND content_type = ?
     AND (session_id IS NULL OR session_id IS ?)
     AND COALESCE(project_evidence,'unknown') IN ('unknown','path_fallback') AND ? IN ('path_fallback','registry','native_cwd')`,
					project, projectEvidence(evidence), file.SourcePath, msg.UUID, msg.Text, msg.ContentType, nullableSessionID(msg.SessionID), projectEvidence(evidence)); err != nil {
					return fmt.Errorf("enrich project proof for %s: %w", file.SourcePath, err)
				}
				if _, err := tx.ExecContext(ctx, `UPDATE search_items SET search_echo = ?
					WHERE source_path = ? AND uuid = ? AND text = ? AND content_type = ?
					AND (search_echo IS NULL OR (search_echo = 0 AND ? = 1))`,
					msg.SearchEcho, file.SourcePath, msg.UUID, msg.Text, msg.ContentType, msg.SearchEcho); err != nil {
					return fmt.Errorf("update search echo provenance for %s: %w", file.SourcePath, err)
				}
				// Interruption is positive evidence: later partial parses cannot erase it.
				if _, err := tx.ExecContext(ctx, `UPDATE search_items
					SET was_interrupted = 1
					WHERE source_path = ? AND uuid = ? AND ? = 1
					AND COALESCE(was_interrupted, 0) = 0`,
					file.SourcePath, msg.UUID, msg.WasInterrupted); err != nil {
					return fmt.Errorf("enrich interruption proof for %s: %w", file.SourcePath, err)
				}
				if err := enrichOrigin(ctx, tx, msg.UUID, origin); err != nil {
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
				var eventUUID sql.NullString
				eventErr := tx.QueryRowContext(ctx, `SELECT message_uuid FROM tool_events WHERE source_path = ? AND ordinal = ?`, file.SourcePath, msg.Ordinal).Scan(&eventUUID)
				if eventErr != nil && eventErr != sql.ErrNoRows {
					return fmt.Errorf("inspect tool_event ordinal for %s: %w", file.SourcePath, eventErr)
				}
				if eventErr == nil && eventUUID.String != "" && eventUUID.String != msg.UUID {
					return fmt.Errorf("tool_event ordinal %d for %s belongs to another identity", msg.Ordinal, file.SourcePath)
				}
				if eventErr == sql.ErrNoRows {
					if _, err := tx.ExecContext(ctx, `
					INSERT INTO tool_events
					(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version, native_call_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
				`, uuidVal, file.SourcePath, msg.Ordinal, msg.ToolName, msg.CommandHead, isErrVal, exitCodeVal, msg.ExtractionVersion, msg.ToolUseID); err != nil {
						return fmt.Errorf("insert tool_event for %s: %w", file.SourcePath, err)
					}
				}
				// Lossy backfill deliberately leaves identity unknown. A retained
				// search anchor proves which incoming call can claim this coordinate.
				// Fill missing metadata only: false, zero, and known names/heads
				// remain evidence, and extraction epochs can only advance.
				if eventErr == nil && eventUUID.String == "" && msg.UUID != "" {
					if _, err := tx.ExecContext(ctx, `UPDATE tool_events
                        SET message_uuid = ?,
                            tool_name = COALESCE(NULLIF(tool_name, ''), ?),
                            command_head = COALESCE(NULLIF(command_head, ''), ?),
                            is_error = COALESCE(is_error, ?),
                            exit_code = COALESCE(exit_code, ?),
                            extraction_version = MAX(COALESCE(extraction_version, 0), ?)
                        WHERE source_path = ? AND ordinal = ?
                          AND COALESCE(message_uuid, '') = ''`,
						msg.UUID, msg.ToolName, msg.CommandHead, isErrVal, exitCodeVal,
						msg.ExtractionVersion, file.SourcePath, msg.Ordinal); err != nil {
						return fmt.Errorf("claim lossy tool_event for %s: %w", file.SourcePath, err)
					}
				}
				// Native call identity is independent of a search row UUID. Fill
				// missing proof, and reject contradictory identities transactionally.
				if eventErr == nil && msg.ToolUseID != "" {
					var nativeID sql.NullString
					if err := tx.QueryRowContext(ctx, `SELECT native_call_id FROM tool_events WHERE source_path = ? AND ordinal = ?`, file.SourcePath, msg.Ordinal).Scan(&nativeID); err != nil {
						return fmt.Errorf("inspect native call identity for %s: %w", file.SourcePath, err)
					}
					if nativeID.Valid && nativeID.String != "" && nativeID.String != msg.ToolUseID {
						return fmt.Errorf("conflicting native call identity for %s ordinal %d", file.SourcePath, msg.Ordinal)
					}
					if _, err := tx.ExecContext(ctx, `UPDATE tool_events SET native_call_id = ? WHERE source_path = ? AND ordinal = ? AND NULLIF(native_call_id, '') IS NULL`, msg.ToolUseID, file.SourcePath, msg.Ordinal); err != nil {
						return fmt.Errorf("enrich native call identity for %s: %w", file.SourcePath, err)
					}
				}
				// A stable call may precede its result. Fill only unknown signals;
				// false and zero are established proof, not missing values.
				if perennial && msg.UUID != "" {
					if _, err := tx.ExecContext(ctx, `UPDATE tool_events
						SET is_error = COALESCE(is_error, ?), exit_code = COALESCE(exit_code, ?)
						WHERE source_path = ? AND message_uuid = ? AND tool_name = ?`,
						isErrVal, exitCodeVal, file.SourcePath, msg.UUID, msg.ToolName); err != nil {
						return fmt.Errorf("enrich tool result proof for %s: %w", file.SourcePath, err)
					}
				}
			}
		}

		// Close omitted-row provenance in the same transaction as every other
		// sync mutation. Closure advances epochs and resolves echo unknowns, but
		// never rewrites actor proof, payload, identity, or positive echo evidence.
		if perennial {
			if err := closePerennialProvenance(ctx, tx, file.SourcePath, file.Messages); err != nil {
				return fmt.Errorf("close provenance backlog for %s: %w", file.SourcePath, err)
			}
		}

		// Native cwd proves ownership for every retained record in the same logical session.
		if hasNativeSession {
			if _, err := tx.ExecContext(ctx, `UPDATE search_items AS si
	 SET project=(SELECT proof.project FROM search_items proof WHERE proof.source_path=si.source_path AND proof.session_id=si.session_id AND proof.project_evidence='native_cwd' LIMIT 1), project_evidence='native_cwd'
	 WHERE si.source_path=? AND si.session_id IS NOT NULL AND COALESCE(si.project_evidence,'unknown') IN ('unknown','path_fallback')
	 AND EXISTS(SELECT 1 FROM search_items proof WHERE proof.source_path=si.source_path AND proof.session_id=si.session_id AND proof.project_evidence='native_cwd')`, file.SourcePath); err != nil {
				return fmt.Errorf("enrich retained native project proof for %s: %w", file.SourcePath, err)
			}
		}

		// Native tags use the entire retained canonical corpus, including expired siblings.
		if err := rebuildNativeSessionTags(ctx, tx, file.SourcePath); err != nil {
			return fmt.Errorf("rebuild identity tags for %s: %w", file.SourcePath, err)
		}

		// Tags are derived only for session sources. A source-semantic replay
		// clears any prior session classification before applying current tags.
		if _, err := tx.ExecContext(ctx, "DELETE FROM session_tags WHERE source_path = ?", file.SourcePath); err != nil {
			return fmt.Errorf("delete old session_tags for %s: %w", file.SourcePath, err)
		}
		if file.Source == "session" {
			for _, tag := range file.Tags {
				if err := ctx.Err(); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, `
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

		// Insert or replace in indexed_files.
		_, err = tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO indexed_files
				(path, hash, last_indexed, file_size, file_mtime, ingestion_fingerprint)
			VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?, ?)
		`,
			file.SourcePath,
			file.Hash,
			file.FileSize,
			file.FileMtime,
			file.IngestionFingerprint,
		)
		if err != nil {
			return fmt.Errorf("upsert indexed_files for %s: %w", file.SourcePath, err)
		}

		// Mine templates from error-bearing tool outputs (inside same tx).
		miner := templates.NewMiner()
		if err := d.mineTemplatesForFile(ctx, tx, file, miner); err != nil {
			return fmt.Errorf("mine templates for %s: %w", file.SourcePath, err)
		}

		// F3: Run detectors and write correction_signals
		if file.Source == "session" && !perennial {
			// Delete old corrections for non-perennial files
			if _, err := tx.ExecContext(ctx, "DELETE FROM correction_signals WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old corrections for %s: %w", file.SourcePath, err)
			}
		}

		// A successful parser pass closes pending historical evidence without
		// inventing proof for retained identities no longer present in the source.
		if file.ProjectEvidence != "" {
			if _, err := tx.ExecContext(ctx, "UPDATE search_items SET project_evidence='unknown' WHERE source_path=? AND project_evidence IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("close project evidence for %s: %w", file.SourcePath, err)
			}
		}

		// Drop this file's signals from superseded detector epochs before re-detecting.
		// correction_signals is append-only with INSERT OR IGNORE, so without this a
		// false positive recorded under older detector rules would survive forever —
		// a detector fix would stop producing NEW bad candidates while the old ones
		// kept topping the census. Signals at the current version are left alone, so
		// re-syncing an up-to-date file is still a no-op.
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM correction_signals
			WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
		`, file.SourcePath, CurrentExtractionVersion); err != nil {
			return fmt.Errorf("clear superseded correction_signals for %s: %w", file.SourcePath, err)
		}

		// Run detectors with prose-only filter: lexicon, rephrase, denial on
		// content_type='text'|'code' + role='user'; interrupt on all user messages.
		detections, err := detectSessionCorrections(ctx, file.Messages)
		if err != nil {
			return fmt.Errorf("detect corrections for %s: %w", file.SourcePath, err)
		}
		for ordinal, dets := range detections {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, det := range dets {
				if err := ctx.Err(); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO correction_signals
					(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
					VALUES (?, ?, ?, ?, ?, ?)
				`, file.Messages[ordinal].UUID, file.SourcePath, file.Messages[ordinal].Ordinal, det.DetectorName, det.Confidence, CurrentExtractionVersion)
				if err != nil {
					return fmt.Errorf("insert correction_signal for %s: %w", file.SourcePath, err)
				}
			}
		}
	}

	// Refresh dynamic stopwords before commit so a refresh failure or
	// cancellation rolls back the complete synchronization batch.
	if err := refreshStopwordsContext(ctx, tx); err != nil {
		return fmt.Errorf("refresh stopwords: %w", err)
	}
	if err := gate.commit(); err != nil {
		return err
	}

	return nil
}

// refreshStopwords updates the dynamic_stopwords table with frequently occurring terms.
// It is retained for focused callers; SyncFilesContext uses the same helper in its
// existing transaction so the refresh and synchronized records commit atomically.
func (d *Database) refreshStopwords() error {
	ctx := context.Background()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin stopword transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := refreshStopwordsContext(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stopword transaction: %w", err)
	}
	return nil
}

// refreshStopwordsContext loads the top 1000 terms from the FTS5 vocabulary
// and replaces dynamic_stopwords inside tx.
func refreshStopwordsContext(ctx context.Context, tx *sql.Tx) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM dynamic_stopwords"); err != nil {
		return fmt.Errorf("clear stopwords: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT term FROM messages_vocab
		ORDER BY doc DESC
		LIMIT 1000
	`)
	if err != nil {
		// Preserve the legacy behavior for an unavailable vocabulary, but never
		// turn cancellation into success.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return nil
	}
	defer func() { _ = rows.Close() }()

	var stopwords []string
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var term string
		if err := rows.Scan(&term); err != nil {
			continue
		}
		stopwords = append(stopwords, term)
	}
	if err := rows.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// Preserve legacy refresh behavior for non-cancellation row errors.
	}
	_ = rows.Close()

	for _, term := range stopwords {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO dynamic_stopwords (term) VALUES (?)", term); err != nil {
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
	Path                 string
	Hash                 string
	Size                 *int64  // NULL for pre-v14 rows
	Mtime                *string // NULL for pre-v14 rows
	LastIndexed          *string // timestamp of the indexing, used for racy-clean detection
	IngestionFingerprint *string // NULL for pre-v18 rows and recovery-pending sources
}

// GetFileMetadata returns all indexed file metadata, including the persisted
// ingestion-semantics fingerprint. Older rows retain NULL fields.
func (d *Database) GetFileMetadata() (map[string]FileMetadata, error) {
	return d.GetFileMetadataContext(context.Background())
}

// GetFileMetadataContext is GetFileMetadata with cancellation propagated through
// the query and row iteration.
func (d *Database) GetFileMetadataContext(ctx context.Context) (map[string]FileMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT path, hash, file_size, file_mtime, last_indexed, ingestion_fingerprint
		FROM indexed_files
		WHERE hash <> ?
	`, recoveredSourceHash)
	if err != nil {
		return nil, fmt.Errorf("query file metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()

	metadata := make(map[string]FileMetadata)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var path, hash string
		var size *int64
		var mtime *string
		var lastIndexed *string
		var ingestionFingerprint *string
		if err := rows.Scan(&path, &hash, &size, &mtime, &lastIndexed, &ingestionFingerprint); err != nil {
			return nil, fmt.Errorf("scan file metadata: %w", err)
		}
		metadata[path] = FileMetadata{
			Path:                 path,
			Hash:                 hash,
			Size:                 size,
			Mtime:                mtime,
			LastIndexed:          lastIndexed,
			IngestionFingerprint: ingestionFingerprint,
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate file metadata: %w", err)
	}

	return metadata, nil
}

// Empty evidence represents historical/unspecified attribution, never a guess
// from the current label.
func projectEvidence(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func nullableSessionID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// rebuildNativeSessionTags derives eligibility only from proven native identities.
// Close the corpus cursor before writes so failures roll back the entire sync.
func rebuildNativeSessionTags(ctx context.Context, tx *sql.Tx, path string) error {
	rows, err := tx.QueryContext(ctx, `SELECT session_id,text FROM search_items
 WHERE source_path=? AND source='session' AND session_id IS NOT NULL AND session_id<>''`, path)
	if err != nil {
		return err
	}
	accumulators := make(map[string]*tagging.Accumulator)
	for rows.Next() {
		var session, text string
		if err := rows.Scan(&session, &text); err != nil {
			rows.Close()
			return err
		}
		a := accumulators[session]
		if a == nil {
			a = &tagging.Accumulator{}
			accumulators[session] = a
		}
		a.Add(text)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_identity_tags WHERE source_path=?", path); err != nil {
		return err
	}
	for session, a := range accumulators {
		for _, tag := range a.Tags() {
			if _, err := tx.ExecContext(ctx, `INSERT INTO session_identity_tags(source_path,session_id,tag) VALUES(?,?,?)`, path, session, tag); err != nil {
				return err
			}
		}
	}
	return nil
}

// detectSessionCorrections preserves physical indices while isolating logical conversations.
func detectSessionCorrections(ctx context.Context, messages []IndexedMessage) (map[int][]corrections.Detection, error) {
	// Detectors see logical sessions, not interleaved siblings in a database.
	detectionMsgs := make(map[string][]models.Message)
	detectionIndices := make(map[string][]int)
	for i, im := range messages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		detectionIndices[im.SessionID] = append(detectionIndices[im.SessionID], i)
		detectionMsgs[im.SessionID] = append(detectionMsgs[im.SessionID], models.Message{
			Role:           im.Role,
			Origin:         normalizedOrigin(im.Origin),
			Content:        im.Text,
			ContentType:    im.ContentType,
			Timestamp:      time.Time{}, // not needed for detection
			UUID:           im.UUID,
			ToolName:       im.ToolName,
			IsError:        im.IsError,
			WasInterrupted: im.WasInterrupted,
		})
	}
	detections := make(map[int][]corrections.Detection)
	for session, groupMessages := range detectionMsgs {
		group, err := corrections.RunDetectorsFilteredContext(ctx, groupMessages)
		if err != nil {
			return nil, err
		}
		for index, detected := range group {
			detections[detectionIndices[session][index]] = detected
		}
	}
	return detections, nil
}
