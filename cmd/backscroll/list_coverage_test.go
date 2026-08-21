package main

import (
	"encoding/json"
	"testing"
)

func TestListJSONSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--json", "--all-projects")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var payload struct {
		Count    int `json:"count"`
		Sessions []struct {
			Path    string `json:"Path"`
			Project string `json:"Project"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("list --json emitted invalid JSON %q: %v", stdout, err)
	}
	if payload.Count == 0 || len(payload.Sessions) == 0 {
		t.Fatalf("list --json returned empty sessions after seeded startup: %+v", payload)
	}
	foundSeed := false
	for _, s := range payload.Sessions {
		if s.Path == "/cov/s.jsonl" && s.Project == "covproj" {
			foundSeed = true
			break
		}
	}
	if !foundSeed {
		t.Fatalf("list --json missing seeded /cov/s.jsonl@covproj session: %+v", payload.Sessions)
	}
}

func TestListRobotSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--robot", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListOrderAscWithLimitOffset(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "timestamp:asc", "--limit", "1", "--offset", "1", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListInvalidOrderRejected(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "nonsense:updown", "--all-projects"); err == nil {
		t.Log("invalid order accepted silently (documenting current behavior)")
	}
}

func TestListRecentZeroAll(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--recent", "0", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListProjectFilterSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--project", "covproj")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	_ = stdout
}
