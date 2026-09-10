package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const queryEchoText = "durable snapshot orchard protocol"

type queryEchoResult struct {
	FilePath    string
	Content     string
	ContentType string
	Rank        int
}

type queryEchoE2E struct {
	t           *testing.T
	root        string
	binary      string
	fixtures    string
	config      string
	database    string
	environment []string
}

func TestDirectBackscrollSearchEchoesDoNotCrowdUnfilteredRecall(t *testing.T) {
	e := newQueryEchoE2E(t)
	corePaths := e.writeCoreProse()
	e.run("status", "--json")

	baseline := e.searchJSON(queryEchoText, "", 20)
	wantBaseline := []string{
		"decoy-2.jsonl:text:1",
		"decoy-1.jsonl:text:2",
		"decoy-0.jsonl:text:3",
		"target.jsonl:text:4",
	}
	if got := queryEchoShape(baseline); !reflect.DeepEqual(got, wantBaseline) {
		t.Fatalf("four-prose baseline must put target at rank 4\ngot:  %v\nwant: %v", got, wantBaseline)
	}
	baselineBudgets := e.budgetReachability(queryEchoText, []int{150, 160, 170, 180, 190, 200, 210})
	wantBaselineBudgets := map[int]bool{150: false, 160: true, 170: true, 180: true, 190: true, 200: true, 210: true}
	if !reflect.DeepEqual(baselineBudgets, wantBaselineBudgets) {
		t.Fatalf("unexpected no-echo budget baseline: got %v want %v", baselineBudgets, wantBaselineBudgets)
	}

	corePaths = append(corePaths, e.writeDirectEchoes()...)
	e.run("status", "--json")

	unfiltered := e.searchJSON(queryEchoText, "", 20)
	if got := queryEchoShape(unfiltered); !reflect.DeepEqual(got, wantBaseline) {
		t.Errorf("direct Backscroll query echoes changed unfiltered baseline; target rank=%d\ngot:  %v\nwant: %v", queryEchoRank(unfiltered, "target.jsonl"), got, wantBaseline)
	}
	if topFive := e.searchJSON(queryEchoText, "", 5); queryEchoRank(topFive, "target.jsonl") == 0 {
		t.Errorf("historical target was crowded out of top five: %v", queryEchoShape(topFive))
	}
	if textOnly := queryEchoShape(e.searchJSON(queryEchoText, "text", 20)); !reflect.DeepEqual(textOnly, wantBaseline) {
		t.Errorf("text-only behavior changed: got %v want %v", textOnly, wantBaseline)
	}
	wantEchoes := []string{"echo-0.jsonl", "echo-1.jsonl", "echo-2.jsonl"}
	if got := queryEchoFiles(e.searchJSON(queryEchoText, "tool", 20)); !reflect.DeepEqual(got, wantEchoes) {
		t.Errorf("explicit tool search must retain exact echo identities: got %v want %v", got, wantEchoes)
	}
	if got := e.budgetReachability(queryEchoText, []int{150, 160, 170, 180, 190, 200, 210}); !reflect.DeepEqual(got, wantBaselineBudgets) {
		t.Errorf("query echoes changed bounded baseline, including focal budget 180: got %v want %v", got, wantBaselineBudgets)
	}
	if repeat := queryEchoShape(e.searchJSON(queryEchoText, "", 20)); !reflect.DeepEqual(repeat, wantBaseline) {
		t.Errorf("repeat must preserve corrected deterministic order: got %v want %v", repeat, wantBaseline)
	}

	e.writeControls()
	e.run("status", "--json")
	e.assertControlBoundaries()

	if got := e.countCoreRows(corePaths); got != 7 {
		t.Fatalf("core records before source removal = %d, want 7", got)
	}
	for _, path := range corePaths {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove owned fixture %s: %v", path, err)
		}
	}
	e.run("status", "--json")
	if got := e.countCoreRows(corePaths); got != 7 {
		t.Errorf("perennial core records after source removal = %d, want 7", got)
	}
	if got := queryEchoShape(e.searchJSON(queryEchoText, "", 20)); !reflect.DeepEqual(got, wantBaseline) {
		t.Errorf("corrected recall changed after source expiry: got %v want %v", got, wantBaseline)
	}
	if got := queryEchoFiles(e.searchJSON(queryEchoText, "tool", 20)); !reflect.DeepEqual(got, wantEchoes) {
		t.Errorf("explicit tool recall changed after source expiry: got %v want %v", got, wantEchoes)
	}
	e.assertIntegrity()
}

