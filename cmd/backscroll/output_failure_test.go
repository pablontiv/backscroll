package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/pablontiv/backscroll/internal/storage"
)

var errRequiredOutput = errors.New("required output unavailable")

// Fail exactly one write, including late writes after a successfully delivered
// prefix. A handler that only checks its first or last write must still fail.
type outputFailureWriter struct {
	calls, failAt int
	buffer        bytes.Buffer
}

func (w *outputFailureWriter) String() string { return w.buffer.String() }
func (w *outputFailureWriter) Len() int       { return w.buffer.Len() }

func (w *outputFailureWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, errRequiredOutput
	}
	return w.buffer.Write(p)
}

func seedOutputBranches(t *testing.T, path string) {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	messages := []storage.IndexedMessage{
		{UUID: "output-anchor", Ordinal: 0, Role: "user", Origin: models.OriginHuman, Text: "Do X", ContentType: "text", Timestamp: "2026-01-01T00:00:00Z"},
		{UUID: "output-assistant", Ordinal: 1, Role: "assistant", Origin: models.OriginAssistant, Text: "Doing X", ContentType: "text", Timestamp: "2026-01-01T00:00:01Z"},
		{UUID: "output-correction", Ordinal: 2, Role: "user", Origin: models.OriginHuman, Text: "no, te pedí otra cosa", ContentType: "text", Timestamp: "2026-01-01T00:00:02Z"},
	}
	for i := 0; i < 3; i++ {
		messages = append(messages, storage.IndexedMessage{UUID: fmt.Sprintf("output-tool-%d", i), Ordinal: i + 3, Role: "assistant", Origin: models.OriginAssistant, Text: "error: database locked (errno 5)", ContentType: "tool", ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), Timestamp: fmt.Sprintf("2026-01-01T00:00:0%dZ", i+3), ExtractionVersion: storage.CurrentExtractionVersion})
	}
	if err := db.SyncFiles([]storage.IndexedFile{{SourcePath: "/output/session.jsonl", Source: "session", Hash: "output", Project: "output", Tags: []string{"testing"}, Messages: messages}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE indexed_files SET last_indexed = '2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE message_templates SET first_seen = '2026-01-01 00:00:00', last_seen = '2026-01-01 00:00:00'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// Removing any required-write error check must turn a subtest red. Exercise
// the public run path, real SQLite queries and all output modes, not serializers
// in isolation. Empty text/robot search deliberately emits no stdout payload.
func TestRequiredOutputFailures(t *testing.T) {
	// Captured from audited HEAD eeb9e8f before changing production writes.
	// Isolated paths and dynamic status size are normalized; seeded metadata is stable.
	goldens := map[string]string{}
	{
		data, err := os.ReadFile("testdata/required_output.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &goldens); err != nil {
			t.Fatal(err)
		}
	}
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated=%t", populated), func(t *testing.T) {
			path, _ := testEnv(t)
			if populated {
				seedOutputBranches(t, path)
			}
			commands := [][]string{{"config"}, {"status"}, {"validate"}, {"list", "--all-projects"}, {"search", "--text", "database", "--all-projects"}}
			for _, kind := range []string{"commands", "failures", "templates", "corrections", "sequences"} {
				commands = append(commands, []string{"patterns", "--kind", kind, "--all-projects", "--min-support", "1"})
			}
			commands = append(commands, []string{"search", "--text", "database", "--all-projects", "--fields", "full"}, []string{"list", "--project", "absent"}, []string{"patterns", "--kind", "corrections", "--origin", "human", "--all-projects"})
			for _, kind := range []string{"commands", "failures"} {
				commands = append(commands, []string{"patterns", "--kind", kind, "--all-projects", "--trend"})
			}
			if populated {
				commands = append(commands, []string{"context", "--uuid", "output-anchor"})
			}
			for _, command := range commands {
				modes := []string{"", "--json", "--robot"}
				if command[0] == "config" || command[0] == "status" || command[0] == "validate" {
					modes = modes[:2]
				}
				for _, mode := range modes {
					args := append([]string(nil), command...)
					if mode != "" {
						args = append(args, mode)
					}
					t.Run(strings.Join(args, " "), func(t *testing.T) {
						good := &outputFailureWriter{}
						if err := run(good, io.Discard, args); err != nil {
							t.Fatal(err)
						}
						key := fmt.Sprintf("%t/%s", populated, strings.Join(args, " "))
						normalized := strings.ReplaceAll(good.String(), filepath.Dir(path), "<env>")
						if command[0] == "status" {
							normalized = normalizeStatusSize(t, normalized, path, mode == "--json")
						}
						// C08 adds census text; validate its behavioral counts without
						// pinning incidental headings, spacing, or prose.
						censusText := command[0] == "patterns" && len(command) > 2 && command[2] == "failures" && mode == ""
						if censusText {
							total := 0
							if populated {
								total = 3
							}
							for _, count := range []string{fmt.Sprintf("Total events: %d", total), fmt.Sprintf("Signalled events (with error signal): %d", total), "Unsignalled events: 0"} {
								if !strings.Contains(normalized, count) {
									t.Fatalf("missing census %q: %s", count, normalized)
								}
							}
						} else if want, ok := goldens[key]; !ok || want != normalized {
							t.Fatalf("output incompatible with audited baseline\nwant: %s\ngot: %s", want, normalized)
						}
						if goldens[key] != "" && good.Len() == 0 {
							t.Fatal("nonempty branch produced no output")
						}
						for failAt := 1; failAt <= good.calls; failAt++ {
							bad := &outputFailureWriter{failAt: failAt}
							root := buildRootCmd(bad, io.Discard)
							root.SetArgs(args)
							if err := root.Execute(); !errors.Is(err, errRequiredOutput) {
								t.Errorf("handler write %d/%d: want output error, got %v", failAt, good.calls, err)
							}
							if err := run(&outputFailureWriter{failAt: failAt}, io.Discard, args); !errors.Is(err, errRequiredOutput) {
								t.Errorf("run write %d: %v", failAt, err)
							}
						}
						// Stderr hints and progress are advisory, not required payload writes.
						var again bytes.Buffer
						if err := run(&again, &outputFailureWriter{failAt: 1}, args); err != nil {
							t.Fatal(err)
						}
						if again.String() != good.String() {
							t.Fatal("successful output changed when advisory stderr failed")
						}
					})
				}
			}
		})
	}
}

