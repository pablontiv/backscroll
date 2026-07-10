package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type cancelAfterChecksContext struct {
	context.Context
	checks   int
	cancelAt int
}

func (c *cancelAfterChecksContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestParseDocumentContextCancelsDuringChunkedRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.md")
	content := bytes.Repeat([]byte("x"), sourceReadBufferSize*3)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterChecksContext{Context: context.Background(), cancelAt: 6}

	item, err := ParseDocumentContext(ctx, path, "ke")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ParseDocumentContext() error = %v, want context.Canceled", err)
	}
	if item != (SourceItem{}) {
		t.Fatalf("ParseDocumentContext() returned partial item: %+v", item)
	}
}

func TestSplitSectionsCancelsDuringTraversalWithoutPartialItems(t *testing.T) {
	content := "## One\nfirst body\n## Two\nsecond body\n## Three\nthird body\n"
	ctx := &cancelAfterChecksContext{Context: context.Background(), cancelAt: 12}

	items, err := splitSections(ctx, content, "sections.md", "decision", "decision-id")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("splitSections() error = %v, want context.Canceled", err)
	}
	if items != nil {
		t.Fatalf("splitSections() returned partial items: %+v", items)
	}
}

func TestParseDocumentContext(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		sourceType string
		expectedID string
	}{
		{
			name: "KE with ID",
			content: `---
id: KE-0001
estado: activo
---

# Kyverno admission webhook timeout

## Symptom

Pods fail to schedule with admission webhook timeout errors.`,
			sourceType: "ke",
			expectedID: "KE-0001",
		},
		{
			name: "Memory with name",
			content: `---
name: feedback_example
description: Example feedback memory
type: feedback
---

Always verify state after deploy before declaring complete.`,
			sourceType: "memory",
			expectedID: "feedback_example",
		},
		{
			name: "No frontmatter",
			content: `# Test Rule

This is a rule file with no frontmatter.
Rules are plain markdown with instructions.`,
			sourceType: "rule",
			expectedID: "rule-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpfile, err := os.CreateTemp("", "source_*.md")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer func() { _ = os.Remove(tmpfile.Name()) }()

			if _, err := tmpfile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write to temp file: %v", err)
			}
			_ = tmpfile.Close()

			// Parse the file
			item, err := ParseDocumentContext(context.Background(), tmpfile.Name(), tt.sourceType)
			if err != nil {
				t.Fatalf("ParseDocumentContext failed: %v", err)
			}

			if item.ID != tt.expectedID {
				t.Errorf("expected ID %q, got %q", tt.expectedID, item.ID)
			}
			if item.Source != tt.sourceType {
				t.Errorf("expected Source %q, got %q", tt.sourceType, item.Source)
			}
			if item.Content != tt.content {
				t.Errorf("content mismatch")
			}
		})
	}
}

func TestParseSectionedContext(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		sourceType        string
		expectedItemCount int
		expectedIDs       []string
	}{
		{
			name: "spec with sections",
			content: `---
tipo: spec
estado: draft
---

# Test Spec

## Section One

Content for section one.

## Section Two

Content for section two with more details.`,
			sourceType:        "spec",
			expectedItemCount: 2,
			expectedIDs:       []string{"spec-default-1", "spec-default-2"},
		},
		{
			name: "decision with ID and sections",
			content: `---
id: DEC-AUTH-001
status: accepted
scope: technical
---

# Implement OAuth 2.0 for API Authentication

We've decided to implement OAuth 2.0 as the standard authentication mechanism for all external API access.

## Rationale

OAuth 2.0 provides industry-standard security guarantees and allows for flexible delegation of access rights.

## Consequences

All client applications must be updated to support OAuth 2.0.`,
			sourceType:        "decision",
			expectedItemCount: 2,
			expectedIDs:       []string{"DEC-AUTH-001-1", "DEC-AUTH-001-2"},
		},
		{
			name: "no sections",
			content: `# Test Backlog

Investigation needed for test purposes.`,
			sourceType:        "backlog",
			expectedItemCount: 1,
			expectedIDs:       []string{"backlog-default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpfile, err := os.CreateTemp("", "source_sectioned_*.md")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer func() { _ = os.Remove(tmpfile.Name()) }()

			if _, err := tmpfile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write to temp file: %v", err)
			}
			_ = tmpfile.Close()

			// Parse the file
			items, err := ParseSectionedContext(context.Background(), tmpfile.Name(), tt.sourceType)
			if err != nil {
				t.Fatalf("ParseSectionedContext failed: %v", err)
			}

			if len(items) != tt.expectedItemCount {
				t.Errorf("expected %d items, got %d", tt.expectedItemCount, len(items))
			}

			for i, item := range items {
				if i < len(tt.expectedIDs) && item.ID != tt.expectedIDs[i] {
					t.Errorf("item %d: expected ID %q, got %q", i, tt.expectedIDs[i], item.ID)
				}
				if item.Source != tt.sourceType {
					t.Errorf("item %d: expected Source %q, got %q", i, tt.sourceType, item.Source)
				}
				if item.Content == "" {
					t.Errorf("item %d: content is empty", i)
				}
			}
		})
	}
}