func TestDirectBackscrollSearchEchoesRefillToolCandidateWindow(t *testing.T) {
	e := newQueryEchoE2E(t)
	query := "overflow scarlet zebra"
	legitimatePath := e.writeRecord("overflow-legitimate.jsonl", "overflow-legitimate", "overflow", 0,
		queryEchoToolBlock("overflow-legitimate", "rg '"+query+"' /synthetic"), false)
	e.run("status", "--json")

	if got := queryEchoRank(e.searchJSON(query, "", 300), filepath.Base(legitimatePath)); got != 1 {
		t.Fatalf("no-echo legitimate command rank = %d, want 1", got)
	}

	echoPath := filepath.Join(e.fixtures, "overflow-echoes.jsonl")
	for i := 0; i < 199; i++ {
		e.writeRecord(filepath.Base(echoPath), fmt.Sprintf("overflow-echo-%03d", i), "overflow", 1+i,
			queryEchoToolBlock(fmt.Sprintf("overflow-echo-%03d", i), "backscroll search --text '"+query+"' --robot"), i > 0)
	}
	e.run("status", "--json")

	exactPage := e.searchJSON(query, "tool", 300)
	if len(exactPage) != 200 || queryEchoRank(exactPage, filepath.Base(legitimatePath)) != 200 {
		t.Fatalf("exact candidate page: count=%d legitimate_rank=%d, want 200/200", len(exactPage), queryEchoRank(exactPage, filepath.Base(legitimatePath)))
	}
	if got := e.searchJSON(query, "", 300); len(got) != 1 || queryEchoRank(got, filepath.Base(legitimatePath)) != 1 {
		t.Fatalf("exact candidate page must preserve legitimate command once: %v", queryEchoShape(got))
	}

	for i := 199; i < 205; i++ {
		e.writeRecord(filepath.Base(echoPath), fmt.Sprintf("overflow-echo-%03d", i), "overflow", 1+i,
			queryEchoToolBlock(fmt.Sprintf("overflow-echo-%03d", i), "backscroll search --text '"+query+"' --robot"), true)
	}
	e.run("status", "--json")

	overflowTool := e.searchJSON(query, "tool", 300)
	if len(overflowTool) != 206 || queryEchoRank(overflowTool, filepath.Base(legitimatePath)) != 206 {
		t.Fatalf("overflow explicit tool results: count=%d legitimate_rank=%d, want 206/206", len(overflowTool), queryEchoRank(overflowTool, filepath.Base(legitimatePath)))
	}
	unfiltered := e.searchJSON(query, "", 300)
	if len(unfiltered) != 1 || queryEchoRank(unfiltered, filepath.Base(legitimatePath)) != 1 {
		t.Errorf("205 echoes hid the unrelated rank-206 command: %v", queryEchoShape(unfiltered))
	}
	if repeat := e.searchJSON(query, "", 300); !reflect.DeepEqual(repeat, unfiltered) {
		t.Errorf("overflow query repeat changed: first=%v repeat=%v", queryEchoShape(unfiltered), queryEchoShape(repeat))
	}
	if got := e.countCoreRows([]string{legitimatePath, echoPath}); got != 206 {
		t.Errorf("overflow stored rows = %d, want 206", got)
	}
	e.assertIntegrity()
}

