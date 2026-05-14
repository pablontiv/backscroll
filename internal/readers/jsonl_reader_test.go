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
const piFixture = "../../tests/fixtures/pi-session.jsonl"

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

// piDef returns an InputDefinition matching the Pi preset for use in regression tests.
func piDef() input_config.InputDefinition {
	return input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "jsonl"},
		Record: input_config.RecordConfig{
			Selector: "$",
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "eq", Value: "message"},
				{Selector: "$.message.role", Op: "in", Value: []any{"user", "assistant"}},
			},
		},
		Map: input_config.MapConfig{
			Role:      "$.message.role",
			UUID:      "$.id",
			Timestamp: "$.timestamp",
			Project:   "$.cwd",
		},
		Content: input_config.ContentConfig{
			Selector:  "$.message.content",
			BlockText: "$.text",
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "eq", Value: "text"},
			},
		},
		Text: input_config.TextConfig{
			Join:      "\n",
			Trim:      true,
			DropEmpty: true,
		},
	}
}

func TestJsonlReader_Parse_PiDeclarative(t *testing.T) {
	r := &JsonlReader{}
	def := piDef()

	pf, err := r.Parse(piFixture, def)
	if err != nil {
		t.Fatalf("Parse Pi: %v", err)
	}

	if pf.Path != piFixture {
		t.Errorf("Path = %q, want %q", pf.Path, piFixture)
	}
	if pf.Hash == "" {
		t.Error("Hash should not be empty")
	}
	// Pi fixture has 2 message-type records with user/assistant roles; toolResult excluded
	if len(pf.Records) != 2 {
		t.Errorf("Pi records = %d, want 2 (user + assistant, toolResult excluded)", len(pf.Records))
	}
}

func TestJsonlReader_Parse_PiRoles(t *testing.T) {
	r := &JsonlReader{}
	def := piDef()

	pf, err := r.Parse(piFixture, def)
	if err != nil {
		t.Fatalf("Parse Pi: %v", err)
	}

	for _, rec := range pf.Records {
		if rec.Role != "user" && rec.Role != "assistant" {
			t.Errorf("unexpected role %q (toolResult should be excluded)", rec.Role)
		}
	}
}

func TestJsonlReader_Parse_PiThinkingExcluded(t *testing.T) {
	r := &JsonlReader{}
	def := piDef()

	pf, err := r.Parse(piFixture, def)
	if err != nil {
		t.Fatalf("Parse Pi: %v", err)
	}

	// Thinking blocks and toolCall blocks should not appear in content
	for _, rec := range pf.Records {
		if contains(rec.Content, "hidden reasoning should not index") {
			t.Error("thinking block content leaked into indexed content")
		}
	}
}

func TestJsonlReader_Parse_PiVisibleContent(t *testing.T) {
	r := &JsonlReader{}
	def := piDef()

	pf, err := r.Parse(piFixture, def)
	if err != nil {
		t.Fatalf("Parse Pi: %v", err)
	}

	found := false
	for _, rec := range pf.Records {
		if contains(rec.Content, "pi visible answer") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'pi visible answer' in indexed content")
	}
}

func TestJsonlReader_Parse_DeclarativePathChosen(t *testing.T) {
	r := &JsonlReader{}

	// With MapConfig set → declarative path
	defWithMap := input_config.InputDefinition{
		Map: input_config.MapConfig{Role: "$.message.role"},
	}
	pf, err := r.Parse(claudeFixture, defWithMap)
	if err != nil {
		t.Fatalf("declarative path: %v", err)
	}
	if pf.Hash == "" {
		t.Error("Hash should not be empty on declarative path")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