func TestMarkdownPreparationContracts(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		source   string
		baseID   string
		headed   bool
		sections []string
	}{
		{
			name:     "frontmatter ID and headed sections",
			content:  "\n---\nid: document-id\n---\n# Preamble\nDiscarded in section mode.\n\n## First\nBody with trailing spaces.  \n\n### Nested\nNested body.\n\n## Second\n  Indented body.\n\n",
			source:   "decision",
			baseID:   "document-id",
			headed:   true,
			sections: []string{"## First\nBody with trailing spaces.  \n\n### Nested\nNested body.", "## Second\n  Indented body."},
		},
		{
			name:     "single headed section",
			content:  "# Preamble\n\n## Only\nBody.\n",
			source:   "spec",
			baseID:   "spec-default",
			headed:   true,
			sections: []string{"## Only\nBody."},
		},
		{
			name:     "frontmatter name and trimmed fallback",
			content:  " \n---\nname: named-document\n---\n# Title\n\nBody with trailing spaces.  \n### Nested\nText.\n \n",
			source:   "backlog",
			baseID:   "named-document",
			sections: []string{"---\nname: named-document\n---\n# Title\n\nBody with trailing spaces.  \n### Nested\nText."},
		},
		{
			name:     "source default and long line",
			content:  " \n# Title\n" + string(bytes.Repeat([]byte("x"), sourceReadBufferSize*3)) + "\n \n",
			source:   "memory",
			baseID:   "memory-default",
			sections: []string{"# Title\n" + string(bytes.Repeat([]byte("x"), sourceReadBufferSize*3))},
		},
		{
			name:     "empty document",
			source:   "rule",
			baseID:   "rule-default",
			sections: []string{""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "document.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			item, err := ParseDocumentContext(context.Background(), path, tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if want := (SourceItem{ID: tt.baseID, Source: tt.source, Content: tt.content, Path: path}); item != want {
				t.Fatal("whole document must retain its exact bytes, base ID, source, and path")
			}
			items, err := ParseSectionedContext(context.Background(), path, tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != len(tt.sections) {
				t.Fatalf("section count = %d, want %d", len(items), len(tt.sections))
			}
			for i, content := range tt.sections {
				id := tt.baseID
				if tt.headed {
					id = fmt.Sprintf("%s-%d", id, i+1)
				}
				if want := (SourceItem{ID: id, Source: tt.source, Content: content, Path: path}); items[i] != want {
					t.Fatalf("section %d must retain its exact content, ordinal ID, source, and path", i)
				}
			}
		})
	}
}

func TestExtractID(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		sourceType string
		expectedID string
	}{
		{
			name: "extract ID from frontmatter",
			content: `---
id: KE-0001
estado: activo
---

# Content`,
			sourceType: "ke",
			expectedID: "KE-0001",
		},
		{
			name: "extract name from frontmatter",
			content: `---
name: my_memory
type: feedback
---

# Content`,
			sourceType: "memory",
			expectedID: "my_memory",
		},
		{
			name: "case insensitive",
			content: `---
ID: TEST-123
---

# Content`,
			sourceType: "rule",
			expectedID: "TEST-123",
		},
		{
			name: "no frontmatter",
			content: `# Just Content

No frontmatter here.`,
			sourceType: "spec",
			expectedID: "spec-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := extractID(context.Background(), tt.content, tt.sourceType)
			if err != nil {
				t.Fatal(err)
			}
			if id != tt.expectedID {
				t.Errorf("expected %q, got %q", tt.expectedID, id)
			}
		})
	}
}

