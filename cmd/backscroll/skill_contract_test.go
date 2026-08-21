package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// livingCLIContractDocs is the explicit current user/operator documentation
// boundary. Historical records under docs/roadmap, docs/superpowers,
// docs/research, docs/eval/corrections-labeling-*.md, docs/backlog.md, and
// docs/intention-agentic-input-definitions.md are intentionally excluded: they
// preserve decisions, evaluations, or planned surfaces from earlier releases.
var livingCLIContractDocs = []string{
	"README.md",
	"docs/audit-integration.md",
	"docs/configuration.md",
	"docs/eval/README.md",
	"docs/eval/corrections-calibration.md",
	"docs/input-contract.md",
	"docs/patterns.md",
	"docs/read.md",
	"docs/search.md",
	"docs/sync.md",
}

func TestBackscrollSkillContractAcceptsCurrentCLIForms(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		"backscroll --version",
		"backscroll --help",
		"backscroll search --help",
		"command -v backscroll >/dev/null",
		"backscroll search \"needle\" --all-projects --source-path \"*uuid*\" --robot --fields full --max-tokens 4000",
		"backscroll list --all-projects --limit 10 --json",
		"backscroll patterns --kind corrections --pending --batch 50 --robot",
		"backscroll annotate --uuid u --kind correction --label false-positive",
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-valid.md", content)
	if len(violations) > 0 {
		t.Fatalf("expected current CLI forms to pass, got violations:\n%s", formatSkillContractViolations(violations))
	}
}

func TestBackscrollSkillContractRejectsUnknownCommands(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		"backscroll sync",
		"backscroll events query id",
		"backscroll version",
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-unknown-commands.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-unknown-commands.md:1: unknown backscroll command \"sync\"",
		"synthetic-unknown-commands.md:2: unknown backscroll command \"events\"",
		"synthetic-unknown-commands.md:3: unknown backscroll command \"version\"",
	)
}

func TestBackscrollSkillContractRejectsUnknownFlags(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := "backscroll search term --input claude"

	violations := validateSkillMarkdown(root, "synthetic-unknown-flags.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-unknown-flags.md:1: unknown flag --input for backscroll search",
	)
}

func TestBackscrollSkillContractRequiresSearchQueryText(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		`backscroll search --source-path "*/session.jsonl" --all-projects --json`,
		`backscroll search --source-path "*/session.jsonl" --text --all-projects --json`,
		`backscroll search --all-projects --limit 5`,
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-queryless-search.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-queryless-search.md:1: search invocation lacks query text (use --text <query> or positional query)",
		"synthetic-queryless-search.md:2: search invocation lacks query text (use --text <query> or positional query)",
		"synthetic-queryless-search.md:3: search invocation lacks query text (use --text <query> or positional query)",
	)
}

func TestBackscrollSkillContractAcceptsSearchQueryText(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		`backscroll search "artifact literal" --source-path "*/session.jsonl" --all-projects --json`,
		`backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --robot --fields full`,
		`backscroll search --source-path "*/session.jsonl" --text "artifact literal" --limit 5`,
		`backscroll search --help`,
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-search-with-query.md", content)
	if len(violations) > 0 {
		t.Fatalf("expected search invocations with query text to pass, got violations:\n%s", formatSkillContractViolations(violations))
	}
}

func TestBackscrollSkillContractDetectsRemovedReadInvocation(t *testing.T) {
	content := strings.Join([]string{
		"The literal `backscroll read` names the removed command in narrative prose.",
		"backscroll read /tmp/session.jsonl",
	}, "\n")

	if !containsBackscrollReadInvocation(content) {
		t.Fatal("expected actual backscroll read invocation to be detected")
	}
	if containsBackscrollReadInvocation("The literal `backscroll read` names the removed command in narrative prose.") {
		t.Fatal("narrative literal backscroll read mention must not be treated as an invocation")
	}
}

