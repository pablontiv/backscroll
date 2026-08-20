# Backscroll Search-Discipline Skill Design

**Date:** 2026-08-19
**Issue:** [#30 — SKILL.md: add search-discipline guardrails](https://github.com/pablontiv/backscroll/issues/30)

## Summary

Agents with the Backscroll skill loaded have still abandoned indexed retrieval and parsed raw session JSONL after four recoverable moments: dismissing a relevant top-ranked hit, searching with conversational rather than artifact vocabulary, treating one malformed invocation as a tool failure, and encountering an apparent indexing gap.

The repository skill also contains command drift. Its main recipe uses semantically misleading project examples and an invalid version command; its pattern examples omit the `backscroll` executable; and `ref-context-mode.md` uses removed `--input` flags. The issue's paste-ready proposal cannot be copied literally because it references removed `events query` and `sync` commands.

This change will make the tracked skill the canonical, current-CLI search recipe. It adds hard search-discipline rules expressed only with supported v3 commands, reconciles the companion context-mode recipe, and adds a Go contract test that checks documented Backscroll commands and flags against the actual Cobra tree so stale command forms fail CI.

## Goals

- Require agents to inspect a relevant top-ranked result before searching for a different session.
- Require pasted artifacts to be queried with literal vocabulary from the artifact and in the artifact's language.
- Treat the first failed invocation as a possible syntax error and require checking current `--help` before abandoning Backscroll.
- Make two empty searches insufficient evidence that a topic or file is absent.
- Provide a current-CLI procedure for distinguishing query mismatch from an indexing gap.
- Make raw JSONL parsing an explicit exceptional diagnostic boundary, never the normal retrieval fallback.
- Reconcile current command drift in the tracked main skill and `ref-context-mode.md`.
- Add automated anti-drift coverage for command names and flags used by both skill files.
- Deliver the work through a dedicated PR for issue #30.

## Non-goals

- Implementing or restoring `events query`, `sync`, `sessions query`, or any other removed CLI surface.
- Fixing the separate 14 MB indexing gap reported in the issue.
- Changing search ranking, FTS tokenization, autosync, storage, or schema behavior.
- Executing every documented command during tests or reading a user's real index.
- Adding a new skill installation destination or changing pre-push/post-merge distribution hooks.
- Editing untracked external copies under `~/.agents`; `.claude/skills/backscroll` remains the versioned source of truth.
- Rewriting historical roadmap/spec documents that describe older CLI generations.

## Existing Problems

### Search behavior guidance

The current skill says empty results do not prove nonexistence, but it does not tell an agent to:

- drill into the top hit by indexed `source_path`;
- use literal artifact vocabulary;
- recover from malformed syntax;
- verify whether a known source path has any indexed rows;
- report an indexing gap before crossing into raw-file inspection.

Consequently, the agent can follow the broad recall-first intent while still abandoning the indexed path at the exact moments observed in issue #30.

### Main skill command drift

The tracked `.claude/skills/backscroll/SKILL.md` currently contains these weaknesses:

- `--project <cwd-or-inferred>` suggests passing a filesystem path, while project scoping is already inferred from cwd and `--project` expects a project identifier.
- The fallback shell example checks an unassigned `$result` variable.
- `backscroll version` is not a command; the supported form is `backscroll --version`.
- Pattern-discovery examples use bare `patterns` and `annotate`, which are not executables.
- Troubleshooting recommends `rebuild` as a generic answer to zero results even though a missing discovered source is not necessarily FTS corruption.
- “Definitive” memory language overstates what an index can prove when a source was never ingested.

### Context-mode drift

`.claude/skills/backscroll/ref-context-mode.md` uses removed `--input claude` flags on `list` and `search`. The supported source restriction is `--source session` on search; list has no `--input` or `--source` flag.

## Design

### 1. Position Backscroll as primary indexed evidence

Change the opening guarantee from “definitive local episodic memory” to “primary local episodic index” or equivalent. The skill remains mandatory recall-first guidance, but it explicitly states:

- a hit is evidence;
- an empty query result is not evidence that the underlying event or artifact never existed;
- an absent source-path probe is an indexing-gap signal, not permission to silently replace indexed retrieval with raw parsing.

This wording preserves strong use of Backscroll without making false completeness claims.

### 2. Canonical project-first retrieval

The first search relies on Backscroll's existing cwd-based project inference and does not pass a filesystem path to `--project`:

```bash
backscroll search "QUERY" --robot --fields minimal --max-tokens 2000
```

If the result is empty or irrelevant, the agent explicitly runs the all-projects variant:

```bash
backscroll search "QUERY" --all-projects --robot --fields minimal --max-tokens 2000
```

The recipe will not include broken shell pseudocode around an unassigned result variable. It describes the two deterministic steps directly.

Execution-shaped queries continue to use `--content-type tool` and terms of at least three characters because the trigram limitation remains real.

### 3. Search discipline hard rules

Add a dedicated section after query patterns.

#### Rule 1: Drill the top hit

If a top-ranked result has relevant keywords, the agent must inspect the indexed rows from that exact `source_path` before changing session hypotheses. Timestamp or assumptions about session contents are not sufficient reasons to dismiss it.

The supported indexed drill-down is:

```bash
SOURCE_PATH="<result_N_source_path>"
backscroll search "" --all-projects --indexed-only --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

This reads rows already stored in SQLite. It does not invoke direct-file `read` and does not parse source JSONL.

#### Rule 2: Use artifact vocabulary

For pasted transcripts, logs, error dumps, generated reports, and similar artifacts, the agent must query literal strings likely to appear in the artifact:

- speaker/display names;
- platform boilerplate;
- exact identifiers;
- error fragments;
- artifact language rather than the conversation's translated paraphrase.

A conceptual description in another language may be a useful secondary search, but not the only evidence before declaring absence.

#### Rule 3: Recover from malformed syntax

A warning, unknown flag, missing argument, “query required,” or session/path resolution error is treated as a syntax problem until the current command help is checked:

```bash
backscroll search --help
backscroll list --help
```

The agent corrects the invocation and retries once. One malformed invocation never justifies switching to raw files.

The recipe does not mention removed `events query` syntax.

#### Rule 4: Two empty searches prove nothing

Before concluding that content is not indexed, the agent must:

1. retry with one or more artifact-literal terms;
2. broaden from inferred project scope to `--all-projects` when appropriate;
3. if a source path or UUID is known, probe indexed rows directly:

```bash
backscroll search "" --all-projects --indexed-only --source-path "*SESSION-UUID*" --json --fields minimal --limit 1
```

4. run one normal literal search without `--indexed-only`, allowing the existing search autosync to run, then repeat the indexed-only source-path probe;
5. if still absent, run diagnostics:

```bash
backscroll status
backscroll validate --indexed-only
```

6. report the source path, literal probe, search scopes, and diagnostic output as an indexing gap.

The skill never recommends a nonexistent `backscroll sync` command. It also does not prescribe `rebuild` for a missing source; `rebuild` repairs/re-derives an existing index and is not proof that input discovery can see the file.

#### Rule 5: Raw-file boundary

The skill explicitly prohibits normal fallback to `cat`, `jq`, Python JSONL parsing, `ls`-driven session hunting, or direct-file `backscroll read` after retrieval uncertainty.

Raw-file inspection is allowed only when the user explicitly asks to diagnose the indexing bug or authorizes leaving the indexed-evidence boundary. Before doing so, the agent must report the gap and commands already attempted. Raw inspection may diagnose ingestion; it must not silently substitute for indexed recall.

### 4. Consolidate degradation guidance

Update the existing degradation/troubleshooting sections to refer to the hard rules instead of contradicting or duplicating them.

- Stale/locked/unhealthy index: preserve full Backscroll diagnostics rather than piping them through `grep` and losing context.
- No results: follow artifact vocabulary, scope broadening, and source-path probe; do not jump directly to “manual human recall.”
- Suspected missing input: allow one normal search autosync, then indexed-only probe and report.
- FTS corruption: `rebuild` may remain documented only as an explicit corruption remediation, not a generic missing-source action.
- Version check: use `backscroll --version`.
- References: do not present direct-file `read` as the indexed retrieval route.

### 5. Correct command examples

Make all runnable examples use the actual executable:

```text
backscroll patterns ...
backscroll annotate ...
```

Remove the filesystem placeholder from `--project`; examples either use implicit cwd project scope, an explicit semantic project ID such as `--project backscroll`, or `--all-projects`.

Keep valid agent-output flags (`--robot`, `--fields`, `--max-tokens`) and current query filters.

### 6. Reconcile context mode

Update `.claude/skills/backscroll/ref-context-mode.md`:

- remove every `--input claude` use;
- list recent indexed entries with valid `list` flags;
- use `--source session` on the broader search when session-only scope is required;
- keep current status/validate gates and the three-section output contract;
- use normal search where a fresh autosync is desired and `--indexed-only` only when explicitly inspecting the existing snapshot.

The context recipe must obey the same search discipline and may link back to the main skill rather than copy all five rules.

## Automated Anti-drift Contract

### Location

Add `cmd/backscroll/skill_contract_test.go`. The command package already owns the actual Cobra tree through `buildRootCmd`, making it the authoritative place to compare documentation against implemented commands and flags.

The test locates tracked skill files relative to the Go source file using `runtime.Caller`, not the process working directory:

```text
.claude/skills/backscroll/SKILL.md
.claude/skills/backscroll/ref-context-mode.md
```

This keeps it hermetic under `go test`, scrubbed HOME, and arbitrary checkout locations.

### Command validation

Extract lowercase `backscroll <token>` command forms from both Markdown files. For each occurrence:

- `backscroll --version` and `backscroll --help` are validated as supported root forms;
- every command token must match a command registered by `buildRootCmd`;
- `backscroll version`, `backscroll sync`, `backscroll events`, and any future removed command fail automatically because they are absent from the Cobra tree.

The extractor ignores URLs and prose where `backscroll` is not followed by a command token.

### Flag validation

For each documented command invocation, extract `--long-flag` tokens and require that the selected Cobra command exposes each flag. This catches removed forms such as `--input` without maintaining a complete hand-written allowlist.

Shell redirection, comments, quoted query text, variables, and placeholders are not executed. The test statically validates only command/flag registration and therefore cannot access a user's config or database.

### Unprefixed command validation

Inspect shell code-fence lines. If a line begins with a known Backscroll subcommand such as `patterns` or `annotate` but lacks the `backscroll` executable, fail with file and line number.

This catches the current census/classification examples that look executable but are not.

### Explicit invariants

In addition to dynamic Cobra checks, assert that:

- `BACKSCROLL_AUTOUPDATE_DISABLE` does not appear in tracked skill guidance because autoupdate has no runtime opt-out;
- both hard-rule headings/anchor phrases and the source-path probe are present;
- raw-file fallback is explicitly prohibited;
- the context-mode file contains no `--input` token.

These checks protect user-visible behavioral intent that command-tree validation alone cannot express.

## Data Flow

### Ranked hit drill-down

```text
project-scoped search
  -> relevant top hit
  -> capture indexed source_path
  -> empty-query + --indexed-only + --source-path
  -> inspect stored rows from that hit
  -> answer/refine from indexed evidence
```

### Empty-result escalation

```text
empty conversational query
  -> artifact-literal query
  -> all-projects query
  -> known source-path/UUID indexed-only probe
  -> one normal search autosync
  -> repeat indexed-only probe
  -> status + validate
  -> report indexing gap
  -> raw inspection only with explicit diagnostic authorization
```

### CI anti-drift

```text
tracked skill Markdown
  -> extract backscroll command forms + flags
  -> compare with buildRootCmd Cobra tree
  -> reject missing commands/flags and unprefixed subcommands
  -> go test / CI failure with file:line evidence
```

## Error Handling

- Missing tracked skill file: contract test fails with its expected repository path.
- Unreadable skill file: contract test fails with contextual read error.
- Unknown documented command: fail with file, line, and command token.
- Unknown documented flag: fail with file, line, selected command, and flag.
- Complex shell line the static extractor cannot classify: keep the snippet simple rather than weakening validation globally.
- Empty search output: handled by search discipline; not converted into proof of absence.
- Autosync or compatibility diagnostic failure: preserve and report full output; do not serve raw-file-derived recall as if it came from the index.
- Known source absent after retry: report indexing gap; do not run destructive or unrelated remediation automatically.

## Testing

### Contract tests

- Current corrected main skill passes command validation.
- Current corrected context-mode skill passes command validation.
- A table of synthetic snippets proves rejection of:
  - unknown root command;
  - valid command with unknown flag;
  - bare known subcommand without `backscroll` prefix;
  - stale autoupdate opt-out.
- Synthetic valid snippets prove support for root `--version`, search flags, list flags, patterns flags, and annotate flags.
- Required discipline anchors and raw-boundary language are present.

### Regression checks

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkill' -count=1
go test ./cmd/backscroll -count=1
just check
just test
just ci
git diff --check origin/main...HEAD
```

The release-blocking aggregate coverage must remain at or above 85%.

## Documentation and Distribution

The PR changes the versioned source files only:

```text
.claude/skills/backscroll/SKILL.md
.claude/skills/backscroll/ref-context-mode.md
```

Existing pre-push/post-merge hooks remain responsible for copying the versioned skill to `~/.claude/skills/backscroll`. The PR does not edit an external installed copy directly and does not add a second installation mechanism.

`CLAUDE.md` does not require a package-layout update because no Go package is added or removed. It may mention the anti-drift test only if needed for maintenance clarity; otherwise the committed spec and test name are sufficient.

## PR and Integration

Issue #30 receives its own branch and PR. The branch is based on `main` after merged PRs #36 and #37. Required workflow:

1. TDD implementation in the issue #30 worktree.
2. Independent per-task review.
3. Independent whole-branch review.
4. Fresh `just check`, `just test`, and `just ci` evidence.
5. Dedicated PR with `Fixes #30`.
6. After CI and independent review pass, squash-merge using the repository's current protection policy.
