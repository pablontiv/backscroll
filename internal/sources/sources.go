package sources

import (
	"fmt"
	"os"
	"strings"
)

// SourceItem represents a parsed external source item.
type SourceItem struct {
	ID      string // extracted from frontmatter or generated
	Source  string // "ke", "decision", "memory", "rule", "spec", "backlog"
	Content string // full content of the item
	Path    string // path to the source file
}

// SourceConfig contains paths for different source types.
type SourceConfig struct {
	KE        []string `toml:"ke"`
	Decisions []string `toml:"decisions"`
	Memories  []string `toml:"memories"`
	Rules     []string `toml:"rules"`
	Specs     []string `toml:"specs"`
	Backlog   []string `toml:"backlog"`
}

// ParseAll returns all source items from the configured paths.
func ParseAll(cfg SourceConfig) ([]SourceItem, error) {
	var items []SourceItem

	// KE: whole-document parsing
	for _, path := range cfg.KE {
		item, err := ParseDocument(path, "ke")
		if err != nil {
			return nil, fmt.Errorf("failed to parse KE %s: %w", path, err)
		}
		items = append(items, item)
	}

	// Decisions: sectioned (by ## headers)
	for _, path := range cfg.Decisions {
		sectioned, err := ParseSectioned(path, "decision")
		if err != nil {
			return nil, fmt.Errorf("failed to parse Decision %s: %w", path, err)
		}
		items = append(items, sectioned...)
	}

	// Memories: whole-document parsing
	for _, path := range cfg.Memories {
		item, err := ParseDocument(path, "memory")
		if err != nil {
			return nil, fmt.Errorf("failed to parse Memory %s: %w", path, err)
		}
		items = append(items, item)
	}

	// Rules: sectioned (by ## headers)
	for _, path := range cfg.Rules {
		sectioned, err := ParseSectioned(path, "rule")
		if err != nil {
			return nil, fmt.Errorf("failed to parse Rule %s: %w", path, err)
		}
		items = append(items, sectioned...)
	}

	// Specs: sectioned (by ## headers)
	for _, path := range cfg.Specs {
		sectioned, err := ParseSectioned(path, "spec")
		if err != nil {
			return nil, fmt.Errorf("failed to parse Spec %s: %w", path, err)
		}
		items = append(items, sectioned...)
	}

	// Backlog: sectioned (by ## headers) or whole-document
	for _, path := range cfg.Backlog {
		sectioned, err := ParseSectioned(path, "backlog")
		if err != nil {
			return nil, fmt.Errorf("failed to parse Backlog %s: %w", path, err)
		}
		items = append(items, sectioned...)
	}

	return items, nil
}

// ParseDocument parses a whole-document source (single item).
// Extracts frontmatter ID and returns the entire content as one item.
func ParseDocument(path string, sourceType string) (SourceItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceItem{}, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	id := extractID(content, sourceType)

	return SourceItem{
		ID:      id,
		Source:  sourceType,
		Content: content,
		Path:    path,
	}, nil
}

// ParseSectioned parses a markdown file split by ## headers (each section = item).
// If no ## headers are found, returns the entire content as a single item.
func ParseSectioned(path string, sourceType string) ([]SourceItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Extract frontmatter ID
	baseID := extractID(content, sourceType)

	var items []SourceItem
	var currentTitle string
	var currentContent strings.Builder
	sectionCount := 0

	for _, line := range lines {
		// Check if this line starts a new section with ##
		if strings.HasPrefix(line, "## ") {
			// Save previous section if any
			if currentTitle != "" {
				sectionCount++
				item := SourceItem{
					ID:      fmt.Sprintf("%s-%d", baseID, sectionCount),
					Source:  sourceType,
					Content: strings.TrimSpace(currentContent.String()),
					Path:    path,
				}
				items = append(items, item)
				currentContent.Reset()
			}

			// Start new section
			currentTitle = strings.TrimPrefix(line, "## ")
			currentTitle = strings.TrimSpace(currentTitle)
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		} else if currentTitle != "" {
			// Add to current section
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}

	// Save last section if any
	if currentTitle != "" {
		sectionCount++
		item := SourceItem{
			ID:      fmt.Sprintf("%s-%d", baseID, sectionCount),
			Source:  sourceType,
			Content: strings.TrimSpace(currentContent.String()),
			Path:    path,
		}
		items = append(items, item)
	}

	// If no sections were found, return the entire content as one item
	if len(items) == 0 {
		items = []SourceItem{
			{
				ID:      baseID,
				Source:  sourceType,
				Content: strings.TrimSpace(content),
				Path:    path,
			},
		}
	}

	return items, nil
}

// extractID extracts the ID from the document's frontmatter.
// Looks for keys: id, name, or generates a default ID based on source type.
func extractID(content string, sourceType string) string {
	lines := strings.Split(content, "\n")

	// Look for frontmatter (starts with --- and ends with ---)
	inFrontmatter := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				break
			}
		}

		if !inFrontmatter {
			continue
		}

		// Look for id, name, or other identifying fields
		if strings.HasPrefix(strings.ToLower(line), "id:") {
			value := strings.TrimPrefix(line, "id:")
			value = strings.TrimPrefix(value, "ID:")
			return strings.TrimSpace(value)
		}
		if strings.HasPrefix(strings.ToLower(line), "name:") {
			value := strings.TrimPrefix(line, "name:")
			value = strings.TrimPrefix(value, "Name:")
			return strings.TrimSpace(value)
		}
	}

	// If no ID found, generate a default one
	return fmt.Sprintf("%s-default", sourceType)
}