func TestBackscrollSkillContractRejectsBareSubcommands(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		"patterns --kind templates",
		"`annotate --uuid u --kind correction --label x`",
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-bare-subcommands.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-bare-subcommands.md:1: bare backscroll subcommand \"patterns\"; prefix with backscroll",
		"synthetic-bare-subcommands.md:2: bare backscroll subcommand \"annotate\"; prefix with backscroll",
	)
}

func TestBackscrollSkillContractAcceptsNarrativeCommandMentions(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		"That answers a question `search` cannot.",
		"`search` retrieves while `patterns` computes a census.",
		"backscroll computes complete, reproducible censuses for an agent.",
		"`--json` is available on `search`, `list`, and `status`.",
		"config directory before running a query command.",
		"`backscroll list` does not support `--source-path`.",
	}, "\n")

	violations := validateSkillMarkdown(root, "synthetic-narrative.md", content)
	if len(violations) > 0 {
		t.Fatalf("expected narrative command mentions to pass, got violations:\n%s", formatSkillContractViolations(violations))
	}
}

func TestBackscrollSkillContractRejectsStaleAutoupdateOptOut(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := "BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status"

	violations := validateSkillMarkdown(root, "synthetic-stale-autoupdate.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-stale-autoupdate.md:1: stale autoupdate opt-out BACKSCROLL_AUTOUPDATE_DISABLE is not supported",
	)
}

func TestBackscrollSkillCommandsMatchCLI(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	path, content := readTrackedSkillMarkdown(t, ".claude/skills/backscroll/SKILL.md")

	violations := validateSkillMarkdown(root, path, content)
	if len(violations) > 0 {
		t.Fatalf("documented backscroll commands must match the Cobra CLI:\n%s", formatSkillContractViolations(violations))
	}
}

func TestBackscrollLivingDocsMatchCLI(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	var violations []skillContractViolation
	for _, relativePath := range livingCLIContractDocs {
		path, content := readTrackedSkillMarkdown(t, relativePath)
		violations = append(violations, validateSkillMarkdown(root, path, content)...)
	}
	if len(violations) > 0 {
		t.Fatalf("living documentation commands must match the Cobra CLI:\n%s", formatSkillContractViolations(violations))
	}
}

func TestBackscrollSkillContainsSearchDiscipline(t *testing.T) {
	_, content := readTrackedSkillMarkdown(t, ".claude/skills/backscroll/SKILL.md")

	anchors := []string{
		"Search discipline (hard rules)",
		"Drill the top hit",
		"artifact's vocabulary",
		"failed invocation is a syntax problem",
		"Two empty searches prove nothing",
		"Raw-file boundary",
		"--source-path",
		"mandatory startup sync",
		"backscroll validate",
	}
	for _, anchor := range anchors {
		if !strings.Contains(content, anchor) {
			t.Errorf("missing search-discipline anchor %q", anchor)
		}
	}

	if strings.Contains(content, "--indexed-only") {
		t.Error("shipped skill must not document removed --indexed-only flag")
	}
	if containsBackscrollReadInvocation(content) {
		t.Error("shipped skill must not invoke removed backscroll read command")
	}

	rawBoundaryAnchors := []string{
		"cat",
		"jq",
		"Python",
		"filesystem session hunting",
		"not a normal retrieval fallback",
	}
	for _, anchor := range rawBoundaryAnchors {
		if !strings.Contains(content, anchor) {
			t.Errorf("missing raw-file boundary anchor %q", anchor)
		}
	}
}

func TestBackscrollContextModeCommandsMatchCLI(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	path, content := readTrackedSkillMarkdown(t, ".claude/skills/backscroll/ref-context-mode.md")

	violations := validateSkillMarkdown(root, path, content)
	if len(violations) > 0 {
		t.Fatalf("documented context-mode backscroll commands must match the Cobra CLI:\n%s", formatSkillContractViolations(violations))
	}

	if strings.Contains(content, "--input") {
		t.Error("context mode must not document removed --input flags")
	}
	assertSourceSessionOnlyOnSearch(t, content)
	assertExactContextOutputSections(t, content)
	if !strings.Contains(content, "main skill's search discipline") && !strings.Contains(content, "indexed boundary") {
		t.Error("context mode must point to the main search discipline or preserve the indexed boundary")
	}
}

