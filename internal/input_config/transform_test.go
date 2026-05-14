package input_config

import (
	"errors"
	"testing"
)

func TestApplyTransforms_remove_regex(t *testing.T) {
	cfg := TextConfig{
		Remove: []RemoveConfig{
			{Kind: "regex", Pattern: `<system-reminder>[\s\S]*?</system-reminder>`},
		},
	}
	input := "hello <system-reminder>noise</system-reminder> world"
	got, err := ApplyTransforms(cfg, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello  world" {
		t.Errorf("got %q, want %q", got, "hello  world")
	}
}

func TestApplyTransforms_remove_substring(t *testing.T) {
	cfg := TextConfig{
		Remove: []RemoveConfig{
			{Kind: "substring", Pattern: "REMOVE_ME"},
		},
	}
	got, err := ApplyTransforms(cfg, "hello REMOVE_ME world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello  world" {
		t.Errorf("got %q, want %q", got, "hello  world")
	}
}

func TestApplyTransforms_trim(t *testing.T) {
	cfg := TextConfig{Trim: true}
	got, err := ApplyTransforms(cfg, "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestApplyTransforms_dropEmpty(t *testing.T) {
	cfg := TextConfig{Trim: true, DropEmpty: true}
	_, err := ApplyTransforms(cfg, "   ")
	if !errors.Is(err, ErrDropped) {
		t.Errorf("expected ErrDropped, got %v", err)
	}
}

func TestApplyTransforms_dropEmpty_nonEmpty(t *testing.T) {
	cfg := TextConfig{Trim: true, DropEmpty: true}
	got, err := ApplyTransforms(cfg, "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestApplyTransforms_invalidRegex(t *testing.T) {
	cfg := TextConfig{
		Remove: []RemoveConfig{
			{Kind: "regex", Pattern: `[invalid`},
		},
	}
	_, err := ApplyTransforms(cfg, "text")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
	var pe *InvalidPatternError
	if !errors.As(err, &pe) {
		t.Errorf("expected InvalidPatternError, got %T", err)
	}
}

func TestApplyTransforms_order(t *testing.T) {
	// Transforms must apply in order: first remove, then trim
	cfg := TextConfig{
		Remove: []RemoveConfig{
			{Kind: "substring", Pattern: "NOISE"},
		},
		Trim: true,
	}
	got, err := ApplyTransforms(cfg, "  NOISE hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestApplyTransforms_claudePreset(t *testing.T) {
	// Verify the remove patterns from claude.inputs.toml actually work
	cfg := TextConfig{
		Remove: []RemoveConfig{
			{Kind: "regex", Pattern: `<system-reminder>[\s\S]*?</system-reminder>`},
			{Kind: "regex", Pattern: `<task-notification>[\s\S]*?</task-notification>`},
		},
		Trim:      true,
		DropEmpty: true,
	}
	input := "Hello\n<system-reminder>ignore this</system-reminder>\nWorld"
	got, err := ApplyTransforms(cfg, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result")
	}
	if contains(got, "system-reminder") || contains(got, "ignore this") {
		t.Errorf("system-reminder content not removed: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
