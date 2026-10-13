package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/storage"
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

// Registry repair must not consume native replay work beyond the shared cap.
func TestProjectProofBacklogConvergesAcrossRegistryRebuild(t *testing.T) {
	for _, count := range []int{201, 401} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			dbPath, _ := testEnv(t)
			root := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
			dir := filepath.Join(root, "-home-shared-proj")
			old := time.Unix(1577836800, 0)
			for i := 0; i < count+2; i++ {
				path := filepath.Join(dir, fmt.Sprintf("%03d.jsonl", i))
				writeFile(t, path, fmt.Sprintf(`{"type":"user","uuid":"proof-%03d","cwd":"/actual/native-label","timestamp":"2020-01-01T00:00:00Z","message":{"role":"user","content":"backlogquartz"}}`+"\n", i))
				if err := os.Chtimes(path, old, old); err != nil {
					t.Fatal(err)
				}
			}
			preset, err := os.ReadFile("../../inputs/claude.inputs.toml")
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "claude.inputs.toml"), replaceManifestRoots(t, "inputs/claude.inputs.toml", string(preset), root))
			invoke := func(args ...string) {
				t.Helper()
				if _, stderr, err := runCmd(args...); err != nil {
					t.Fatalf("%v: %v %s", args, err, stderr)
				}
			}
			invoke("status", "--json")
			db, err := storage.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			type row struct {
				id         int64
				label      string
				proof      sql.NullString
				uuid, text string
			}
			read := func() []row {
				t.Helper()
				rs, err := db.DB().Query("SELECT id,project,project_evidence,uuid,text FROM search_items ORDER BY uuid")
				if err != nil {
					t.Fatal(err)
				}
				defer rs.Close()
				var out []row
				for rs.Next() {
					var r row
					if err := rs.Scan(&r.id, &r.label, &r.proof, &r.uuid, &r.text); err != nil {
						t.Fatal(err)
					}
					out = append(out, r)
				}
				if err := rs.Err(); err != nil {
					t.Fatal(err)
				}
				return out
			}
			before := read()
			// One unavailable historical source and one expired proven native row.
			for i := count; i < count+2; i++ {
				if err := os.Remove(filepath.Join(dir, fmt.Sprintf("%03d.jsonl", i))); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.DB().Exec("UPDATE search_items SET project='historical',project_evidence=NULL WHERE uuid != ?", fmt.Sprintf("proof-%03d", count+1)); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(os.Getenv("HOME"), ".config", "backscroll", "projects.toml"), "[[projects]]\nid=\"encoded-registry\"\nroots=[\"/home/shared/proj\"]\n")
			processed := 0
			for processed < count {
				if processed == 200 {
					invoke("status", "--json")
				} else {
					invoke("rebuild")
				}
				processed += 200
				if processed > count {
					processed = count
				}
				after := read()
				if len(after) != len(before) {
					t.Fatal("row count changed")
				}
				for i, r := range after {
					if r.id != before[i].id || r.uuid != before[i].uuid || r.text != before[i].text {
						t.Fatalf("identity/payload changed: %+v", r)
					}
					if i < processed || i == count+1 {
						if r.label != "native-label" || r.proof.String != "native_cwd" {
							t.Fatalf("native row %d: %+v", i, r)
						}
					} else if r.label != "historical" || r.proof.Valid {
						t.Fatalf("unreplayed row %d consumed: %+v", i, r)
					}
				}
			}
			invoke("rebuild")
			if got := read()[count]; got.proof.Valid || got.label != "historical" {
				t.Fatalf("unavailable source relabeled: %+v", got)
			}
		})
	}
}
