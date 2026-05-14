package readers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	bsync "github.com/pablontiv/backscroll/internal/sync"
)

const claudeFixture = "../../tests/fixtures/claude-tool-events.jsonl"

func TestJsonlReader_Name(t *testing.T) {
	r := &JsonlReader{}
	if r.Name() != "jsonl" {
		t.Errorf("Name() = %q, want %q", r.Name(), "jsonl")
	}
}

func TestJsonlReader_Hash(t *testing.T) {
	r := &JsonlReader{}

	want, err := bsync.HashFile(claudeFixture)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}

	got, err := r.Hash(claudeFixture)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if got != want {
		t.Errorf("Hash mismatch: got %q, want %q", got, want)
	}
}

func TestJsonlReader_Hash_Missing(t *testing.T) {
	r := &JsonlReader{}
	_, err := r.Hash("/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestJsonlReader_Parse_MatchesParseSessions(t *testing.T) {
	r := &JsonlReader{}
	def := input_config.InputDefinition{}

	pf, err := r.Parse(claudeFixture, def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	msgs, err := bsync.ParseSessions(claudeFixture)
	if err != nil {
		t.Fatalf("ParseSessions: %v", err)
	}

	if pf.Path != claudeFixture {
		t.Errorf("Path = %q, want %q", pf.Path, claudeFixture)
	}
	if pf.Hash == "" {
		t.Error("Hash should not be empty")
	}
	if len(pf.Records) != len(msgs) {
		t.Errorf("Records count = %d, ParseSessions count = %d", len(pf.Records), len(msgs))
	}
	for i := range msgs {
		if pf.Records[i].Role != msgs[i].Role {
			t.Errorf("Record[%d].Role = %q, want %q", i, pf.Records[i].Role, msgs[i].Role)
		}
		if pf.Records[i].Content != msgs[i].Content {
			t.Errorf("Record[%d].Content mismatch", i)
		}
	}
}

func TestJsonlReader_Discover(t *testing.T) {
	dir := t.TempDir()

	// Create a couple of JSONL files
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := &JsonlReader{}
	def := input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{
			Roots:   []string{dir},
			Include: []string{"*.jsonl"},
		},
	}

	paths, err := r.Discover(def)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("Discover returned %d paths, want 2", len(paths))
	}
}

func TestJsonlReader_Discover_Empty(t *testing.T) {
	r := &JsonlReader{}
	def := input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{
			Roots:   []string{},
			Include: []string{"*.jsonl"},
		},
	}

	paths, err := r.Discover(def)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("Discover returned %d paths, want 0", len(paths))
	}
}

func TestJsonlReader_Parse_IncludesHash(t *testing.T) {
	r := &JsonlReader{}
	def := input_config.InputDefinition{}

	pf, err := r.Parse(claudeFixture, def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want, err := bsync.HashFile(claudeFixture)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Hash != want {
		t.Errorf("ParsedFile.Hash = %q, want %q", pf.Hash, want)
	}
}

func TestJsonlReader_Parse_Missing(t *testing.T) {
	r := &JsonlReader{}
	_, err := r.Parse("/nonexistent/session.jsonl", input_config.InputDefinition{})
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestJsonlReader_Parse_Timestamps(t *testing.T) {
	r := &JsonlReader{}
	def := input_config.InputDefinition{}

	pf, err := r.Parse(claudeFixture, def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	zero := time.Time{}
	for i, rec := range pf.Records {
		if rec.Timestamp == zero {
			t.Errorf("Record[%d].Timestamp is zero", i)
		}
	}
}

func TestJsonlReader_ImplementsSessionReader(t *testing.T) {
	var _ SessionReader = &JsonlReader{}
}
