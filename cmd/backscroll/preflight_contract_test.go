package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidInvocationBeforeStartup(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{}
	for _, command := range []string{"status", "validate", "rebuild", "config", "list", "context", "annotate", "recover", "patterns"} {
		cases = append(cases, struct {
			args []string
			want string
		}{[]string{command, "extra"}, "extra"})
	}
	for _, command := range []string{"search", "list", "patterns"} {
		args := []string{command, "--project", "named", "--all-projects"}
		if command == "search" {
			args = append(args, "query")
		}
		if command == "patterns" {
			args = append(args, "--kind", "commands")
		}
		cases = append(cases, struct {
			args []string
			want string
		}{args, "all-projects"})
	}
	for _, value := range []string{"NaN", "+Inf", "-Inf", "-0.1", "1.1"} {
		cases = append(cases, struct {
			args []string
			want string
		}{[]string{"search", "query", "--similarity-threshold", value}, "--similarity-threshold"})
		cases = append(cases, struct {
			args []string
			want string
		}{[]string{"patterns", "--kind", "corrections", "--min-confidence", value}, "--min-confidence"})
	}
	for _, flag := range []string{"min-support", "min-length", "max-length", "batch"} {
		cases = append(cases, struct {
			args []string
			want string
		}{[]string{"patterns", "--kind", "sequences", "--" + flag, "-1"}, "--" + flag})
	}
	cases = append(cases, struct {
		args []string
		want string
	}{[]string{"patterns", "--kind", "sequences", "--min-length", "7"}, "--min-length"})
	for _, flag := range []string{"limit", "offset", "recent"} {
		cases = append(cases, struct {
			args []string
			want string
		}{[]string{"list", "--" + flag, "-1"}, "--" + flag})
	}
	cases = append(cases, struct {
		args []string
		want string
	}{[]string{"search", "query", "--max-tokens", "-1"}, "--max-tokens"})
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "index.db")
			setIndexPolicyEnv(t, dbPath, t.TempDir())
			calls := 0
			var out, errout bytes.Buffer
			root := buildRootCmdWithDependencies(&out, &errout, func(context.Context, io.Writer, startupCommandClass) startupResult {
				calls++
				return startupResult{Failure: &startupFailure{Cause: errors.New("startup reached")}}
			}, nil, nil)
			root.SetArgs(c.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error=%v want %q", err, c.want)
			}
			if calls != 0 {
				t.Fatalf("startup calls=%d", calls)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout=%q", out.String())
			}
			out.Reset()
			errout.Reset()
			if code := run(&out, &errout, c.args); code == nil || !strings.Contains(code.Error(), c.want) {
				t.Fatalf("actual CLI error=%v stderr=%q", code, errout.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("side effects=%v error=%v", entries, err)
			}
			for _, surface := range []string{"HOME", "BACKSCROLL_CONFIG_DIR", "BACKSCROLL_SESSION_DIRS"} {
				entries, err := os.ReadDir(os.Getenv(surface))
				if err != nil || len(entries) != 0 {
					t.Fatalf("%s side effects=%v error=%v", surface, entries, err)
				}
			}
		})
	}
}

// Boundary flags must not rewrite the historical empty-index JSON payloads.
func TestPreflightBoundariesPreservePayloads(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"search", "needle", "--all-projects", "--similarity-threshold", "0", "--max-tokens", "0", "--json"}, "[]"},
		{[]string{"search", "--text", "needle", "--all-projects", "--similarity-threshold", "1", "--limit", "0", "--offset", "0", "--json"}, "[]"},
		{[]string{"patterns", "--kind", "sequences", "--all-projects", "--min-support", "0", "--min-length", "0", "--max-length", "0", "--limit", "0", "--batch", "0", "--json"}, `{"count":0,"kind":"sequences","patterns":[]}`},
		{[]string{"patterns", "--kind", "corrections", "--all-projects", "--min-confidence", "0", "--json"}, `{"count":0,"kind":"corrections","patterns":[]}`},
		{[]string{"patterns", "--kind", "corrections", "--all-projects", "--min-confidence", "1", "--json"}, `{"count":0,"kind":"corrections","patterns":[]}`},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			setIndexPolicyEnv(t, filepath.Join(t.TempDir(), "index.db"), t.TempDir())
			var out, errout bytes.Buffer
			if err := run(&out, &errout, c.args); err != nil {
				t.Fatalf("run: %v stderr=%q", err, errout.String())
			}
			if strings.TrimSpace(out.String()) != c.want {
				t.Fatalf("payload=%q want %q", out.String(), c.want)
			}
		})
	}
}