func newQueryEchoE2E(t *testing.T) *queryEchoE2E {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "fixtures"),
		filepath.Join(root, "config", "backscroll", "inputs"),
		filepath.Join(root, "home"),
		filepath.Join(root, "xdg-data"),
		filepath.Join(root, "xdg-cache"),
		filepath.Join(root, "xdg-state"),
		filepath.Join(root, "go-cache"),
		filepath.Join(root, "go-tmp"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create isolated E2E path %s: %v", path, err)
		}
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "backscroll")
	build := exec.Command("go", "build", "-mod=readonly", "-buildvcs=false", "-ldflags", "-X main.version=dev", "-o", binary, "./cmd/backscroll/")
	build.Dir = repoRoot
	build.Env = queryEchoOverrideEnv(os.Environ(), map[string]string{
		"GOCACHE":     filepath.Join(root, "go-cache"),
		"GOPROXY":     "off",
		"GOTELEMETRY": "off",
		"GOTMPDIR":    filepath.Join(root, "go-tmp"),
		"TMPDIR":      filepath.Join(root, "go-tmp"),
	})
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual Backscroll CLI: %v\n%s", err, output)
	}

	config := filepath.Join(root, "config")
	fixtures := filepath.Join(root, "fixtures")
	database := filepath.Join(root, "index.db")
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = %q\nsource = %q\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\nexclude = []\n[inputs.decode]\nformat = \"claude\"\n", "query-echo-e2e", "session", fixtures)
	if err := os.WriteFile(filepath.Join(config, "backscroll", "inputs", "query-echo.inputs.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := queryEchoOverrideEnv(os.Environ(), map[string]string{
		"HOME":                     filepath.Join(root, "home"),
		"XDG_CONFIG_HOME":          config,
		"XDG_DATA_HOME":            filepath.Join(root, "xdg-data"),
		"XDG_CACHE_HOME":           filepath.Join(root, "xdg-cache"),
		"XDG_STATE_HOME":           filepath.Join(root, "xdg-state"),
		"BACKSCROLL_CONFIG_DIR":    config,
		"BACKSCROLL_DATABASE_PATH": database,
		"BACKSCROLL_SESSION_DIRS":  "",
		"GOTELEMETRY":              "off",
		"NO_COLOR":                 "1",
		"TMPDIR":                   filepath.Join(root, "go-tmp"),
		"TZ":                       "UTC",
	})
	return &queryEchoE2E{t: t, root: root, binary: binary, fixtures: fixtures, config: config, database: database, environment: environment}
}

func (e *queryEchoE2E) writeCoreProse() []string {
	e.t.Helper()
	var paths []string
	for i := 0; i < 3; i++ {
		content := "We asked about " + queryEchoText + ". No answer yet. " + strings.Repeat("Unrelated planning notes. ", 10+i*10)
		paths = append(paths, e.writeRecord(fmt.Sprintf("decoy-%d.jsonl", i), fmt.Sprintf("decoy-%d", i), fmt.Sprintf("decoy-%d", i), i, content, false))
	}
	paths = append(paths, e.writeRecord("target.jsonl", "target", "historical-target", 3, queryEchoText+": readers use the last committed SQLite snapshot.", false))
	return paths
}

func (e *queryEchoE2E) writeDirectEchoes() []string {
	e.t.Helper()
	commands := []string{
		"backscroll search --text '" + queryEchoText + "' --robot",
		"backscroll search --text '" + queryEchoText + "' --robot --fields minimal",
		"backscroll search --text '" + queryEchoText + "' --all-projects",
	}
	paths := make([]string, 0, len(commands))
	for i, command := range commands {
		paths = append(paths, e.writeRecord(fmt.Sprintf("echo-%d.jsonl", i), fmt.Sprintf("echo-%d", i), fmt.Sprintf("echo-%d", i), 4+i, queryEchoToolBlock(fmt.Sprintf("echo-%d", i), command), false))
	}
	return paths
}

