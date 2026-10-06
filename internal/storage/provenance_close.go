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
	type appliedEvidence struct {
		uuid        string
		text        string
		contentType string
	}
	emittedEvidence := make(map[appliedEvidence]struct{}, len(messages))
	for _, message := range messages {
		if message.UUID != "" {
			emittedEvidence[appliedEvidence{
				uuid:        message.UUID,
				text:        message.Text,
				contentType: message.ContentType,
			}] = struct{}{}
		}
	}

	// SQL intentionally supplies only the established broad superset. The one
	// serialized-call recognizer remains the final boundary for every omitted
	// shape. Exclude an emitted identity only when its payload and content type
	// match the retained row, which is the same boundary used by SyncFiles when
	// applying parser evidence. A UUID replay whose payload drifted did not
	// classify the retained payload, so closure must classify that stored text.
	rows, err := tx.Query(`
		SELECT id, uuid, text, content_type
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
		var text, contentType string
		if err := rows.Scan(&id, &uuid, &text, &contentType); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan search echo candidate: %w", err)
		}
		if uuid.Valid {
			if _, applied := emittedEvidence[appliedEvidence{
				uuid:        uuid.String,
				text:        text,
				contentType: contentType,
			}]; applied {
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
