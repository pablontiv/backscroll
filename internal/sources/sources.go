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

const sourceReadBufferSize = 32 * 1024

// ParseDocumentContext parses a whole-document source with cancellation support.
// It extracts the frontmatter ID and returns the entire content as one item.
func ParseDocumentContext(ctx context.Context, path string, sourceType string) (SourceItem, error) {
	content, id, err := prepareSource(ctx, path, sourceType)
	if err != nil {
		return SourceItem{}, err
	}

	return SourceItem{
		ID:      id,
		Source:  sourceType,
		Content: content,
		Path:    path,
	}, nil
}

// ParseSectionedContext splits a Markdown source by ## headers with cancellation support.
// If no ## headers are found, it returns the entire content as a single item.
func ParseSectionedContext(ctx context.Context, path string, sourceType string) ([]SourceItem, error) {
	content, baseID, err := prepareSource(ctx, path, sourceType)
	if err != nil {
		return nil, err
	}
	return splitSections(ctx, content, path, sourceType, baseID)
}

// prepareSource shares the cancellable read, conversion, and base-ID extraction
// while leaving whole-document and section expansion to their respective parsers.
func prepareSource(ctx context.Context, path, sourceType string) (string, string, error) {
	data, err := readSourceFile(ctx, path)
	if err != nil {
		return "", "", fmt.Errorf("failed to read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	content := string(data)
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	baseID, err := extractID(ctx, content, sourceType)
	if err != nil {
		return "", "", err
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return content, baseID, nil
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
