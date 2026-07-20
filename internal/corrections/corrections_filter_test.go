package corrections

import (
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestLexiconDetectorIgnoresToolResultText(t *testing.T) {
	// A tool_result row with tool-like content that quotes a lexicon phrase.
	// It should NOT trigger because content_type='tool'.
	msgs := []models.Message{
		{Role: "assistant", Content: "Bash command=go test", ContentType: "tool", ToolName: "Bash", Timestamp: time.Now()},
		{Role: "user", Content: "error: eso no es definido", ContentType: "tool", ToolName: "", Timestamp: time.Now()},
		{Role: "user", Content: "fix the error", ContentType: "text", Timestamp: time.Now()},
	}

	result := RunDetectorsFiltered(msgs)

	// Ordinal 1 (tool_result) should NOT be detected (content_type='tool')
	if _, foundAtOrdinal1 := result[1]; foundAtOrdinal1 {
		t.Errorf("tool_result row (ordinal 1) must NOT trigger detectors; got detections: %+v", result[1])
	}

	// Ordinal 2 (user prose "fix the error") should also not trigger
	// (doesn't match lexicon).
	if _, foundAtOrdinal2 := result[2]; foundAtOrdinal2 {
		t.Errorf("ordinal 2 should not trigger; got: %+v", result[2])
	}
}

func TestRephraseDetectorIgnoresToolResultText(t *testing.T) {
	// Two user prose messages with high overlap, then a tool_result
	// echoing the first. The tool_result should not be compared to
	// the first user message (content_type='tool').
	msgs := []models.Message{
		{Role: "user", Content: "go test please", ContentType: "text", Timestamp: time.Now()},
		{Role: "assistant", Content: "running...", ContentType: "text", Timestamp: time.Now()},
		{Role: "user", Content: "go test again please", ContentType: "text", Timestamp: time.Now()},
		{Role: "user", Content: "go test please", ContentType: "tool", Timestamp: time.Now()}, // tool_result echoing user
	}

	result := RunDetectorsFiltered(msgs)

	// Ordinal 2 (user prose) should trigger rephrase (high similarity to ordinal 0)
	if dets, ok := result[2]; !ok || len(dets) == 0 {
		t.Errorf("ordinal 2 (rephrase candidate) must be detected; got: %+v", result)
	}

	// Ordinal 3 (tool_result) must NOT trigger (content_type='tool')
	if _, foundAtOrdinal3 := result[3]; foundAtOrdinal3 {
		t.Errorf("ordinal 3 (tool_result) must NOT trigger; got: %+v", result[3])
	}
}

func TestInterruptDetectorRunsOnAllUserMessages(t *testing.T) {
	// Interrupt detector should trigger on ALL user messages, including tool-content messages
	msgs := []models.Message{
		{Role: "user", Content: "Let me try a completely different approach", ContentType: "text", WasInterrupted: true, Timestamp: time.Now()},
		{Role: "user", Content: "I need to reconsider this strategy now", ContentType: "tool", WasInterrupted: true, Timestamp: time.Now()},
	}

	result := RunDetectorsFiltered(msgs)

	// Both ordinals should have interrupt detector triggering
	if dets, ok := result[0]; !ok || len(dets) == 0 {
		t.Errorf("ordinal 0 (user text with interrupt) must trigger; got: %+v", result[0])
	}
	if dets, ok := result[1]; !ok || len(dets) == 0 {
		t.Errorf("ordinal 1 (user tool with interrupt) must trigger; got: %+v", result[1])
	}
}
