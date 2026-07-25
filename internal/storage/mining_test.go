package storage

import (
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/templates"
)

func TestIsInputSerialization(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Input serializations (should return true — skip from mining)
		{"Bash command=cd /foo && npm run test", true},
		{"Go command=go test ./...", true},
		{"TaskOutput block=false timeout=5000", true},
		{"Git command=git log --oneline", true},
		// Error output (should return false — include in mining)
		{"error: connection refused", false},
		{"FAIL: test suite", false},
		{"undefined variable x", false},
		{"", false}, // Empty string
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isInputSerialization(tt.input)
			if got != tt.expected {
				t.Errorf("isInputSerialization(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMineTemplatesForFileSyncsSkipsInputSerializations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	miner := templates.NewMiner()
	tx, _ := db.db.Begin()
	defer func() { _ = tx.Rollback() }()

	// Mix of tool messages: some are error outputs (include), some are input serializations (skip)
	isErrorTrue := true
	msgs := []IndexedMessage{
		{
			Ordinal:     0,
			UUID:        "u1",
			Role:        "assistant",
			ToolName:    "Bash",
			IsError:     &isErrorTrue,
			Text:        "error: undefined variable",
			ContentType: "tool",
		},
		{
			Ordinal:     1,
			UUID:        "u2",
			Role:        "assistant",
			ToolName:    "Bash",
			IsError:     &isErrorTrue,
			Text:        "Bash command=cd /foo && go test ./...",
			ContentType: "tool",
		},
		{
			Ordinal:     2,
			UUID:        "u3",
			Role:        "assistant",
			ToolName:    "Bash",
			IsError:     &isErrorTrue,
			Text:        "error: connection refused",
			ContentType: "tool",
		},
	}

	// Mine templates from the mix
	err = db.mineTemplatesForFile(tx, IndexedFile{
		SourcePath: "/test/s.jsonl",
		Messages:   msgs,
	}, miner)
	if err != nil {
		t.Fatalf("mineTemplatesForFile: %v", err)
	}

	// Only 2 error messages should produce templates (u1, u3).
	// Input serialization at u2 should be skipped.
	// Verify that no template from "Bash command=..." exists.
	var tmplCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM message_templates WHERE template_text LIKE '%command=%'`).Scan(&tmplCount); err != nil {
		t.Fatal(err)
	}
	if tmplCount != 0 {
		t.Errorf("sync-time mining should not create templates from input serializations; got %d", tmplCount)
	}

	// Verify that the function worked (mined templates from error messages)
	var totalCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&totalCount); err != nil {
		t.Fatal(err)
	}
	if totalCount == 0 {
		t.Errorf("sync-time mining should have mined templates from error messages; got 0")
	}
}

// TestIsInputSerializationKeepsErrorsWithKeyValues pins the discriminators that keep
// genuine error output out of the exclusion filter. ParseToolFromSerialized's heuristic
// is "first token has no '=', a later token does", which was written to recover a tool
// name from stored text, not to separate errors from inputs — so any error line
// carrying a key=value matched it and was discarded, throwing away exactly the
// recurring errors the census exists to surface.
func TestIsInputSerializationKeepsErrorsWithKeyValues(t *testing.T) {
	keep := []string{
		"error: cannot find package /tmp/a/b.go",
		"error: connection refused host=127.0.0.1 port=8080",
		"error: exit status 1: FOO=bar make failed",
		"--- FAIL: TestParse got=1 want=2",
		"error: Exit code 1",
		"FAIL github.com/x/y coverage=0%",
		"panic goroutine=1 [running]",
		"npm ERR! code=ELIFECYCLE",
		"Error connection refused addr=1.2.3.4",
	}
	for _, text := range keep {
		if isInputSerialization(text) {
			t.Errorf("genuine error output discarded as an input serialization: %q", text)
		}
	}

	drop := []string{
		"Bash command=go test ./...",
		"Edit file_path=/a old_string=b",
		"TaskOutput block=false timeout=5000",
	}
	for _, text := range drop {
		if !isInputSerialization(text) {
			t.Errorf("tool input serialization was not excluded: %q", text)
		}
	}
}
