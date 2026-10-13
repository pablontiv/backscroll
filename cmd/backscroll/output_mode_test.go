package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputModeRejectsDualFlagsBeforeStartup(t *testing.T) {
	for _, argv := range [][]string{{"list"}, {"search", "--text", "needle"}, {"patterns", "--kind", "commands"}, {"context", "--uuid", "u"}} {
		t.Run(argv[0], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			called := false
			root := buildRootCmdWithDependencies(&stdout, &stderr, func(_ context.Context, _ io.Writer, _ startupCommandClass) startupResult {
				called = true
				return startupResult{}
			}, nil, nil)
			root.SetArgs(append(argv, "--json", "--robot"))
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("error=%v", err)
			}
			if called || stdout.Len() != 0 {
				t.Fatalf("startup=%v stdout=%q", called, stdout.String())
			}
		})
	}
}

func testOutputMode(json, robot bool) outputMode {
	mode, err := resolveOutputMode(json, robot)
	if err != nil {
		panic(err)
	}
	return mode
}

func TestOutputModeEmptyAndManifestRefusal(t *testing.T) {
	for _, argv := range [][]string{{"list", "--all-projects"}, {"search", "--text", "unfindable", "--all-projects"}, {"patterns", "--kind", "commands", "--all-projects"}, {"context", "--uuid", "missing"}, {"status"}, {"validate"}, {"config"}} {
		modes := []string{"--json"}
		if argv[0] == "list" || argv[0] == "search" || argv[0] == "patterns" || argv[0] == "context" {
			modes = append(modes, "--robot")
		}
		for _, flag := range modes {
			t.Run(argv[0]+flag, func(t *testing.T) {
				_, cleanup := testEnv(t)
				defer cleanup()
				var stdout, stderr bytes.Buffer
				root := buildRootCmd(&stdout, &stderr)
				root.SetArgs(append(append([]string{}, argv...), flag))
				err := root.Execute()
				if argv[0] == "context" {
					if err == nil {
						t.Fatal("missing context succeeded")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if flag == "--json" && argv[0] == "list" && stdout.String() != "{\"count\":0,\"sessions\":[]}\n" {
					t.Fatalf("changed list envelope %q", stdout.String())
				}
				if flag == "--json" && !json.Valid(stdout.Bytes()) {
					t.Fatalf("invalid JSON %q", stdout.String())
				}
				if flag == "--robot" && argv[0] != "context" && stdout.Len() != 0 {
					t.Fatalf("empty robot output=%q", stdout.String())
				}
				if argv[0] == "list" && stderr.Len() == 0 {
					t.Fatal("missing list hint")
				}
				inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
				if err := os.MkdirAll(inputs, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(inputs, "broken.inputs.toml"), []byte("invalid = ["), 0600); err != nil {
					t.Fatal(err)
				}
				stdout.Reset()
				stderr.Reset()
				root = buildRootCmd(&stdout, &stderr)
				root.SetArgs(append(append([]string{}, argv...), flag))
				if err := root.Execute(); err == nil {
					t.Fatal("malformed manifest succeeded")
				}
				if flag == "--json" && !json.Valid(stdout.Bytes()) {
					t.Fatalf("invalid refusal JSON %q", stdout.String())
				}
				if flag == "--robot" && !strings.Contains(stdout.String(), "code=") {
					t.Fatalf("invalid robot refusal %q", stdout.String())
				}
			})
		}
	}
}

func TestOutputModeHelpAndExplicitFalse(t *testing.T) {
	for _, name := range []string{"list", "search", "patterns", "context"} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := buildRootCmd(&stdout, &stderr)
			root.SetArgs([]string{name, "--help"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), "mutually exclusive with --robot") || !strings.Contains(stdout.String(), "mutually exclusive with --json") {
				t.Fatalf("help=%q", stdout.String())
			}
		})
	}
	for _, tc := range []struct {
		json, robot bool
		mode        outputMode
	}{{false, false, outputText}, {true, false, outputJSON}, {false, true, outputRobot}} {
		got, err := resolveOutputMode(tc.json, tc.robot)
		if err != nil || got != tc.mode {
			t.Fatalf("mode=%v error=%v", got, err)
		}
	}
}
