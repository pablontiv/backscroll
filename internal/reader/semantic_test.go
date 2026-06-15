package reader

import (
	"strings"
	"testing"
)

func TestReadSemanticTailIncludesPiToolCallArguments(t *testing.T) {
	rows, err := ReadSemanticTail("../../tests/fixtures/pi-session.jsonl", 0)
	if err != nil {
		t.Fatalf("ReadSemanticTail: %v", err)
	}

	for _, row := range rows {
		if row.Kind == "tool_use" && strings.Contains(row.Content, "name=read") {
			if !strings.Contains(row.Content, `"path":"secret"`) {
				t.Fatalf("tool_use row missing Pi arguments payload: %#v", row)
			}
			return
		}
	}
	t.Fatalf("missing Pi tool_use row in %#v", rows)
}

func TestReadSemanticTailClassifiesPiToolResultRole(t *testing.T) {
	rows, err := ReadSemanticTail("../../tests/fixtures/pi-session.jsonl", 0)
	if err != nil {
		t.Fatalf("ReadSemanticTail: %v", err)
	}

	for _, row := range rows {
		if row.Role == "toolResult" {
			if row.Kind != "tool_result" {
				t.Fatalf("toolResult role row kind = %q, want tool_result: %#v", row.Kind, row)
			}
			if !strings.Contains(row.Content, "tool result should not index") {
				t.Fatalf("toolResult role row missing content snippet: %#v", row)
			}
			return
		}
	}
	t.Fatalf("missing Pi toolResult role row in %#v", rows)
}
