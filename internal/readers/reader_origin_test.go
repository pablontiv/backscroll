package readers

import (
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestMessageOriginForRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want models.MessageOrigin
	}{
		{name: "user", role: "user", want: models.OriginHuman},
		{name: "assistant", role: "assistant", want: models.OriginAssistant},
		{name: "reasoning", role: "reasoning", want: models.OriginAssistant},
		{name: "system", role: "system", want: models.OriginSystem},
		{name: "tool", role: "tool", want: models.OriginAutomation},
		{name: "empty", role: "", want: models.OriginUnknown},
		{name: "unrecognized", role: "developer", want: models.OriginUnknown},
		{name: "historical origin label", role: "human", want: models.OriginUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageOriginForRole(tt.role); got != tt.want {
				t.Fatalf("messageOriginForRole(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}
