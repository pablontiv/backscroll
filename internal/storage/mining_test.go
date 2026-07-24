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
