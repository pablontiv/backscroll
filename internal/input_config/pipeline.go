package input_config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
	internalsync "github.com/pablontiv/backscroll/internal/sync"
)

// TestRecord is a single parsed record from a dry-run of the input pipeline.
type TestRecord struct {
	Role      string `json:"role"`
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// TestFile runs the full input pipeline on a file as a dry-run (no DB writes).
// Returns the records that would be indexed, or an error.
func TestFile(path string, def InputDefinition) ([]TestRecord, error) {
	switch def.Decode.Format {
	case "jsonl", "":
		return testJSONL(path, def)
	default:
		return nil, fmt.Errorf("unsupported format for dry-run: %s", def.Decode.Format)
	}
}

func testJSONL(path string, def InputDefinition) ([]TestRecord, error) {
	var results []TestRecord
	err := internalsync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			return nil // skip malformed lines
		}

		// Apply include_when predicates
		if len(def.Record.IncludeWhen) > 0 {
			ok, err := EvalPredicates(def.Record.IncludeWhen, record)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}

		// Apply exclude_when predicates
		if len(def.Record.ExcludeWhen) > 0 {
			excluded, err := EvalPredicates(def.Record.ExcludeWhen, record)
			if err != nil {
				return err
			}
			if excluded {
				return nil
			}
		}

		// Map fields
		role, _ := SelectString(record, def.Map.Role)
		uuid, _ := SelectString(record, def.Map.UUID)
		ts, _ := SelectString(record, def.Map.Timestamp)
		sessionID, _ := SelectString(record, def.Map.SessionID)

		// Extract content (simplified: use content selector or string representation)
		content := extractRawContent(record, def.Content)

		// Apply text transforms
		content, err := ApplyTransforms(def.Text, content)
		if errors.Is(err, ErrDropped) {
			return nil
		}
		if err != nil {
			return err
		}
		if content == "" {
			return nil
		}

		results = append(results, TestRecord{
			Role:      role,
			UUID:      uuid,
			Timestamp: ts,
			SessionID: sessionID,
			Content:   content,
		})
		return nil
	})
	return results, err
}

// extractRawContent extracts content from a record using ContentConfig.
// This is a simplified extraction for dry-run purposes.
func extractRawContent(record map[string]any, cfg ContentConfig) string {
	if cfg.Selector == "" {
		return ""
	}

	raw, ok := SelectField(record, cfg.Selector)
	if !ok || raw == nil {
		return ""
	}

	// Try as string
	if s, ok := raw.(string); ok {
		return s
	}

	// Try as array of blocks
	blocks, ok := raw.([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}

		// Apply include_when for this block
		if len(cfg.IncludeWhen) > 0 {
			pass, err := EvalPredicates(cfg.IncludeWhen, block)
			if err != nil || !pass {
				continue
			}
		}

		// Extract block text
		if cfg.BlockText != "" {
			text, ok := SelectString(block, cfg.BlockText)
			if ok && text != "" {
				parts = append(parts, text)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	join := cfg.DefaultContentType
	if join == "" {
		join = "\n"
	}
	_ = join

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

// ParseDeclarative parses a JSONL file using the declarative pipeline from InputDefinition.
// It applies record predicates, extracts fields via MapConfig selectors,
// extracts content via ContentConfig, applies TextConfig transforms, and normalizes roles.
func ParseDeclarative(path string, def InputDefinition) ([]models.Message, error) {
	var results []models.Message
	err := internalsync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			return nil
		}

		if len(def.Record.IncludeWhen) > 0 {
			ok, err := EvalPredicates(def.Record.IncludeWhen, record)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}

		if len(def.Record.ExcludeWhen) > 0 {
			excluded, err := EvalPredicates(def.Record.ExcludeWhen, record)
			if err != nil {
				return err
			}
			if excluded {
				return nil
			}
		}

		role, _ := SelectString(record, def.Map.Role)
		role = normalizeRole(role)
		tsStr, _ := SelectString(record, def.Map.Timestamp)

		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			ts = time.Now()
		}

		content := extractRawContent(record, def.Content)
		content, err = ApplyTransforms(def.Text, content)
		if errors.Is(err, ErrDropped) {
			return nil
		}
		if err != nil {
			return err
		}
		if content == "" {
			return nil
		}

		results = append(results, models.Message{
			Role:        role,
			Content:     content,
			ContentType: "text",
			Timestamp:   ts,
		})
		return nil
	})
	return results, err
}

// normalizeRole maps non-standard role names to canonical values.
func normalizeRole(role string) string {
	if role == "human" {
		return "user"
	}
	return role
}