// SQLite layout is not an output contract. Check the live byte count and its
// representation before masking only that value; the golden still checks every
// semantic field, JSON type, key, and surrounding machine-output byte.
func normalizeStatusSize(t *testing.T, payload, path string, jsonMode bool) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 0 {
		t.Fatalf("status database must be nonempty: %d", info.Size())
	}
	var value, marker string
	if jsonMode {
		var status struct {
			Database struct {
				Size json.RawMessage `json:"size"`
			} `json:"database"`
		}
		if err := json.Unmarshal([]byte(payload), &status); err != nil {
			t.Fatal(err)
		}
		var size int64
		if err := json.Unmarshal(status.Database.Size, &size); err != nil || size != info.Size() {
			t.Fatalf("status size must be integer bytes matching database: got %s, want %d (error %v)", status.Database.Size, info.Size(), err)
		}
		value = fmt.Sprintf(`"size":%d`, size)
		marker = `"size":<database-bytes>`
	} else {
		value = fmt.Sprintf("  Size: %.2f MB\n", float64(info.Size())/1024/1024)
		marker = "  Size: <database-megabytes> MB\n"
	}
	if strings.Count(payload, value) != 1 {
		t.Fatalf("expected exactly one status size %q: %s", value, payload)
	}
	return strings.Replace(payload, value, marker, 1)
}

func TestRequiredOutputProcessSmoke(t *testing.T) {
	testEnv(t)
	file, err := os.Open(os.DevNull) // intentionally read-only
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := run(file, io.Discard, []string{"config"}); err == nil {
		t.Fatal("run accepted read-only stdout")
	}
	if code := realMain(file, io.Discard, []string{"config"}); code == 0 {
		t.Fatal("process entrypoint returned zero for read-only stdout")
	}
}

func TestRequiredMutationOutputFailures(t *testing.T) {
	for _, args := range [][]string{
		{"annotate", "--uuid", "output-anchor", "--kind", "correction", "--label", "accepted"},
		{"annotate", "--path", "/output/session.jsonl", "--ordinal", "0", "--kind", "correction", "--label", "accepted"},
		{"rebuild"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			path, _ := testEnv(t)
			seedOutputBranches(t, path)
			good := &outputFailureWriter{}
			if err := run(good, io.Discard, args); err != nil {
				t.Fatal(err)
			}
			if good.calls == 0 {
				t.Fatal("missing mutation output")
			}
			for i := 1; i <= good.calls; i++ {
				if err := run(&outputFailureWriter{failAt: i}, io.Discard, args); !errors.Is(err, errRequiredOutput) {
					t.Errorf("write %d: %v", i, err)
				}
				root := buildRootCmd(&outputFailureWriter{failAt: i}, io.Discard)
				root.SetArgs(args)
				if err := root.Execute(); !errors.Is(err, errRequiredOutput) {
					t.Errorf("handler write %d: %v", i, err)
				}
			}
		})
	}
}

