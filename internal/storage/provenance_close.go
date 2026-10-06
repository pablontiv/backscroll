package storage

import (
	"database/sql"
	"fmt"

	"github.com/pablontiv/backscroll/internal/directsearch"
)

type storedPathIdentity struct {
	totalRows int
	uuidRows  int
}

func inspectStoredPathIdentity(tx *sql.Tx, sourcePath string) (storedPathIdentity, error) {
	var state storedPathIdentity
	if err := tx.QueryRow(`
		SELECT COUNT(*), COUNT(uuid)
		FROM search_items
		WHERE source_path = ?
	`, sourcePath).Scan(&state.totalRows, &state.uuidRows); err != nil {
		return storedPathIdentity{}, err
	}
	return state, nil
}

func (s storedPathIdentity) identified() bool {
	return s.uuidRows > 0
}

func (s storedPathIdentity) pureLegacy() bool {
	return s.totalRows > 0 && s.uuidRows == 0
}

// uuidReplacementsAvailable reports whether a complete UUID parse can be
// installed for sourcePath. Every emitted identity is preflighted before legacy
// history is removed: duplicate UUIDs in the parse are invalid, and a UUID
// already owned by another path would be ignored by the perennial INSERT.
func uuidReplacementsAvailable(tx *sql.Tx, sourcePath string, messages []IndexedMessage) (bool, error) {
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message.UUID == "" {
			continue
		}
		if _, duplicate := seen[message.UUID]; duplicate {
			return false, fmt.Errorf("duplicate UUID %q in complete parse", message.UUID)
		}
		seen[message.UUID] = struct{}{}
	}

	for uuid := range seen {
		var conflicting int
		if err := tx.QueryRow(`
			SELECT COUNT(*)
			FROM search_items
			WHERE uuid = ? AND source_path <> ?
		`, uuid, sourcePath).Scan(&conflicting); err != nil {
			return false, err
		}
		if conflicting != 0 {
			return false, nil
		}
	}
	return true, nil
}

// closePerennialProvenance closes replay backlogs without replacing perennial
// identity or payload. It runs inside SyncFiles' transaction so a later
// conflict or write error rolls every closure back with the rest of the batch.
func closePerennialProvenance(tx *sql.Tx, sourcePath string, messages []IndexedMessage) error {
	if err := closeSearchEchoProvenance(tx, sourcePath, messages); err != nil {
		return err
	}

	// Origin is monotonic evidence. An omitted row may be advanced to the
	// current parser epoch, but known actor proof must never be rewritten.
	if _, err := tx.Exec(`
		UPDATE search_items
		SET origin_version = ?
		WHERE source_path = ?
		  AND (origin_version IS NULL OR origin_version < ?)
	`, CurrentOriginVersion, sourcePath, CurrentOriginVersion); err != nil {
		return fmt.Errorf("close origin version: %w", err)
	}
	return nil
}

func closeSearchEchoProvenance(tx *sql.Tx, sourcePath string, messages []IndexedMessage) error {
	emittedUUIDs := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message.UUID != "" {
			emittedUUIDs[message.UUID] = struct{}{}
		}
	}

	// SQL intentionally supplies only the established broad superset. The one
	// serialized-call recognizer remains the final boundary for every omitted
	// shape. Emitted identities retain the parser's current evidence and can
	// remain queued when that evidence is still zero.
	rows, err := tx.Query(`
		SELECT id, uuid, text
		FROM search_items
		WHERE source_path = ?
		  AND `+searchEchoZeroPrefilterSQL("search_items"), sourcePath)
	if err != nil {
		return fmt.Errorf("query search echo candidates: %w", err)
	}

	var directCallIDs []int64
	for rows.Next() {
		var id int64
		var uuid sql.NullString
		var text string
		if err := rows.Scan(&id, &uuid, &text); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan search echo candidate: %w", err)
		}
		if uuid.Valid {
			if _, emitted := emittedUUIDs[uuid.String]; emitted {
				continue
			}
		}
		if directsearch.IsSerializedDirectSearchCall(text) {
			directCallIDs = append(directCallIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate search echo candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close search echo candidates: %w", err)
	}

	for _, id := range directCallIDs {
		if _, err := tx.Exec(`
			UPDATE search_items
			SET search_echo = 1
			WHERE id = ? AND COALESCE(search_echo, 0) = 0
		`, id); err != nil {
			return fmt.Errorf("promote direct search call %d: %w", id, err)
		}
	}

	// Positive evidence is never touched. Once recognizable candidates have
	// been promoted, every remaining unknown row can leave the replay queue.
	if _, err := tx.Exec(`
		UPDATE search_items
		SET search_echo = 0
		WHERE source_path = ? AND search_echo IS NULL
	`, sourcePath); err != nil {
		return fmt.Errorf("close unknown search echo: %w", err)
	}
	return nil
}
