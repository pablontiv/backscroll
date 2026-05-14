package sources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDocument(t *testing.T) {
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
			item, err := ParseDocument(tmpfile.Name(), tt.sourceType)
			if err != nil {
				t.Fatalf("ParseDocument failed: %v", err)
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

func TestParseSectioned(t *testing.T) {
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
			items, err := ParseSectioned(tmpfile.Name(), tt.sourceType)
			if err != nil {
				t.Fatalf("ParseSectioned failed: %v", err)
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

func TestParseAll(t *testing.T) {
	// Create temp directory and files
	tmpdir, err := os.MkdirTemp("", "sources_all_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpdir) }()

	// Create test files
	keContent := `---
id: KE-0001
estado: activo
---

# Kyverno admission webhook timeout

## Symptom

Pods fail to schedule with admission webhook timeout errors.`

	decisionContent := `---
id: DEC-AUTH-001
status: accepted
scope: technical
---

# Implement OAuth 2.0 for API Authentication

## Rationale

OAuth 2.0 provides industry-standard security guarantees.`

	memoryContent := `---
name: feedback_example
description: Example feedback memory
type: feedback
---

Always verify state after deploy before declaring complete.`

	keFile := filepath.Join(tmpdir, "ke.md")
	decisionFile := filepath.Join(tmpdir, "decision.md")
	memoryFile := filepath.Join(tmpdir, "memory.md")

	if err := os.WriteFile(keFile, []byte(keContent), 0o644); err != nil {
		t.Fatalf("failed to create KE file: %v", err)
	}
	if err := os.WriteFile(decisionFile, []byte(decisionContent), 0o644); err != nil {
		t.Fatalf("failed to create Decision file: %v", err)
	}
	if err := os.WriteFile(memoryFile, []byte(memoryContent), 0o644); err != nil {
		t.Fatalf("failed to create Memory file: %v", err)
	}

	// Create config
	cfg := SourceConfig{
		KE:        []string{keFile},
		Decisions: []string{decisionFile},
		Memories:  []string{memoryFile},
	}

	// Parse all
	items, err := ParseAll(cfg)
	if err != nil {
		t.Fatalf("ParseAll failed: %v", err)
	}

	// We expect: 1 KE + 1 Decision (with 1 section) + 1 Memory = 3 items
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Verify source types
	sources := make(map[string]int)
	for _, item := range items {
		sources[item.Source]++
	}

	if sources["ke"] != 1 {
		t.Errorf("expected 1 ke item, got %d", sources["ke"])
	}
	if sources["decision"] != 1 {
		t.Errorf("expected 1 decision item, got %d", sources["decision"])
	}
	if sources["memory"] != 1 {
		t.Errorf("expected 1 memory item, got %d", sources["memory"])
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
			id := extractID(tt.content, tt.sourceType)
			if id != tt.expectedID {
				t.Errorf("expected %q, got %q", tt.expectedID, id)
			}
		})
	}
}

func TestParseAllWithAllTypes(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "sources_all_types_*")
	if err != nil {
		t.Fatalf("create tmpdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpdir) }()

	writeFile := func(name, content string) string {
		path := filepath.Join(tmpdir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	ruleFile := writeFile("rule.md", "---\nid: RULE-001\n---\n## Rule One\nDo this.\n## Rule Two\nDo that.")
	specFile := writeFile("spec.md", "---\nid: SPEC-001\n---\n## Spec One\nSpec detail.")
	backlogFile := writeFile("backlog.md", "---\nid: BACK-001\n---\n## Task One\nDescription.")

	cfg := SourceConfig{
		Rules:   []string{ruleFile},
		Specs:   []string{specFile},
		Backlog: []string{backlogFile},
	}

	items, err := ParseAll(cfg)
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}

	src := make(map[string]int)
	for _, item := range items {
		src[item.Source]++
	}
	if src["rule"] == 0 {
		t.Error("expected at least one rule item")
	}
	if src["spec"] == 0 {
		t.Error("expected at least one spec item")
	}
	if src["backlog"] == 0 {
		t.Error("expected at least one backlog item")
	}
}

func TestParseAllMissingFile(t *testing.T) {
	cfg := SourceConfig{
		KE: []string{"/nonexistent/ke.md"},
	}
	_, err := ParseAll(cfg)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestFixtureFiles(t *testing.T) {
	// Test with actual fixture files if they exist
	fixtureDir := "/home/shared/backscroll/tests/fixtures"

	// Test KE fixture
	keFile := filepath.Join(fixtureDir, "ke-001.md")
	if _, err := os.Stat(keFile); err == nil {
		item, err := ParseDocument(keFile, "ke")
		if err != nil {
			t.Errorf("ParseDocument(ke-001.md) failed: %v", err)
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
		items, err := ParseSectioned(decisionFile, "decision")
		if err != nil {
			t.Errorf("ParseSectioned(decisions-test-001.md) failed: %v", err)
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
		item, err := ParseDocument(memoryFile, "memory")
		if err != nil {
			t.Errorf("ParseDocument(memory-test.md) failed: %v", err)
		} else {
			if item.Source != "memory" {
				t.Errorf("Memory fixture: wrong source type: %s", item.Source)
			}
		}
	}

	// Test Spec fixture (sectioned)
	specFile := filepath.Join(fixtureDir, "spec-test.md")
	if _, err := os.Stat(specFile); err == nil {
		items, err := ParseSectioned(specFile, "spec")
		if err != nil {
			t.Errorf("ParseSectioned(spec-test.md) failed: %v", err)
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
		items, err := ParseSectioned(ruleFile, "rule")
		if err != nil {
			t.Errorf("ParseSectioned(rule-test.md) failed: %v", err)
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
		items, err := ParseSectioned(backlogFile, "backlog")
		if err != nil {
			t.Errorf("ParseSectioned(backlog-test.md) failed: %v", err)
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
