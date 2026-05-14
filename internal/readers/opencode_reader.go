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

// Hash returns a stable watermark for the database based on MAX(updated_at) of messages.
// If the database is empty or inaccessible, it returns an empty string and no error.
func (r *OpenCodeReader) Hash(dbPath string) (string, error) {
	db, err := openReadOnly(dbPath)
	if err != nil {
		return "", fmt.Errorf("opencode hash open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	var maxUpdated sql.NullInt64
	row := db.QueryRow(`SELECT MAX(updated_at) FROM messages`)
	if err := row.Scan(&maxUpdated); err != nil {
		return "", fmt.Errorf("opencode hash query %s: %w", dbPath, err)
	}
	if !maxUpdated.Valid {
		return "empty", nil
	}
	return fmt.Sprintf("%016x", maxUpdated.Int64), nil
}

// Parse reads all messages from the OpenCode database and returns them as a ParsedFile.
// Only parts of type "text" are indexed; reasoning, tool_call, tool_result, and finish are skipped.
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
		SELECT id, session_id, role, parts, created_at
		FROM messages
		ORDER BY created_at ASC
	`)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("opencode parse query %s: %w", dbPath, err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []models.Message
	for rows.Next() {
		var (
			id        string
			sessionID string
			role      string
			partsJSON string
			createdAt int64
		)
		if err := rows.Scan(&id, &sessionID, &role, &partsJSON, &createdAt); err != nil {
			continue
		}

		text := extractTextFromParts(partsJSON)
		if text == "" {
			continue
		}

		msgs = append(msgs, models.Message{
			Role:        normalizeOpenCodeRole(role),
			Content:     text,
			ContentType: "text",
			Timestamp:   time.UnixMilli(createdAt),
		})
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

// openCodePart represents one element of the OpenCode parts JSON array.
type openCodePart struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// textData is the data payload for a "text" part.
type textData struct {
	Text string `json:"text"`
}

// extractTextFromParts parses the parts JSON array and returns the joined text content.
func extractTextFromParts(partsJSON string) string {
	var parts []openCodePart
	if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil {
		return ""
	}

	var textParts []string
	for _, p := range parts {
		if p.Type != "text" {
			continue
		}
		var td textData
		if err := json.Unmarshal(p.Data, &td); err != nil {
			continue
		}
		trimmed := strings.TrimSpace(td.Text)
		if trimmed != "" {
			textParts = append(textParts, trimmed)
		}
	}
	return strings.Join(textParts, "\n")
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
