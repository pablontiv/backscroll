package sync

import (
	"testing"
)

func TestIsNoiseType(t *testing.T) {
	tests := []struct {
		typ     string
		isNoise bool
	}{
		{"system-reminder", true},
		{"task-notification", true},
		{"command", true},
		{"command-result", true},
		{"local-command-caveat", true},
		{"progress", true},
		{"message", false},
		{"summary", false},
		{"", false},
	}
	for _, tc := range tests {
		got := IsNoiseType(tc.typ)
		if got != tc.isNoise {
			t.Errorf("IsNoiseType(%q) = %v, want %v", tc.typ, got, tc.isNoise)
		}
	}
}

func TestExportedNoiseHelpers(t *testing.T) {
	if !IsNoiseType("system-reminder") {
		t.Error("IsNoiseType(system-reminder) = false, want true")
	}
	if IsNoiseType("user") {
		t.Error("IsNoiseType(user) = true, want false")
	}
	got := CleanContent("hello <system-reminder>drop</system-reminder> world")
	if got != "hello world" {
		t.Errorf("CleanContent = %q, want %q", got, "hello world")
	}
}