func containsBackscrollReadInvocation(content string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if startsWithBackscrollRead(shellishFields(withoutInlineCodeSpans(line))) {
			return true
		}
		for _, span := range inlineCodeSpans(line) {
			if startsWithBackscrollRead(shellishFields(span)) {
				return true
			}
		}
	}
	return false
}

func startsWithBackscrollRead(tokens []string) bool {
	for i, token := range tokens {
		if token == "backscroll" && i+2 < len(tokens) && tokens[i+1] == "read" && isInvocationStart(tokens, i) {
			return true
		}
	}
	return false
}

func assertSourceSessionOnlyOnSearch(t *testing.T, content string) {
	t.Helper()
	found := false
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "--source session") {
			continue
		}
		found = true
		if !strings.HasPrefix(trimmed, "backscroll search ") {
			t.Errorf("--source session must be used only on backscroll search, got %q", trimmed)
		}
	}
	if !found {
		t.Error("context mode must include a session-only search with --source session")
	}
}

func assertExactContextOutputSections(t *testing.T, content string) {
	t.Helper()
	want := []string{
		"1. `Backscroll`: relevant sessions/documents and paths.",
		"2. `Rootline`: live records found, or `not available` with the skipped gate.",
		"3. `Gaps`: missing manifests, empty index, absent session-state, or schema/validation errors.",
	}
	lastIndex := -1
	for _, section := range want {
		if strings.Count(content, section) != 1 {
			t.Errorf("context mode must require exactly one output section line %q", section)
			continue
		}
		index := strings.Index(content, section)
		if index <= lastIndex {
			t.Errorf("context output section %q is out of order", section)
		}
		lastIndex = index
	}
}

func assertSkillContractViolations(t *testing.T, got []skillContractViolation, want ...string) {
	t.Helper()
	gotText := strings.TrimSpace(formatSkillContractViolations(got))
	wantText := strings.Join(want, "\n")
	if gotText != wantText {
		t.Fatalf("unexpected violations:\nwant:\n%s\n\ngot:\n%s", wantText, gotText)
	}
}

func readTrackedSkillMarkdown(t *testing.T, relativePath string) (string, string) {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	content, err := os.ReadFile(filepath.Join(repoRoot, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return relativePath, string(content)
}

type skillContractViolation struct {
	path    string
	line    int
	message string
}

func validateSkillMarkdown(root *cobra.Command, path, content string) []skillContractViolation {
	registeredSubcommands := backscrollSubcommands(root)
	var violations []skillContractViolation
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lineNumber := i + 1
		if strings.Contains(line, "BACKSCROLL_AUTOUPDATE_DISABLE") {
			violations = append(violations, skillContractViolation{
				path:    path,
				line:    lineNumber,
				message: "stale autoupdate opt-out BACKSCROLL_AUTOUPDATE_DISABLE is not supported",
			})
		}

		// This is intentionally not a general shell parser. We tokenize enough
		// Markdown code/prose to catch documented invocations without executing
		// anything. Inline spans are checked separately so surrounding prose flags
		// cannot be attributed to an inline command mention.
		prose := withoutInlineCodeSpans(line)
		violations = append(violations, validateBackscrollInvocations(root, path, lineNumber, prose)...)
		violations = append(violations, validateBareSubcommand(path, lineNumber, prose, registeredSubcommands)...)
		for _, span := range inlineCodeSpans(line) {
			violations = append(violations, validateBackscrollInvocations(root, path, lineNumber, span)...)
			violations = append(violations, validateBareSubcommand(path, lineNumber, span, registeredSubcommands)...)
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].path != violations[j].path {
			return violations[i].path < violations[j].path
		}
		if violations[i].line != violations[j].line {
			return violations[i].line < violations[j].line
		}
		return violations[i].message < violations[j].message
	})
	return uniqueSkillContractViolations(violations)
}

