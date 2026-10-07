package sources

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

const sourceReadBufferSize = 32 * 1024

// ParseDocument parses a whole-document source (single item).
// Extracts frontmatter ID and returns the entire content as one item.
func ParseDocument(path string, sourceType string) (SourceItem, error) {
	return ParseDocumentContext(context.Background(), path, sourceType)
}

// ParseDocumentContext is ParseDocument with cancellation support.
func ParseDocumentContext(ctx context.Context, path string, sourceType string) (SourceItem, error) {
	data, err := readSourceFile(ctx, path)
	if err != nil {
		return SourceItem{}, fmt.Errorf("failed to read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return SourceItem{}, err
	}
	content := string(data)
	if err := ctx.Err(); err != nil {
		return SourceItem{}, err
	}
	id, err := extractID(ctx, content, sourceType)
	if err != nil {
		return SourceItem{}, err
	}
	if err := ctx.Err(); err != nil {
		return SourceItem{}, err
	}

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
	return ParseSectionedContext(context.Background(), path, sourceType)
}

// ParseSectionedContext is ParseSectioned with cancellation support.
func ParseSectionedContext(ctx context.Context, path string, sourceType string) ([]SourceItem, error) {
	data, err := readSourceFile(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content := string(data)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseID, err := extractID(ctx, content, sourceType)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return splitSections(ctx, content, path, sourceType, baseID)
}

func readSourceFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var content bytes.Buffer
	buffer := make([]byte, sourceReadBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := file.Read(buffer)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n > 0 {
			_, _ = content.Write(buffer[:n])
		}
		if readErr == io.EOF {
			return content.Bytes(), nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func splitSections(ctx context.Context, content, path, sourceType, baseID string) ([]SourceItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var items []SourceItem
	var currentTitle string
	var currentContent strings.Builder
	sectionCount := 0

	appendSection := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		sectionContent := strings.TrimSpace(currentContent.String())
		if err := ctx.Err(); err != nil {
			return err
		}
		sectionCount++
		items = append(items, SourceItem{
			ID:      fmt.Sprintf("%s-%d", baseID, sectionCount),
			Source:  sourceType,
			Content: sectionContent,
			Path:    path,
		})
		return ctx.Err()
	}

	err := visitLines(ctx, content, func(line string) (bool, error) {
		if strings.HasPrefix(line, "## ") {
			if currentTitle != "" {
				if err := appendSection(); err != nil {
					return false, err
				}
				currentContent.Reset()
			}
			currentTitle = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		} else if currentTitle != "" {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	if currentTitle != "" {
		if err := appendSection(); err != nil {
			return nil, err
		}
	}

	if len(items) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(content)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items = []SourceItem{{ID: baseID, Source: sourceType, Content: trimmed, Path: path}}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// extractID extracts the ID from the document's frontmatter.
// Looks for keys: id, name, or generates a default ID based on source type.
func extractID(ctx context.Context, content string, sourceType string) (string, error) {
	inFrontmatter := false
	id := ""
	found := false
	err := visitLines(ctx, content, func(line string) (bool, error) {
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				return false, nil
			}
			return true, nil
		}
		if !inFrontmatter {
			return false, nil
		}
		if strings.HasPrefix(strings.ToLower(line), "id:") {
			value := strings.TrimPrefix(line, "id:")
			value = strings.TrimPrefix(value, "ID:")
			id = strings.TrimSpace(value)
			found = true
			return true, nil
		}
		if strings.HasPrefix(strings.ToLower(line), "name:") {
			value := strings.TrimPrefix(line, "name:")
			value = strings.TrimPrefix(value, "Name:")
			id = strings.TrimSpace(value)
			found = true
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return "", err
	}
	if found {
		return id, nil
	}
	return fmt.Sprintf("%s-default", sourceType), nil
}

func visitLines(ctx context.Context, content string, visit func(line string) (done bool, err error)) error {
	for start := 0; ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		relativeEnd := strings.IndexByte(content[start:], '\n')
		if err := ctx.Err(); err != nil {
			return err
		}
		end := len(content)
		if relativeEnd >= 0 {
			end = start + relativeEnd
		}
		done, err := visit(content[start:end])
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if done || relativeEnd < 0 {
			return nil
		}
		start = end + 1
	}
}
