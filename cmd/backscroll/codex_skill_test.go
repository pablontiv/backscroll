package main

import (
	"strings"
	"testing"
)

func TestBackscrollSkillManualPresetCopyIncludesCodex(t *testing.T) {
	_, skill := readTrackedSkillMarkdown(t, ".claude/skills/backscroll/SKILL.md")
	for _, line := range strings.Split(skill, "\n") {
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