func validateBackscrollInvocations(root *cobra.Command, path string, lineNumber int, text string) []skillContractViolation {
	tokens := shellishFields(text)
	var violations []skillContractViolation
	for i, token := range tokens {
		if token != "backscroll" || isURLToken(token) || isCommandVBackscroll(tokens, i) || !isInvocationStart(tokens, i) {
			continue
		}

		args := invocationArgs(tokens[i+1:])
		cmd := root
		cmdName := root.Name()
		flagArgs := args
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			child := findSubcommand(root, args[0])
			if child == nil {
				// General documentation also uses "backscroll" as the project name
				// in prose. Long, flag-free phrases are narrative, not shell snippets.
				if len(args) > 3 && len(longFlags(args)) == 0 {
					continue
				}
				violations = append(violations, skillContractViolation{
					path:    path,
					line:    lineNumber,
					message: fmt.Sprintf("unknown backscroll command %q", args[0]),
				})
				continue
			}
			cmd = child
			cmdName = root.Name() + " " + child.Name()
			flagArgs = args[1:]
		}

		for _, flagName := range longFlags(flagArgs) {
			if isSupportedFlag(cmd, flagName) {
				continue
			}
			violations = append(violations, skillContractViolation{
				path:    path,
				line:    lineNumber,
				message: fmt.Sprintf("unknown flag --%s for %s", flagName, cmdName),
			})
		}
		if cmd.Name() == "search" && !searchInvocationHasQuery(cmd, flagArgs) {
			violations = append(violations, skillContractViolation{
				path:    path,
				line:    lineNumber,
				message: "search invocation lacks query text (use --text <query> or positional query)",
			})
		}
	}
	return violations
}

func searchInvocationHasQuery(cmd *cobra.Command, tokens []string) bool {
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if isShellTerminator(token) {
			return false
		}
		if token == "--help" || token == "-h" {
			return true
		}
		if token == "--" {
			return firstNonFlagToken(tokens[i+1:]) != ""
		}
		if strings.HasPrefix(token, "--") {
			name, value, hasValue := splitLongFlag(token)
			if name == "text" && hasValue {
				return strings.TrimSpace(value) != ""
			}
			if flagConsumesNext(cmd, name) {
				if i+1 < len(tokens) {
					if name == "text" {
						return !strings.HasPrefix(tokens[i+1], "--") && strings.TrimSpace(tokens[i+1]) != "" && !isShellTerminator(tokens[i+1])
					}
					i++
				}
			}
			continue
		}
		if strings.HasPrefix(token, "-") {
			continue
		}
		return strings.TrimSpace(token) != ""
	}
	return false
}

func firstNonFlagToken(tokens []string) string {
	for _, token := range tokens {
		if isShellTerminator(token) {
			return ""
		}
		if strings.HasPrefix(token, "-") {
			continue
		}
		if strings.TrimSpace(token) != "" {
			return token
		}
	}
	return ""
}

func splitLongFlag(token string) (name, value string, hasValue bool) {
	name = strings.TrimPrefix(token, "--")
	if before, after, ok := strings.Cut(name, "="); ok {
		return before, after, true
	}
	return name, "", false
}

func flagConsumesNext(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(name)
	}
	if flag == nil {
		flag = cmd.PersistentFlags().Lookup(name)
	}
	return flag != nil && flag.NoOptDefVal == ""
}

func validateBareSubcommand(path string, lineNumber int, text string, registeredSubcommands map[string]struct{}) []skillContractViolation {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "backscroll ") || strings.HasPrefix(trimmed, "command -v backscroll") {
		return nil
	}
	trimmed = strings.TrimPrefix(trimmed, "$ ")
	trimmed = strings.TrimPrefix(trimmed, "> ")
	tokens := shellishFields(trimmed)
	if len(tokens) < 2 {
		return nil
	}
	command := tokens[0]
	if _, ok := registeredSubcommands[command]; !ok || !strings.HasPrefix(tokens[1], "-") {
		return nil
	}
	return []skillContractViolation{{
		path:    path,
		line:    lineNumber,
		message: fmt.Sprintf("bare backscroll subcommand %q; prefix with backscroll", command),
	}}
}

