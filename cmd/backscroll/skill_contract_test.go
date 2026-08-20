package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBackscrollSkillContractAcceptsCurrentCLIForms(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := strings.Join([]string{
		"backscroll --version",
		"backscroll --help",
		"backscroll search --help",
		"command -v backscroll >/dev/null",
		"backscroll search \"needle\" --all-projects --source-path \"*uuid*\" --indexed-only --robot --fields full --max-tokens 4000",
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

func TestBackscrollSkillContractRejectsStaleAutoupdateOptOut(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	content := "BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status"

	violations := validateSkillMarkdown(root, "synthetic-stale-autoupdate.md", content)
	assertSkillContractViolations(t, violations,
		"synthetic-stale-autoupdate.md:1: stale autoupdate opt-out BACKSCROLL_AUTOUPDATE_DISABLE is not supported",
	)
}

func assertSkillContractViolations(t *testing.T, got []skillContractViolation, want ...string) {
	t.Helper()
	gotText := strings.TrimSpace(formatSkillContractViolations(got))
	wantText := strings.Join(want, "\n")
	if gotText != wantText {
		t.Fatalf("unexpected violations:\nwant:\n%s\n\ngot:\n%s", wantText, gotText)
	}
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

		// This is intentionally not a general shell parser. The skill uses simple
		// one-command snippets, so we tokenize enough Markdown code/prose to catch
		// documented `backscroll ...` invocations without executing anything.
		violations = append(violations, validateBackscrollInvocations(root, path, lineNumber, line)...)
		violations = append(violations, validateBareSubcommand(path, lineNumber, line, registeredSubcommands)...)
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
		if token != "backscroll" || isURLToken(token) || isCommandVBackscroll(tokens, i) {
			continue
		}

		args := invocationArgs(tokens[i+1:])
		cmd := root
		cmdName := root.Name()
		flagArgs := args
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			child := findSubcommand(root, args[0])
			if child == nil {
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
	}
	return violations
}

func validateBareSubcommand(path string, lineNumber int, text string, registeredSubcommands map[string]struct{}) []skillContractViolation {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "backscroll ") || strings.HasPrefix(trimmed, "command -v backscroll") {
		return nil
	}
	trimmed = strings.TrimPrefix(trimmed, "$ ")
	trimmed = strings.TrimPrefix(trimmed, "> ")
	tokens := shellishFields(trimmed)
	if len(tokens) == 0 {
		return nil
	}
	command := tokens[0]
	if _, ok := registeredSubcommands[command]; !ok {
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
