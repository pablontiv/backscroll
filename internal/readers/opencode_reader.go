package readers

import (
	"database/sql"
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
func (r *OpenCodeReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

// Hash returns a stable watermark for the database based on MAX(time_updated) of messages.
// If the database is empty or inaccessible, it returns "empty" and no error.
func (r *OpenCodeReader) Hash(dbPath string) (string, error) {
	db, err := openReadOnly(dbPath)
	if err != nil {
		return "", fmt.Errorf("opencode hash open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	var maxUpdated sql.NullInt64
	row := db.QueryRow(`SELECT MAX(time_updated) FROM message`)
	if err := row.Scan(&maxUpdated); err != nil {
		return "", fmt.Errorf("opencode hash query %s: %w", dbPath, err)
	}
	if !maxUpdated.Valid {
		return "empty", nil
	}
	return fmt.Sprintf("%016x", maxUpdated.Int64), nil
}

type msgInfoData struct {
	Role string `json:"role"`
}

type partInfoData struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Ignored *bool  `json:"ignored"`
}

// Parse reads all messages from the OpenCode database and returns them as a ParsedFile.
// Only parts of type "text" with ignored != true are indexed; all other part types are skipped.
func (r *OpenCodeReader) Parse(dbPath string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := r.Hash(dbPath)
	if err != nil {
		return models.ParsedFile{}, err
	}

	db, err := openReadOnly(dbPath)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`
		SELECT m.id, m.session_id, m.data, m.time_created, p.data
		FROM message m
		JOIN part p ON p.message_id = m.id
		ORDER BY m.time_created ASC, p.id ASC
	`)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse query %s: %w", dbPath, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		msgs         []models.Message
		currentMsgID string
		currentRole  string
		currentTime  int64
		textParts    []string
	)

	flush := func() {
		if len(textParts) == 0 {
			return
		}
		msgs = append(msgs, models.Message{
			Role:        normalizeOpenCodeRole(currentRole),
			Content:     strings.Join(textParts, "\n"),
			ContentType: "text",
			Timestamp:   time.UnixMilli(currentTime),
		})
	}

	for rows.Next() {
		var (
			msgID       string
			sessionID   string
			msgData     string
			timeCreated int64
			partData    string
		)
		if err := rows.Scan(&msgID, &sessionID, &msgData, &timeCreated, &partData); err != nil {
			continue
		}

		var pd partInfoData
		if err := json.Unmarshal([]byte(partData), &pd); err != nil || pd.Type != "text" {
			continue
		}
		if pd.Ignored != nil && *pd.Ignored {
			continue
		}
		text := strings.TrimSpace(pd.Text)
		if text == "" {
			continue
		}

		if msgID != currentMsgID {
			flush()
			currentMsgID = msgID
			textParts = nil
			var md msgInfoData
			if err := json.Unmarshal([]byte(msgData), &md); err == nil {
				currentRole = md.Role
			} else {
				currentRole = ""
			}
			currentTime = timeCreated
		}
		textParts = append(textParts, text)
	}
	flush()

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
