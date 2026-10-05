package main

import (
	"strings"
	"testing"
)

func TestBackscrollSkillPayloadManualPresetCopyIncludesCodex(t *testing.T) {
	for _, line := range strings.Split(embeddedBackscrollSkill, "\n") {
		if !strings.HasPrefix(line, "cp -n inputs/") {
			continue
		}
		for _, arg := range strings.Fields(line) {
			if arg == "inputs/codex.inputs.toml" {
				return
			}
		}
		t.Fatalf("manual preset-copy command omits Codex: %s", line)
	}
	t.Fatal("manual preset-copy command not found")
}
