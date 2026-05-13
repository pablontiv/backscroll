package plans

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlan(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedCount int
		expectedTitle string
	}{
		{
			name:          "single section",
			content:       "## First Section\n\nContent here",
			expectedCount: 1,
			expectedTitle: "First Section",
		},
		{
			name: "multiple sections",
			content: `## Overview
Content for overview

## Details
More detailed content

## Summary
Final summary`,
			expectedCount: 3,
			expectedTitle: "Overview",
		},
		{
			name:          "no sections (no headers)",
			content:       "This is just plain content without any headers",
			expectedCount: 1,
			expectedTitle: "Untitled",
		},
		{
			name: "sections with blank lines",
			content: `## First

Content

## Second

More content`,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", "plan_*.md")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write to temp file: %v", err)
			}
			tmpfile.Close()

			// Parse the file
			sections, err := ParsePlan(tmpfile.Name())
			if err != nil {
				t.Fatalf("ParsePlan failed: %v", err)
			}

			if len(sections) != tt.expectedCount {
				t.Errorf("expected %d sections, got %d", tt.expectedCount, len(sections))
			}

			if tt.expectedTitle != "" && len(sections) > 0 {
				if sections[0].Title != tt.expectedTitle {
					t.Errorf("expected first title %q, got %q", tt.expectedTitle, sections[0].Title)
				}
			}

			// Verify all sections have Source = "plan"
			for _, section := range sections {
				if section.Source != "plan" {
					t.Errorf("expected Source=%q, got %q", "plan", section.Source)
				}
				if section.Content == "" {
					t.Errorf("section %q has empty content", section.Title)
				}
			}
		})
	}
}

func TestDiscoverPlanFiles(t *testing.T) {
	// Create temp directory
	tmpdir, err := os.MkdirTemp("", "plans_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create some test files
	testFiles := []string{"plan1.md", "plan2.md", "notes.txt", "README.md"}
	for _, f := range testFiles {
		path := filepath.Join(tmpdir, f)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Discover files
	files, err := DiscoverPlanFiles(tmpdir)
	if err != nil {
		t.Fatalf("DiscoverPlanFiles failed: %v", err)
	}

	// Should find 3 .md files (plan1.md, plan2.md, README.md)
	if len(files) != 3 {
		t.Errorf("expected 3 .md files, got %d", len(files))
	}

	// Verify all returned files are .md
	for _, f := range files {
		if filepath.Ext(f) != ".md" {
			t.Errorf("expected .md file, got %s", f)
		}
	}
}

func TestDiscoverPlanFilesEmpty(t *testing.T) {
	// Create temp directory with no files
	tmpdir, err := os.MkdirTemp("", "plans_empty_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Discover files
	files, err := DiscoverPlanFiles(tmpdir)
	if err != nil {
		t.Fatalf("DiscoverPlanFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

// Helper function to check if a path has a given extension
func hasExtension(path, ext string) bool {
	return filepath.Ext(path) == ext
}

// Adjust the test to use the correct check
func TestDiscoverPlanFilesVerifyMd(t *testing.T) {
	// Create temp directory
	tmpdir, err := os.MkdirTemp("", "plans_verify_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create test files
	testFiles := []string{"plan1.md", "plan2.md", "notes.txt"}
	for _, f := range testFiles {
		path := filepath.Join(tmpdir, f)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	files, err := DiscoverPlanFiles(tmpdir)
	if err != nil {
		t.Fatalf("DiscoverPlanFiles failed: %v", err)
	}

	// Should find 2 .md files
	if len(files) != 2 {
		t.Errorf("expected 2 .md files, got %d: %v", len(files), files)
	}

	for _, f := range files {
		if filepath.Ext(f) != ".md" {
			t.Errorf("expected .md file, got %s", f)
		}
	}
}