func TestFixtureFiles(t *testing.T) {
	// Test with actual fixture files if they exist
	fixtureDir := "/home/shared/harness/backscroll/tests/fixtures"

	// Test KE fixture
	keFile := filepath.Join(fixtureDir, "ke-001.md")
	if _, err := os.Stat(keFile); err == nil {
		item, err := ParseDocumentContext(context.Background(), keFile, "ke")
		if err != nil {
			t.Errorf("ParseDocumentContext(ke-001.md) failed: %v", err)
		} else {
			if item.ID == "" {
				t.Errorf("KE fixture: empty ID")
			}
			if item.Source != "ke" {
				t.Errorf("KE fixture: wrong source type: %s", item.Source)
			}
		}
	}

	// Test Decision fixture (sectioned)
	decisionFile := filepath.Join(fixtureDir, "decisions-test-001.md")
	if _, err := os.Stat(decisionFile); err == nil {
		items, err := ParseSectionedContext(context.Background(), decisionFile, "decision")
		if err != nil {
			t.Errorf("ParseSectionedContext(decisions-test-001.md) failed: %v", err)
		} else {
			if len(items) == 0 {
				t.Errorf("Decision fixture: no items parsed")
			}
			for _, item := range items {
				if item.Source != "decision" {
					t.Errorf("Decision fixture: wrong source type: %s", item.Source)
				}
			}
		}
	}

	// Test Memory fixture
	memoryFile := filepath.Join(fixtureDir, "memory-test.md")
	if _, err := os.Stat(memoryFile); err == nil {
		item, err := ParseDocumentContext(context.Background(), memoryFile, "memory")
		if err != nil {
			t.Errorf("ParseDocumentContext(memory-test.md) failed: %v", err)
		} else {
			if item.Source != "memory" {
				t.Errorf("Memory fixture: wrong source type: %s", item.Source)
			}
		}
	}

	// Test Spec fixture (sectioned)
	specFile := filepath.Join(fixtureDir, "spec-test.md")
	if _, err := os.Stat(specFile); err == nil {
		items, err := ParseSectionedContext(context.Background(), specFile, "spec")
		if err != nil {
			t.Errorf("ParseSectionedContext(spec-test.md) failed: %v", err)
		} else {
			if len(items) == 0 {
				t.Errorf("Spec fixture: no items parsed")
			}
			for _, item := range items {
				if item.Source != "spec" {
					t.Errorf("Spec fixture: wrong source type: %s", item.Source)
				}
			}
		}
	}

	// Test Rule fixture
	ruleFile := filepath.Join(fixtureDir, "rule-test.md")
	if _, err := os.Stat(ruleFile); err == nil {
		items, err := ParseSectionedContext(context.Background(), ruleFile, "rule")
		if err != nil {
			t.Errorf("ParseSectionedContext(rule-test.md) failed: %v", err)
		} else {
			if len(items) == 0 {
				t.Errorf("Rule fixture: no items parsed")
			}
			for _, item := range items {
				if item.Source != "rule" {
					t.Errorf("Rule fixture: wrong source type: %s", item.Source)
				}
			}
		}
	}

	// Test Backlog fixture
	backlogFile := filepath.Join(fixtureDir, "backlog-test.md")
	if _, err := os.Stat(backlogFile); err == nil {
		items, err := ParseSectionedContext(context.Background(), backlogFile, "backlog")
		if err != nil {
			t.Errorf("ParseSectionedContext(backlog-test.md) failed: %v", err)
		} else {
			if len(items) == 0 {
				t.Errorf("Backlog fixture: no items parsed")
			}
			for _, item := range items {
				if item.Source != "backlog" {
					t.Errorf("Backlog fixture: wrong source type: %s", item.Source)
				}
			}
		}
	}
}