func TestRequiredRecoveryReportFailures(t *testing.T) {
	path, _ := testEnv(t)
	cfg := &config.Config{DatabasePath: path, SessionDirs: []string{filepath.Join(filepath.Dir(path), "empty-inputs")}}
	report := recovery.Report{ActivePath: "active", BackupPath: "backup", InputCounts: []int{2, 3}, ExactDuplicates: 1, FinalCount: 4, Shapes: []compat.SchemaShape{{AppliedVersion: 16, Signature: "shape"}}, Conflicts: []compat.Diagnostic{{Code: "conflict", Summary: " summary "}}}
	for _, dryRun := range []bool{false, true} {
		execute := func(w io.Writer) error {
			root := buildRootCmdWithDependencies(w, io.Discard, func(context.Context, io.Writer, startupCommandClass) startupResult { return startupResult{Config: cfg} }, newStartupSyncService(), func(context.Context, recovery.Options) (recovery.Report, error) { return report, nil })
			args := []string{"recover", "--from", "stranded"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			root.SetArgs(args)
			return root.Execute()
		}
		good := &outputFailureWriter{}
		if err := execute(good); err != nil {
			t.Fatal(err)
		}
		heading := "recovery complete\n"
		if dryRun {
			heading = "recovery dry run\n"
		}
		want := heading + "active path: active\nreplacement target: active\nbackup path: backup\ninput 1 records: 2\ninput 2 records: 3\nexact duplicates: 1\nfinal count: 4\ninput 1 shape: version=16 signature=shape\ndiagnostic: conflict: summary\n"
		if good.String() != want {
			t.Fatalf("recovery payload: %q", good.String())
		}
		for i := 1; i <= good.calls; i++ {
			if err := execute(&outputFailureWriter{failAt: i}); !errors.Is(err, errRequiredOutput) {
				t.Errorf("dryRun=%t write %d: %v", dryRun, i, err)
			}
		}
	}
}

// Cobra's help callback has no error return; the embedded run boundary must
// retain the actual stdout error rather than accept Help's nil result.
func TestRequiredInformationalOutputFailures(t *testing.T) {
	testEnv(t)
	for _, args := range [][]string{{"--help"}, {"help", "search"}, {"recover", "--help"}, {"--version"}, {"--skill"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			file, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := run(file, io.Discard, args); err == nil {
				t.Fatal("informational output accepted read-only stdout")
			}
		})
	}
}

func TestRequiredInvalidValidationOutputFailures(t *testing.T) {
	path, _ := testEnv(t)
	seedOutputBranches(t, path)
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM indexed_files`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, jsonMode := range []bool{false, true} {
		mode := outputText
		if jsonMode {
			mode = outputJSON
		}
		cfg := &config.Config{DatabasePath: path}
		var out bytes.Buffer
		if err := runValidate(context.Background(), &out, io.Discard, cfg, mode); err == nil {
			t.Fatal("orphaned index validated")
		}
		if out.Len() == 0 {
			// Text compatibility refusals belong on stderr and already fail.
			if jsonMode {
				t.Fatal("JSON validation refusal omitted its payload")
			}
			continue
		}
		if err := runValidate(context.Background(), &outputFailureWriter{failAt: 1}, io.Discard, cfg, mode); !errors.Is(err, errRequiredOutput) {
			t.Fatalf("json=%t: %v", jsonMode, err)
		}
	}
}

func TestRequiredDeclarativeOutputFailures(t *testing.T) {
	path, _ := testEnv(t)
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "output.inputs.toml"), fmt.Sprintf(`version = 1
[[inputs]]
id = "output-documents"
source = "spec"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.md"]
exclude = ["**/ignored.md"]
[inputs.decode]
format = "markdown_document"
`, filepath.Dir(path)))
	for _, args := range [][]string{{"config"}, {"config", "--json"}, {"status"}, {"status", "--json"}} {
		good := &outputFailureWriter{}
		if err := run(good, io.Discard, args); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(good.String(), "output-documents") {
			t.Fatal("declarative branch not exercised")
		}
		for i := 1; i <= good.calls; i++ {
			if err := run(&outputFailureWriter{failAt: i}, io.Discard, args); !errors.Is(err, errRequiredOutput) {
				t.Errorf("%v write %d: %v", args, i, err)
			}
		}
	}
}
