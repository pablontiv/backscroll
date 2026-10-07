package readers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sources"
)

// MarkdownDocumentReader parses an entire Markdown file as one document record.
type MarkdownDocumentReader struct{}

// Name returns the declarative decode format for whole-document Markdown inputs.
func (r *MarkdownDocumentReader) Name() string { return "markdown_document" }

// Discover returns files matching the input definition's discovery rules.
func (r *MarkdownDocumentReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	return discoverMarkdownFiles(ctx, def)
}

// Hash returns the SHA-256 hash of the Markdown file at path.
func (r *MarkdownDocumentReader) Hash(ctx context.Context, path string) (string, error) {
	return hashMarkdownFile(ctx, path)
}

// Parse reads path using the existing whole-document source parser.
func (r *MarkdownDocumentReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	return parseMarkdownFile(ctx, path, def.Source, parseMarkdownDocument)
}

// MarkdownSectionsReader parses Markdown files into records split by ## sections.
type MarkdownSectionsReader struct{}

// Name returns the declarative decode format for sectioned Markdown inputs.
func (r *MarkdownSectionsReader) Name() string { return "markdown_sections" }

// Discover returns files matching the input definition's discovery rules.
func (r *MarkdownSectionsReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	return discoverMarkdownFiles(ctx, def)
}

// Hash returns the SHA-256 hash of the Markdown file at path.
func (r *MarkdownSectionsReader) Hash(ctx context.Context, path string) (string, error) {
	return hashMarkdownFile(ctx, path)
}

// Parse reads path using the existing sectioned source parser.
func (r *MarkdownSectionsReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	return parseMarkdownFile(ctx, path, def.Source, parseMarkdownSections)
}

func discoverMarkdownFiles(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	paths, err := input_config.DiscoverFilesContext(ctx, def.Discover)
	if err != nil {
		return nil, fmt.Errorf("discover markdown files: %w", err)
	}
	return paths, nil
}

func hashMarkdownFile(ctx context.Context, path string) (string, error) {
	hash, err := hashFile(ctx, path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hash, nil
}

type markdownParser func(ctx context.Context, path, source string) ([]sources.SourceItem, error)

func parseMarkdownFile(ctx context.Context, path, source string, parser markdownParser) (models.ParsedFile, error) {
	hash, err := hashMarkdownFile(ctx, path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	if err := ctx.Err(); err != nil {
		return models.ParsedFile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return models.ParsedFile{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return models.ParsedFile{}, err
	}

	items, err := parser(ctx, path, source)
	if err != nil {
		return models.ParsedFile{}, err
	}
	if err := ctx.Err(); err != nil {
		return models.ParsedFile{}, err
	}

	messages := make([]models.Message, 0, len(items))
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}
		messages = append(messages, models.Message{
			Role:        "document",
			Origin:      models.OriginUnknown,
			Content:     item.Content,
			ContentType: "text",
			Timestamp:   info.ModTime(),
			UUID:        "",
		})
	}
	if err := ctx.Err(); err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{
		Path:    path,
		Hash:    hash,
		Records: messages,
		Cwd:     filepath.Dir(path),
	}, nil
}

func parseMarkdownDocument(ctx context.Context, path, source string) ([]sources.SourceItem, error) {
	item, err := sources.ParseDocumentContext(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse markdown document %s: %w", path, err)
	}
	return []sources.SourceItem{item}, nil
}

func parseMarkdownSections(ctx context.Context, path, source string) ([]sources.SourceItem, error) {
	items, err := sources.ParseSectionedContext(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse markdown sections %s: %w", path, err)
	}
	return items, nil
}
