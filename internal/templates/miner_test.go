package templates

import (
	"strings"
	"testing"
)

func TestMinerProcessLine(t *testing.T) {
	m := NewMiner()

	tests := []struct {
		name string
		line string
		want string // substring expected in template.Text
	}{
		{
			name: "error with numeric",
			line: "error: file descriptor 123 not found",
			want: "error: file descriptor <*> not found",
		},
		{
			name: "path substitution",
			line: "go: open /home/user/project/file.go: no such file",
			want: "go: open <*> no such file",
		},
		{
			name: "empty",
			line: "   ",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := m.ProcessLine(tt.line)
			if tt.want == "" {
				if tmpl.Text != "" {
					t.Errorf("want empty template, got %q", tmpl.Text)
				}
			} else {
				if tmpl.Text != tt.want {
					t.Errorf("template text = %q, want %q", tmpl.Text, tt.want)
				}
			}
			if tmpl.Signature == "" && tt.want != "" {
				t.Errorf("signature empty but template non-empty")
			}
		})
	}
}

func TestMinerDeterminism(t *testing.T) {
	m := NewMiner()
	line := "error: connection refused 127.0.0.1:8080"

	tmpl1 := m.ProcessLine(line)
	tmpl2 := m.ProcessLine(line)

	if tmpl1.Signature != tmpl2.Signature {
		t.Errorf("signature not deterministic: %q vs %q", tmpl1.Signature, tmpl2.Signature)
	}
	if tmpl1.Text != tmpl2.Text {
		t.Errorf("text not deterministic: %q vs %q", tmpl1.Text, tmpl2.Text)
	}
}

func TestMinerVariableDetection(t *testing.T) {
	m := NewMiner()

	line := "FAIL user.TestName (0.123s) /Users/pones/project/test_file.go:42"
	tmpl := m.ProcessLine(line)

	if !strings.Contains(tmpl.Text, "<*>") {
		t.Errorf("expected variables but got none: %q", tmpl.Text)
	}
	if tmpl.VariableCount == 0 {
		t.Errorf("VariableCount = 0, want > 0")
	}
}

func TestMinerNumericVariables(t *testing.T) {
	m := NewMiner()

	tests := []struct {
		line string
		want bool
	}{
		{"error: exit code 1", true},
		{"error: value 123.456", true},
		{"error: hex 0x1F", true},
		{"error: port -1", true},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			tmpl := m.ProcessLine(tt.line)
			hasVar := strings.Contains(tmpl.Text, "<*>")
			if hasVar != tt.want {
				t.Errorf("line %q: hasVar=%v, want %v (text=%q)", tt.line, hasVar, tt.want, tmpl.Text)
			}
		})
	}
}

func TestMinerQuotedStrings(t *testing.T) {
	m := NewMiner()

	line := `error: message "somevalue" end`
	tmpl := m.ProcessLine(line)

	if !strings.Contains(tmpl.Text, "<*>") {
		t.Errorf("expected quoted string to be variable: %q", tmpl.Text)
	}
}

func TestMinerPorts(t *testing.T) {
	m := NewMiner()

	line := "error: connection to localhost:8080 failed"
	tmpl := m.ProcessLine(line)

	if !strings.Contains(tmpl.Text, "<*>") {
		t.Errorf("expected port to be variable: %q", tmpl.Text)
	}
}

func TestMinerUUID(t *testing.T) {
	m := NewMiner()

	line := "error: request 550e8400e29b41d4aeb31d476bb62f60 not found"
	tmpl := m.ProcessLine(line)

	if !strings.Contains(tmpl.Text, "<*>") {
		t.Errorf("expected UUID to be variable: %q", tmpl.Text)
	}
}
