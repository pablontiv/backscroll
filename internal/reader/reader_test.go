package reader_test

import (
	"testing"

	"github.com/pablontiv/backscroll/internal/reader"
)

const piFixture = "../../tests/fixtures/pi-session.jsonl"

func TestReadFile(t *testing.T) {
	msgs, err := reader.ReadFile(piFixture)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected at least one message, got zero")
	}
	for _, m := range msgs {
		if m.Role == "" {
			t.Errorf("message has empty role: %+v", m)
		}
	}
}

func TestReadFileNotExist(t *testing.T) {
	_, err := reader.ReadFile("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}
