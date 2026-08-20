package readers

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestMarkdownDocumentReaderParse(t *testing.T) {
	path := writeMarkdownTestFile(t, "doc.md", "---\nid: ke-1\n---\n# Knowledge\n\nFull document body.\n")
	modTime := fixedMarkdownModTime(t, path)

	reader := &MarkdownDocumentReader{}
	if got := reader.Name(); got != "markdown_document" {
		t.Fatalf("Name() = %q, want markdown_document", got)
	}

	pf, err := reader.Parse(path, input_config.InputDefinition{Source: "ke"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if pf.Path != path {
		t.Errorf("ParsedFile.Path = %q, want %q", pf.Path, path)
	}
	if pf.Hash != sha256Hex("---\nid: ke-1\n---\n# Knowledge\n\nFull document body.\n") {
		t.Errorf("ParsedFile.Hash = %q, want SHA-256 of file content", pf.Hash)
	}
	if pf.Cwd != filepath.Dir(path) {
		t.Errorf("ParsedFile.Cwd = %q, want %q", pf.Cwd, filepath.Dir(path))
	}
	if len(pf.Records) != 1 {
		t.Fatalf("len(Records) = %d, want 1", len(pf.Records))
	}
	assertMarkdownMessage(t, pf.Records[0], "---\nid: ke-1\n---\n# Knowledge\n\nFull document body.\n", modTime)
}

func TestMarkdownSectionsReaderParse(t *testing.T) {
	content := "# Decisions\n\n## First\nAlpha decision.\n\n## Second\nBeta decision.\n"
	path := writeMarkdownTestFile(t, "decisions.md", content)
	modTime := fixedMarkdownModTime(t, path)

	reader := &MarkdownSectionsReader{}
	if got := reader.Name(); got != "markdown_sections" {
		t.Fatalf("Name() = %q, want markdown_sections", got)
	}

	pf, err := reader.Parse(path, input_config.InputDefinition{Source: "decision"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if pf.Path != path {
		t.Errorf("ParsedFile.Path = %q, want %q", pf.Path, path)
	}
	if pf.Hash != sha256Hex(content) {
		t.Errorf("ParsedFile.Hash = %q, want SHA-256 of file content", pf.Hash)
	}
	if pf.Cwd != filepath.Dir(path) {
		t.Errorf("ParsedFile.Cwd = %q, want %q", pf.Cwd, filepath.Dir(path))
	}
	if len(pf.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2", len(pf.Records))
	}
	assertMarkdownMessage(t, pf.Records[0], "## First\nAlpha decision.", modTime)
	assertMarkdownMessage(t, pf.Records[1], "## Second\nBeta decision.", modTime)
}

func TestMarkdownSectionsReaderFallsBackToDocument(t *testing.T) {
	path := writeMarkdownTestFile(t, "notes.md", "\nNo section heading here.\n\n")
	modTime := fixedMarkdownModTime(t, path)

	pf, err := (&MarkdownSectionsReader{}).Parse(path, input_config.InputDefinition{Source: "rule"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(pf.Records) != 1 {
		t.Fatalf("len(Records) = %d, want 1", len(pf.Records))
	}
	assertMarkdownMessage(t, pf.Records[0], "No section heading here.", modTime)
}

func TestMarkdownReaderDiscoverAndHash(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.md"), "keep root")
	writeFile(t, filepath.Join(root, "skip.txt"), "skip")
	writeFile(t, filepath.Join(root, "nested", "keep.md"), "keep nested")
	excluded := filepath.Join(root, "nested", "excluded.md")
	writeFile(t, excluded, "exclude")

	reader := &MarkdownDocumentReader{}
	def := input_config.InputDefinition{Discover: input_config.DiscoverConfig{
		Roots:   []string{root},
		Include: []string{"**/*.md"},
		Exclude: []string{"excluded.md"},
	}}

	got, err := reader.Discover(def)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	sort.Strings(got)
	want := []string{filepath.Join(root, "keep.md"), filepath.Join(root, "nested", "keep.md")}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %#v, want %#v", got, want)
	}

	hash, err := reader.Hash(filepath.Join(root, "nested", "keep.md"))
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if hash != sha256Hex("keep nested") {
		t.Errorf("Hash() = %q, want SHA-256 of file content", hash)
	}
}

func TestMarkdownReaderReportsPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")
	_, err := (&MarkdownDocumentReader{}).Parse(missing, input_config.InputDefinition{Source: "ke"})
	if err == nil {
		t.Fatal("Parse() error = nil, want missing-file error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("Parse() error = %q, want it to include %q", err.Error(), missing)
	}
}

func assertMarkdownMessage(t *testing.T, msg models.Message, wantContent string, wantTimestamp time.Time) {
	t.Helper()
	if msg.Role != "document" {
		t.Errorf("Role = %q, want document", msg.Role)
	}
	if msg.ContentType != "text" {
		t.Errorf("ContentType = %q, want text", msg.ContentType)
	}
	if msg.UUID != "" {
		t.Errorf("UUID = %q, want empty", msg.UUID)
	}
	if msg.Content != wantContent {
		t.Errorf("Content = %q, want %q", msg.Content, wantContent)
	}
	if !msg.Timestamp.Equal(wantTimestamp) {
		t.Errorf("Timestamp = %s, want %s", msg.Timestamp, wantTimestamp)
	}
}

func writeMarkdownTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func fixedMarkdownModTime(t *testing.T, path string) time.Time {
	t.Helper()
	modTime := time.Date(2026, 8, 19, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	return modTime
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
