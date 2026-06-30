package sync

import (
	"testing"
)

func TestGetNoisePatterns(t *testing.T) {
	np := GetNoisePatterns()
	if np == nil {
		t.Fatal("GetNoisePatterns returned nil")
	}

	// Check filtered types
	for _, typ := range []string{"system-reminder", "task-notification", "command", "command-result", "local-command-caveat", "progress"} {
		if !np.FilteredTypes[typ] {
			t.Errorf("expected FilteredTypes[%q] = true", typ)
		}
	}

	// Patterns should be compiled
	if np.SystemReminderPattern == nil {
		t.Error("SystemReminderPattern is nil")
	}
	if np.TaskNotificationPattern == nil {
		t.Error("TaskNotificationPattern is nil")
	}
	if np.CaveatPattern == nil {
		t.Error("CaveatPattern is nil")
	}
	if np.CommandPattern == nil {
		t.Error("CommandPattern is nil")
	}
}

func TestGetNoisePatternsLazy(t *testing.T) {
	// Calling twice should return the same pointer (lazy init)
	np1 := GetNoisePatterns()
	np2 := GetNoisePatterns()
	if np1 != np2 {
		t.Error("GetNoisePatterns should return the same pointer (singleton)")
	}
}

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
