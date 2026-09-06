package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type options struct {
	bin, evalSet, dataset, fixtureRoot, sourceSHA string
	limit                                         int
	verbose                                       bool
}

type commandClient struct {
	bin string
	env []string
	dir string
}

func main() { os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr)) }

func runMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("recall-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.bin, "backscroll", "", "path to dev backscroll binary")
	fs.StringVar(&o.evalSet, "eval-set", "docs/eval/queries.toml", "TOML evaluation set")
	fs.StringVar(&o.dataset, "dataset", "legacy", "legacy or synthetic")
	fs.StringVar(&o.fixtureRoot, "fixture-root", "docs/eval/fixtures/recall", "synthetic fixture root")
	fs.StringVar(&o.sourceSHA, "source-sha", "unknown", "evaluated source SHA")
	fs.IntVar(&o.limit, "limit", 0, "maximum cases after dataset selection")
	fs.BoolVar(&o.verbose, "verbose", false, "print per-case evidence")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if o.bin == "" || (o.dataset != "legacy" && o.dataset != "synthetic") || o.limit < 0 {
		_, _ = fmt.Fprintln(stderr, "--backscroll is required; --dataset must be legacy or synthetic; --limit must be non-negative")
		return 2
	}
	set, err := loadEvalSet(o.evalSet)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	queries := selectQueries(set.Queries, o.dataset, o.limit)
	if len(queries) == 0 {
		_, _ = fmt.Fprintln(stderr, "no queries selected")
		return 2
	}
	if err := validateQueries(queries, o.dataset); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	fixtureRoot := ""
	workDir := ""
	env := os.Environ()
	cleanup := func() {}
	digest := "operator-backed"
	if o.dataset == "synthetic" {
		prepared, err := prepareSyntheticEnvironment(o.fixtureRoot)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		fixtureRoot, env, cleanup, digest = prepared.FixtureRoot, prepared.Env, prepared.Cleanup, prepared.FixtureDigest
		workDir = prepared.WorkDir
		defer cleanup()
	}
	client := commandClient{bin: o.bin, env: env, dir: workDir}
	status := client.execute([]string{"status", "--json"})
	if status.Err != nil || status.ExitCode != 0 {
		_, _ = fmt.Fprintf(stderr, "status failed (exit %d): %s\n", status.ExitCode, strings.TrimSpace(status.Stderr))
		return 2
	}
	var statusJSON struct {
		Index struct {
			TotalFiles    int `json:"total_files"`
			TotalMessages int `json:"total_messages"`
		} `json:"index"`
	}
	if err := json.Unmarshal([]byte(status.Stdout), &statusJSON); err != nil {
		_, _ = fmt.Fprintln(stderr, "invalid status JSON:", err)
		return 2
	}
	if statusJSON.Index.TotalFiles < 1 {
		_, _ = fmt.Fprintln(stderr, "index is empty; no evaluation can be performed")
		return 2
	}
	_, _ = fmt.Fprintf(stdout, "Dataset: %s\nCases: %d\nIndex: %d files, %d messages\nSource SHA: %s\nFixture root: %s\nFixture tree SHA-256: %s\n", o.dataset, len(queries), statusJSON.Index.TotalFiles, statusJSON.Index.TotalMessages, o.sourceSHA, fixtureRoot, digest)
	runner := func(query string, flags []string, fields string, maxTokens int) commandResult {
		return client.execute(searchArgs(query, flags, fields, maxTokens))
	}
	return runEvaluation(queries, fixtureRoot, runner, o.verbose, o.dataset == "legacy", 80, stdout)
}

func searchArgs(query string, flags []string, fields string, maxTokens int) []string {
	args := []string{"search", "--text", query}
	args = append(args, flags...)
	joined := " " + strings.Join(flags, " ") + " "
	if !strings.Contains(joined, " --all-projects ") && !strings.Contains(joined, " --project ") {
		args = append(args, "--all-projects")
	}
	return append(args, "--robot", "--fields", fields, "--limit", "20", "--max-tokens", strconv.Itoa(maxTokens))
}

func (c commandClient) execute(args []string) commandResult {
	cmd := exec.Command(c.bin, args...)
	cmd.Env = c.env
	if c.dir != "" {
		cmd.Dir = c.dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return commandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode, Err: err}
}
