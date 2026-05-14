package input_config

import (
	"testing"
)

func TestEvalPredicate(t *testing.T) {
	tests := []struct {
		name  string
		op    string
		value any
		input any
		want  bool
	}{
		{"eq string match", "eq", "user", "user", true},
		{"eq string no match", "eq", "user", "assistant", false},
		{"eq bool true", "eq", true, true, true},
		{"eq bool false", "eq", true, false, false},
		{"ne string", "ne", "user", "assistant", true},
		{"ne string same", "ne", "user", "user", false},
		{"in string list", "in", []any{"user", "assistant"}, "user", true},
		{"in string miss", "in", []any{"user", "assistant"}, "system", false},
		{"exists non-nil", "exists", nil, "hello", true},
		{"exists nil", "exists", nil, nil, false},
		{"missing nil", "missing", nil, nil, true},
		{"missing non-nil", "missing", nil, "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Predicate{Op: tt.op, Value: tt.value}
			got, err := EvalPredicate(p, tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvalPredicate_unknownOp(t *testing.T) {
	p := Predicate{Op: "contains", Value: "x"}
	_, err := EvalPredicate(p, "xyz")
	if err == nil {
		t.Error("expected error for unknown op")
	}
}

func TestEvalPredicates(t *testing.T) {
	record := map[string]any{
		"type":   "user",
		"isMeta": false,
	}

	// include_when from claude preset: type in [user, assistant]
	predicates := []Predicate{
		{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
	}
	ok, err := EvalPredicates(predicates, record)
	if err != nil {
		t.Fatalf("EvalPredicates: %v", err)
	}
	if !ok {
		t.Error("expected true for user type")
	}

	// exclude_when: isMeta eq true (should be false here)
	excl := []Predicate{
		{Selector: "$.isMeta", Op: "eq", Value: true},
	}
	ok, err = EvalPredicates(excl, record)
	if err != nil {
		t.Fatalf("EvalPredicates: %v", err)
	}
	if ok {
		t.Error("expected false for isMeta=false")
	}
}

func TestEvalPredicates_ANDSemantics(t *testing.T) {
	record := map[string]any{"a": "x", "b": "y"}
	predicates := []Predicate{
		{Selector: "$.a", Op: "eq", Value: "x"},
		{Selector: "$.b", Op: "eq", Value: "z"}, // fails
	}
	ok, err := EvalPredicates(predicates, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("AND: both must pass, expected false")
	}
}
