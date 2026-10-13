package main

import (
	"github.com/pablontiv/backscroll/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeProjectProofSurvivesExpiredSourceRebuild(t *testing.T) {
	dbPath, _ := testEnv(t)
	root := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	source := filepath.Join(root, "-home-shared-proj", "native.jsonl")
	writeFile(t, source, `{"type":"user","uuid":"native-proof","cwd":"/actual/native-label","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"nativeproofquartz"}}`+"\n")
	preset, err := os.ReadFile("../../inputs/claude.inputs.toml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "claude.inputs.toml"), replaceManifestRoots(t, "inputs/claude.inputs.toml", string(preset), root))
	if _, stderr, err := runCmd("search", "--text", "nativeproofquartz", "--all-projects", "--lexical-only", "--json"); err != nil {
		t.Fatalf("%v %s", err, stderr)
	}
	read := func() (int64, string, string, string, string) {
		t.Helper()
		db, err := storage.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var id int64
		var label, proof, uuid, text string
		if err := db.DB().QueryRow("SELECT id,project,project_evidence,uuid,text FROM search_items WHERE uuid='native-proof'").Scan(&id, &label, &proof, &uuid, &text); err != nil {
			t.Fatal(err)
		}
		return id, label, proof, uuid, text
	}
	id, label, proof, uuid, text := read()
	if label != "native-label" || proof != "native_cwd" {
		t.Fatalf("native proof missing: %s %s", label, proof)
	}
	// An unchanged surviving legacy source must replay to recover native proof,
	// rather than allowing its encoded directory to overwrite the native cwd.
	legacy, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.DB().Exec("UPDATE search_items SET project_evidence=NULL WHERE uuid='native-proof'"); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runCmd("rebuild"); err != nil {
		t.Fatalf("%v %s", err, stderr)
	}
	replayID, replayLabel, replayProof, replayUUID, replayText := read()
	if replayID != id || replayLabel != label || replayProof != proof || replayUUID != uuid || replayText != text {
		t.Fatal("surviving legacy source did not regain native proof")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(os.Getenv("HOME"), ".config", "backscroll", "projects.toml"), "[[projects]]\nid = \"wrong-path-label\"\nroots = [\"/Users/Shared/proj\"]\naliases = [\"/home/shared/proj\"]\n")
	if _, stderr, err := runCmd("rebuild"); err != nil {
		t.Fatalf("%v %s", err, stderr)
	}
	gotID, gotLabel, gotProof, gotUUID, gotText := read()
	if gotID != id || gotLabel != label || gotProof != proof || gotUUID != uuid || gotText != text {
		t.Fatal("expired native row changed")
	}
}