func (e *queryEchoE2E) writeControls() {
	e.t.Helper()
	samePath := e.writeRecord("same-session-control.jsonl", "same-legit", "same-session", 10, "same session legitimate harbor records a real decision before retrieval.", false)
	e.appendRecord(samePath, "same-query", "same-session", 11, queryEchoToolBlock("same-query", "backscroll search --text 'same session legitimate harbor' --robot"))
	e.writeRecord("mention-control.jsonl", "mention", "mention", 12, "Backscroll search documentation mentions the prose-only mention control prism without being a command.", false)
	e.writeRecord("unrelated-tool.jsonl", "unrelated", "unrelated", 13, queryEchoToolBlock("unrelated", "rg 'unrelated tool compass' /synthetic"), false)
	wrappers := map[string]string{
		"absolute-wrapper.jsonl": "/private/tmp/bin/backscroll search --text 'wrapper echo nebula' --robot",
		"env-wrapper.jsonl":      "env backscroll search --text 'wrapper echo nebula' --robot",
		"shell-wrapper.jsonl":    "bash -lc \"backscroll search --text 'wrapper echo nebula' --robot\"",
	}
	names := []string{"absolute-wrapper.jsonl", "env-wrapper.jsonl", "shell-wrapper.jsonl"}
	for i, name := range names {
		e.writeRecord(name, strings.TrimSuffix(name, ".jsonl"), "wrappers", 14+i, queryEchoToolBlock(name, wrappers[name]), false)
	}
}

func (e *queryEchoE2E) assertControlBoundaries() {
	e.t.Helper()
	if got := queryEchoShape(e.searchJSON("same session legitimate harbor", "", 20)); !reflect.DeepEqual(got, []string{"same-session-control.jsonl:text:1"}) {
		e.t.Errorf("same-session legitimate prose boundary: got %v", got)
	}
	if got := queryEchoFiles(e.searchJSON("same session legitimate harbor", "tool", 20)); !reflect.DeepEqual(got, []string{"same-session-control.jsonl"}) {
		e.t.Errorf("same-session direct query must remain tool-searchable: got %v", got)
	}
	if got := queryEchoFiles(e.searchJSON("unrelated tool compass", "", 20)); !reflect.DeepEqual(got, []string{"unrelated-tool.jsonl"}) {
		e.t.Errorf("unrelated command changed in unfiltered search: got %v", got)
	}
	if got := queryEchoFiles(e.searchJSON("unrelated tool compass", "tool", 20)); !reflect.DeepEqual(got, []string{"unrelated-tool.jsonl"}) {
		e.t.Errorf("unrelated command changed in tool search: got %v", got)
	}
	mentionText := queryEchoFiles(e.searchJSON("Backscroll search mention control prism", "text", 20))
	if !reflect.DeepEqual(mentionText, []string{"mention-control.jsonl"}) {
		e.t.Errorf("Backscroll mention prose changed in text search: got %v", mentionText)
	}
	if got := queryEchoFiles(e.searchJSON("Backscroll search mention control prism", "", 20)); !containsQueryEchoFile(got, "mention-control.jsonl") {
		e.t.Errorf("Backscroll mention prose missing from unfiltered search: got %v", got)
	}
	wantWrappers := []string{"absolute-wrapper.jsonl", "env-wrapper.jsonl", "shell-wrapper.jsonl"}
	if got := queryEchoFiles(e.searchJSON("wrapper echo nebula", "", 20)); !reflect.DeepEqual(got, wantWrappers) {
		e.t.Errorf("wrapper behavior changed in unfiltered search: got %v want %v", got, wantWrappers)
	}
	if got := queryEchoFiles(e.searchJSON("wrapper echo nebula", "tool", 20)); !reflect.DeepEqual(got, wantWrappers) {
		e.t.Errorf("wrapper behavior changed in tool search: got %v want %v", got, wantWrappers)
	}
}

