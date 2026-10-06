package models

import "testing"

func TestValidMessageOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin MessageOrigin
		want   bool
	}{
		{name: "human", origin: OriginHuman, want: true},
		{name: "assistant", origin: OriginAssistant, want: true},
		{name: "system", origin: OriginSystem, want: true},
		{name: "automation", origin: OriginAutomation, want: true},
		{name: "unknown", origin: OriginUnknown, want: true},
		{name: "zero", origin: "", want: false},
		{name: "invalid", origin: "user", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidMessageOrigin(tt.origin); got != tt.want {
				t.Fatalf("ValidMessageOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestPersistedMessageOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin MessageOrigin
		want   MessageOrigin
	}{
		{name: "human", origin: OriginHuman, want: OriginHuman},
		{name: "assistant", origin: OriginAssistant, want: OriginAssistant},
		{name: "system", origin: OriginSystem, want: OriginSystem},
		{name: "automation", origin: OriginAutomation, want: OriginAutomation},
		{name: "unknown", origin: OriginUnknown, want: OriginUnknown},
		{name: "zero", origin: "", want: OriginUnknown},
		{name: "invalid", origin: "user", want: OriginUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PersistedMessageOrigin(tt.origin); got != tt.want {
				t.Fatalf("PersistedMessageOrigin(%q) = %q, want %q", tt.origin, got, tt.want)
			}
		})
	}
}

func TestOriginFieldsRetainParserBackedValue(t *testing.T) {
	message := Message{Origin: OriginHuman}
	if message.Origin != OriginHuman {
		t.Fatalf("Message.Origin = %q, want %q", message.Origin, OriginHuman)
	}

	record := IndexedRecord{Origin: OriginAutomation}
	if record.Origin != OriginAutomation {
		t.Fatalf("IndexedRecord.Origin = %q, want %q", record.Origin, OriginAutomation)
	}
}
