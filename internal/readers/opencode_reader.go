package readers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	_ "modernc.org/sqlite"
)

// OpenCodeReader implements SessionReader for OpenCode SQLite databases.
// OpenCode stores sessions in <project_dir>/.opencode/opencode.db.
type OpenCodeReader struct{}

func (r *OpenCodeReader) Name() string { return "opencode" }

// Discover returns paths to OpenCode database files matching the definition's discover config.
func (r *OpenCodeReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFilesContext(ctx, def.Discover)
}

// Hash returns a SHA-256 token of native message and part rows in one read
// snapshot, including committed WAL data (not the database file's bytes).
func (r *OpenCodeReader) Hash(ctx context.Context, dbPath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	db, err := openReadOnly(dbPath)
	if err != nil {
		return "", fmt.Errorf("opencode hash open %s: %w", dbPath, err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	token, err := openCodeSnapshotHash(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("opencode hash %s: %w", dbPath, err)
	}
	return token, nil
}

// Hash raw columns before JSON decoding or output truncation. Length framing
// distinguishes row/column boundaries; RawBytes avoids copying payloads merely
// to hash them. Both table scans share the transaction's SQLite snapshot.
func openCodeSnapshotHash(ctx context.Context, tx *sql.Tx) (string, error) {
	h := sha256.New()
	var size [8]byte
	for _, query := range []string{
		`SELECT id, session_id, time_created, time_updated, data FROM message ORDER BY id`,
		`SELECT id, message_id, session_id, time_created, time_updated, data FROM part ORDER BY id`,
	} {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return "", fmt.Errorf("opencode hash query: %w", err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		values := make([]sql.RawBytes, len(columns))
		dest := make([]any, len(values))
		for i := range values {
			dest[i] = &values[i]
		}
		h.Write([]byte{byte(len(columns))})
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				rows.Close()
				return "", err
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return "", err
			}
			h.Write([]byte{1})
			for _, value := range values {
				if value == nil {
					h.Write([]byte{0})
					continue
				}
				h.Write([]byte{1})
				binary.BigEndian.PutUint64(size[:], uint64(len(value)))
				h.Write(size[:])
				h.Write(value)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type msgInfoData struct {
	Role string `json:"role"`
}

type partInfoData struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Ignored *bool          `json:"ignored"`
	Tool    string         `json:"tool"`
	State   *toolPartState `json:"state"`
}

// toolPartState is the `state` object of an OpenCode `tool` part. A single tool
// part carries both the input (an object) and the output (a string).
type toolPartState struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

// Parse reads all messages from the OpenCode database and returns them as a ParsedFile.
// Parts of type "text" (with ignored != true) and "tool" (with non-empty state.input/state.output) are indexed; all other part types are skipped.
func (r *OpenCodeReader) Parse(ctx context.Context, dbPath string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	db, err := openReadOnly(dbPath)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse open %s: %w", dbPath, err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return models.ParsedFile{}, err
	}
	defer tx.Rollback()
	return parseOpenCodeSnapshot(ctx, tx, dbPath)
}

func parseOpenCodeSnapshot(ctx context.Context, tx *sql.Tx, dbPath string) (models.ParsedFile, error) {
	hash, err := openCodeSnapshotHash(ctx, tx)
	if err != nil {
		return models.ParsedFile{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT m.id, m.session_id, m.data, m.time_created, p.data
		FROM message m JOIN part p ON p.message_id = m.id
		ORDER BY m.time_created ASC, m.id ASC, p.id ASC
	`)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse query %s: %w", dbPath, err)
	}
	defer rows.Close()

	var (
		msgs         []models.Message
		currentMsgID string
		currentRole  string
		currentTime  int64
		textParts    []string
		toolMsgs     []models.Message
	)

	flush := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(textParts) > 0 {
			msgs = append(msgs, models.Message{
				Role:        normalizeOpenCodeRole(currentRole),
				Origin:      openCodeOriginForRole(currentRole),
				Content:     strings.Join(textParts, "\n"),
				ContentType: "text",
				Timestamp:   time.UnixMilli(currentTime),
			})
		}
		msgs = append(msgs, toolMsgs...)
		textParts = nil
		toolMsgs = nil
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}
		if !rows.Next() {
			break
		}
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}
		var (
			msgID       string
			sessionID   string
			msgData     sql.RawBytes
			timeCreated int64
			partData    sql.RawBytes
		)
		if err := rows.Scan(&msgID, &sessionID, &msgData, &timeCreated, &partData); err != nil {
			return models.ParsedFile{}, err
		}
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}

		var pd partInfoData
		if err := json.Unmarshal(partData, &pd); err != nil {
			continue
		}

		if msgID != currentMsgID {
			if err := flush(); err != nil {
				return models.ParsedFile{}, err
			}
			currentMsgID = msgID
			var md msgInfoData
			if err := json.Unmarshal(msgData, &md); err == nil {
				currentRole = md.Role
			} else {
				currentRole = ""
			}
			currentTime = timeCreated
		}

		switch pd.Type {
		case "text":
			if pd.Ignored != nil && *pd.Ignored {
				continue
			}
			if text := strings.TrimSpace(pd.Text); text != "" {
				textParts = append(textParts, text)
			}
		case "tool":
			if pd.State == nil {
				continue
			}
			role := normalizeOpenCodeRole(currentRole)
			origin := openCodeOriginForRole(currentRole)
			ts := time.UnixMilli(currentTime)
			// One part carries both the call and its output, so a direct
			// Backscroll search marks both rows by identity; no adjacency guess.
			echo := isDirectSearchInput(pd.Tool, pd.State.Input)
			if in := SerializeToolInput(pd.Tool, pd.State.Input); strings.TrimSpace(in) != "" {
				toolMsgs = append(toolMsgs, models.Message{Role: role, Origin: origin, Content: in, ContentType: "tool", Timestamp: ts, SearchEcho: echo})
			}
			if out := SerializeToolOutput(pd.State.Output); strings.TrimSpace(out) != "" {
				toolMsgs = append(toolMsgs, models.Message{Role: role, Origin: models.OriginAutomation, Content: out, ContentType: "tool", Timestamp: ts, SearchEcho: echo})
			}
		}
	}
	if err := flush(); err != nil {
		return models.ParsedFile{}, err
	}

	if err := rows.Err(); err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse rows %s: %w", dbPath, err)
	}

	return models.ParsedFile{
		Path:    dbPath,
		Hash:    hash,
		Records: msgs,
	}, nil
}

// openReadOnly opens a SQLite database in read-only mode.
func openReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

// openCodeOriginForRole maps only OpenCode's validated actor roles. Tool
// outputs are assigned automation at the structured part boundary instead.
func openCodeOriginForRole(role string) models.MessageOrigin {
	switch role {
	case "user":
		return models.OriginHuman
	case "assistant":
		return models.OriginAssistant
	default:
		return models.OriginUnknown
	}
}

// normalizeOpenCodeRole maps OpenCode roles to canonical backscroll roles.
func normalizeOpenCodeRole(role string) string {
	switch role {
	case "user", "system", "tool":
		return role
	case "assistant":
		return "assistant"
	default:
		return role
	}
}
