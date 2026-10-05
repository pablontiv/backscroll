package main

import (
	_ "embed"
	"errors"
	"strings"
)

// embeddedBackscrollSkill is the authoritative instruction payload printed by
// the --skill entry point. The discoverable SKILL.md is intentionally only a
// small shim that directs runtimes to this payload.
//
//go:embed backscroll_skill.md
var embeddedBackscrollSkill string

var errSkillMustBeUsedAlone = errors.New("usage: --skill must be used alone")

func validateSkillInvocation(args []string) (bool, error) {
	hasSkill := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--skill" || strings.HasPrefix(arg, "--skill=") {
			hasSkill = true
			break
		}
	}
	if !hasSkill {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--skill" {
		return true, nil
	}
	return false, errSkillMustBeUsedAlone
}