func (e *queryEchoE2E) writeRecord(name, uuid, session string, minute int, content any, appendFile bool) string {
	e.t.Helper()
	path := filepath.Join(e.fixtures, name)
	record := map[string]any{
		"uuid": uuid, "timestamp": fmt.Sprintf("2026-01-01T00:%02d:00Z", minute),
		"sessionId": session, "cwd": "/synthetic/query-echo-e2e", "type": "assistant",
		"message": map[string]any{"role": "assistant", "content": content},
	}
	data, err := json.Marshal(record)
	if err != nil {
		e.t.Fatal(err)
	}
	flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendFile {
		flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		e.t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		e.t.Fatal(err)
	}
	return path
}

func (e *queryEchoE2E) appendRecord(path, uuid, session string, minute int, content any) {
	e.t.Helper()
	e.writeRecord(filepath.Base(path), uuid, session, minute, content, true)
}

func queryEchoToolBlock(id, command string) []map[string]any {
	return []map[string]any{{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": command}}}
}

func (e *queryEchoE2E) run(args ...string) string {
	e.t.Helper()
	cmd := exec.Command(e.binary, args...)
	cmd.Dir = e.root
	cmd.Env = e.environment
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		e.t.Fatalf("Backscroll %v: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr.String())
	}
	return string(stdout)
}

func (e *queryEchoE2E) searchJSON(query, contentType string, limit int) []queryEchoResult {
	e.t.Helper()
	args := []string{"search", "--text", query, "--all-projects", "--json", "--fields", "full", "--max-tokens", "0", "--limit", fmt.Sprint(limit)}
	if contentType != "" {
		args = append(args, "--content-type", contentType)
	}
	var results []queryEchoResult
	if err := json.Unmarshal([]byte(e.run(args...)), &results); err != nil {
		e.t.Fatalf("decode Backscroll JSON for %q: %v", query, err)
	}
	return results
}

func (e *queryEchoE2E) budgetReachability(query string, budgets []int) map[int]bool {
	e.t.Helper()
	result := make(map[int]bool, len(budgets))
	for _, budget := range budgets {
		output := e.run("search", "--text", query, "--all-projects", "--robot", "--fields", "minimal", "--limit", "20", "--max-tokens", fmt.Sprint(budget))
		result[budget] = strings.Contains(output, string(filepath.Separator)+"target.jsonl")
	}
	return result
}

func (e *queryEchoE2E) countCoreRows(paths []string) int {
	e.t.Helper()
	db, err := sql.Open("sqlite", "file:"+e.database+"?mode=ro")
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	count := 0
	for _, path := range paths {
		var pathCount int
		if err := db.QueryRow("SELECT count(*) FROM search_items WHERE source_path = ?", path).Scan(&pathCount); err != nil {
			e.t.Fatal(err)
		}
		count += pathCount
	}
	return count
}

func (e *queryEchoE2E) assertIntegrity() {
	e.t.Helper()
	db, err := sql.Open("sqlite", "file:"+e.database+"?mode=ro")
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		e.t.Fatal(err)
	}
	if integrity != "ok" {
		e.t.Errorf("SQLite integrity_check = %q, want ok", integrity)
	}
}

func queryEchoShape(results []queryEchoResult) []string {
	shape := make([]string, 0, len(results))
	for _, result := range results {
		shape = append(shape, fmt.Sprintf("%s:%s:%d", filepath.Base(result.FilePath), result.ContentType, result.Rank))
	}
	return shape
}

func queryEchoRank(results []queryEchoResult, name string) int {
	for _, result := range results {
		if filepath.Base(result.FilePath) == name {
			return result.Rank
		}
	}
	return 0
}

func queryEchoFiles(results []queryEchoResult) []string {
	files := make([]string, 0, len(results))
	for _, result := range results {
		files = append(files, filepath.Base(result.FilePath))
	}
	sort.Strings(files)
	return files
}

func containsQueryEchoFile(files []string, want string) bool {
	for _, file := range files {
		if file == want {
			return true
		}
	}
	return false
}

func queryEchoOverrideEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[key]; !replaced && !strings.HasPrefix(key, "BACKSCROLL_") {
			out = append(out, entry)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
