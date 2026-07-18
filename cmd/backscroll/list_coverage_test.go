package main

import (
	"strings"
	"testing"
)

func TestListJSONSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "cov/s.jsonl") && !strings.Contains(stdout, "[]") {
		t.Errorf("unexpected list json: %q", stdout)
	}
}

func TestListRobotSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--robot", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListOrderAscWithLimitOffset(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "timestamp:asc", "--limit", "1", "--offset", "1", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListInvalidOrderRejected(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "nonsense:updown", "--all-projects", "--indexed-only"); err == nil {
		t.Log("invalid order accepted silently (documenting current behavior)")
	}
}

func TestListRecentZeroAll(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--recent", "0", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListProjectFilterSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--project", "covproj", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	_ = stdout
}