func backscrollSubcommands(root *cobra.Command) map[string]struct{} {
	commands := make(map[string]struct{})
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = struct{}{}
	}
	return commands
}

func findSubcommand(root *cobra.Command, name string) *cobra.Command {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name || cmd.HasAlias(name) {
			return cmd
		}
	}
	return nil
}

func invocationArgs(tokens []string) []string {
	for i, token := range tokens {
		if isShellTerminator(token) {
			return tokens[:i]
		}
	}
	return tokens
}

func isShellTerminator(token string) bool {
	switch token {
	case "|", "||", "&&", ";", "#":
		return true
	default:
		return strings.HasPrefix(token, "#")
	}
}

func isInvocationStart(tokens []string, index int) bool {
	if index == 0 {
		return true
	}
	previous := tokens[index-1]
	if previous == "$" || previous == ">" || isShellTerminator(previous) {
		return true
	}
	for _, token := range tokens[:index] {
		if !isShellAssignment(token) {
			return false
		}
	}
	return true
}

func isShellAssignment(token string) bool {
	name, _, ok := strings.Cut(token, "=")
	if !ok || name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func longFlags(tokens []string) []string {
	var flags []string
	for _, token := range tokens {
		if !strings.HasPrefix(token, "--") || token == "--" {
			continue
		}
		name := strings.TrimPrefix(token, "--")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		name = strings.TrimRight(name, ",.;:)]}")
		if name != "" {
			flags = append(flags, name)
		}
	}
	return flags
}

func isSupportedFlag(cmd *cobra.Command, name string) bool {
	if name == "help" {
		return true
	}
	if cmd.Parent() == nil && name == "version" {
		return true
	}
	return cmd.Flags().Lookup(name) != nil ||
		cmd.InheritedFlags().Lookup(name) != nil ||
		cmd.PersistentFlags().Lookup(name) != nil
}

func isCommandVBackscroll(tokens []string, index int) bool {
	return index >= 2 && tokens[index-2] == "command" && tokens[index-1] == "-v"
}

func isURLToken(token string) bool {
	return strings.Contains(token, "://")
}

func withoutInlineCodeSpans(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "```") {
		return line
	}
	var out strings.Builder
	for {
		start := strings.Index(line, "`")
		if start == -1 {
			out.WriteString(line)
			return out.String()
		}
		out.WriteString(line[:start])
		line = line[start+1:]
		end := strings.Index(line, "`")
		if end == -1 {
			out.WriteString(line)
			return out.String()
		}
		out.WriteByte(' ')
		line = line[end+1:]
	}
}

func inlineCodeSpans(line string) []string {
	if strings.HasPrefix(strings.TrimSpace(line), "```") {
		return nil
	}
	var spans []string
	for {
		start := strings.Index(line, "`")
		if start == -1 {
			return spans
		}
		line = line[start+1:]
		end := strings.Index(line, "`")
		if end == -1 {
			return spans
		}
		spans = append(spans, line[:end])
		line = line[end+1:]
	}
}

func shellishFields(text string) []string {
	var fields []string
	var current strings.Builder
	var quote rune
	for _, r := range text {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == '\t' || r == ' ':
			if current.Len() > 0 {
				fields = append(fields, cleanShellToken(current.String()))
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, cleanShellToken(current.String()))
	}
	return fields
}

func cleanShellToken(token string) string {
	return strings.Trim(token, "`.,;:()[]{}")
}

func uniqueSkillContractViolations(violations []skillContractViolation) []skillContractViolation {
	if len(violations) < 2 {
		return violations
	}
	unique := violations[:1]
	for _, violation := range violations[1:] {
		last := unique[len(unique)-1]
		if violation == last {
			continue
		}
		unique = append(unique, violation)
	}
	return unique
}

func formatSkillContractViolations(violations []skillContractViolation) string {
	var out strings.Builder
	for _, violation := range violations {
		_, _ = fmt.Fprintf(&out, "%s:%d: %s\n", violation.path, violation.line, violation.message)
	}
	return out.String()
}
