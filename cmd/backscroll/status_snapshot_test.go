package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/startuplock"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestStatusRetainsInvocationSnapshot(t *testing.T) {
	for _, swap := range []string{"fifo", "file-swap"} {
		for _, jsonFormat := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/json=%v", swap, jsonFormat), func(t *testing.T) {
				t.Chdir(t.TempDir())
				home := t.TempDir()
				dbPath := filepath.Join(home, "index.db")
				setIndexPolicyEnv(t, dbPath, home)
				originalRoot, replacementRoot := t.TempDir(), t.TempDir()
				for root, text := range map[string]string{originalRoot: "statusoriginalsentinel", replacementRoot: "statusreplacementsentinel"} {
					if err := os.WriteFile(filepath.Join(root, "evidence.md"), []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
				}
				dir := filepath.Join(home, "backscroll", "inputs")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "status.inputs.toml")
				original := snapshotManifest(originalRoot, "original", "markdown_document")
				replacement := snapshotManifest(replacementRoot, "replacement", "markdown_sections")
				coordinator := newStartupCoordinator()
				var written <-chan error
				if swap == "fifo" {
					written = changingConfigFIFO(t, path, original, replacement)
				} else {
					if err := os.WriteFile(path, []byte(original), 0600); err != nil {
						t.Fatal(err)
					}
					coordinator.tryAcquire = func(db string) (startupLease, bool, error) {
						if err := os.WriteFile(path+".new", []byte(replacement), 0600); err != nil {
							return nil, false, err
						}
						if err := os.Rename(path+".new", path); err != nil {
							return nil, false, err
						}
						return startuplock.TryAcquire(db)
					}
				}
				var out, errout bytes.Buffer
				root := buildRootCmdWithCoordinator(&out, &errout, coordinator)
				argv := []string{"status"}
				if jsonFormat {
					argv = append(argv, "--json")
				}
				root.SetArgs(argv)
				err := root.Execute()
				if written != nil {
					if writeErr := <-written; writeErr != nil {
						t.Fatal(writeErr)
					}
				}
				if err != nil {
					t.Fatalf("status: %v; stdout=%s stderr=%s", err, &out, &errout)
				}
				db, err := storage.OpenReadOnly(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				results, err := db.Search("statusoriginalsentinel", models.SearchOptions{Limit: 10})
				if err != nil || len(results) != 1 || results[0].SourcePath != filepath.Join(originalRoot, "evidence.md") {
					t.Fatalf("original input not indexed: %+v, %v", results, err)
				}
				results, err = db.Search("statusreplacementsentinel", models.SearchOptions{Limit: 10})
				if err != nil || len(results) != 0 {
					t.Fatalf("replacement input was indexed: %+v, %v", results, err)
				}
				if jsonFormat {
					var status struct {
						Config struct {
							ActiveInputs []string `json:"active_inputs"`
							Declarative  bool     `json:"using_declarative_inputs"`
						} `json:"config"`
						Index struct {
							Files int `json:"total_files"`
						} `json:"index"`
					}
					if err := json.Unmarshal(out.Bytes(), &status); err != nil {
						t.Fatal(err)
					}
					if !status.Config.Declarative || !reflect.DeepEqual(status.Config.ActiveInputs, []string{"original"}) || status.Index.Files != 1 {
						t.Fatalf("status did not reuse ingested snapshot: %s", &out)
					}
				} else if !strings.Contains(out.String(), "    - original\n") || strings.Contains(out.String(), "replacement") {
					t.Fatalf("text status did not reuse snapshot: %s", &out)
				}
			})
		}
	}
}
