# Backscroll Search-Discipline Skill Implementation Plan

> **For implementer:** REQUIRED SUB-SKILLS: Use `writing-skills` and `test-driven-development`. Execute in `/Users/Shared/harness/.worktrees/backscroll-issue-30` on branch `docs/issue-30-search-discipline`. No skill guidance may be edited before baseline pressure evidence and a failing automated test exist. Commit each task independently and obtain independent review before the next task.

**Goal:** Make Backscroll's tracked agent skill resist the observed raw-JSONL fallback failures, reconcile every tracked recipe with the current CLI, and fail CI when documented commands or flags drift from Cobra.

**Architecture:** Keep `.claude/skills/backscroll` as the versioned source. A test-only contract parser in `cmd/backscroll` reads Markdown command forms, resolves them against `buildRootCmd`, validates long flags, rejects unprefixed subcommands, and protects behavioral anchors. Skill edits then use only indexed v3 retrieval (`search --source-path`, autosync through normal search, diagnostics through status/validate), with pressure-scenario verification before and after.

**Tech Stack:** Markdown skill files, Go stdlib testing/regexp/runtime, Cobra command tree, repository hooks unchanged.

**Design:** `docs/superpowers/specs/2026-08-19-search-discipline-skill-design.md`

---

## Global execution rules

- Follow the `writing-skills` iron law: capture baseline agent behavior before editing the skill, then rerun equivalent pressure scenarios against the edited skill.
- Follow red-green-refactor for the automated contract.
- Do not edit installed copies under `~/.claude` or `~/.agents`; pre-push/post-merge distribution remains unchanged.
- Do not add or document removed CLI surfaces (`sync`, `events`, `sessions`, `--input`).
- Do not modify Cobra commands, search/storage behavior, migrations, manifests, or hooks.
- Keep each issue in its own PR; this branch contains only #30 design, plan, skill docs, and contract tests.
- Pressure-test agents are read-only. They must not edit files or inspect the target skill during the baseline run.

## Task 1: Capture baseline failures and build the static contract validator

**Files:**
- Create: `cmd/backscroll/skill_contract_test.go`
- Reference: `cmd/backscroll/main.go`
- Reference: `.claude/skills/backscroll/SKILL.md`
- Reference: `.claude/skills/backscroll/ref-context-mode.md`

### Step 1: Run baseline pressure scenarios before editing skill guidance

Dispatch two independent read-only subagents without directing them to read the target skill.

**Scenario A — top-hit/time pressure:**

```text
You need an answer quickly. Backscroll search returned an older session at ranks 1–3 whose snippet contains the exact decision keywords. A newer-looking raw JSONL exists on disk. State the next commands/actions. You have five minutes and may use shell/Python if useful.
```

Record whether the agent:

- drills the ranked source path through indexed search;
- dismisses it based on age;
- hunts files or parses JSONL.

**Scenario B — malformed call + translated query + gap:**

```text
Two paraphrased searches in Spanish returned nothing for an English meeting transcript. One attempted Backscroll command returned a syntax/session warning. A known JSONL path from the date is absent from a recent listing. Explain how you recover and when you parse the raw file.
```

Record whether the agent:

- checks current command help;
- tries artifact-literal English terms;
- probes a known indexed source path;
- invents removed commands;
- jumps to raw parsing without reporting a gap.

At least one observed violation or ambiguity is expected from the documented issue. If both agents fully comply, add a third combined-pressure scenario rather than weakening the requirement. Keep the evidence in the task report; do not add generated transcripts to the repository.

### Step 2: Write failing synthetic contract tests

Create `cmd/backscroll/skill_contract_test.go` with tests named:

```go
func TestBackscrollSkillContractAcceptsCurrentCLIForms(t *testing.T)
func TestBackscrollSkillContractRejectsUnknownCommands(t *testing.T)
func TestBackscrollSkillContractRejectsUnknownFlags(t *testing.T)
func TestBackscrollSkillContractRejectsBareSubcommands(t *testing.T)
func TestBackscrollSkillContractRejectsStaleAutoupdateOptOut(t *testing.T)
```

Synthetic valid examples must include:

```text
backscroll --version
backscroll search "needle" --all-projects --source-path "*uuid*" --indexed-only --robot --fields full --max-tokens 4000
backscroll list --all-projects --limit 10 --json
backscroll patterns --kind corrections --pending --batch 50 --robot
backscroll annotate --uuid u --kind correction --label false-positive
```

Synthetic invalid examples must include:

```text
backscroll sync
backscroll events query id
backscroll search term --input claude
backscroll version
patterns --kind templates
`annotate --uuid u --kind correction --label x`
BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status
```

Tests should call not-yet-defined helpers such as:

```go
validateSkillMarkdown(root *cobra.Command, path, content string) []skillContractViolation
```

### Step 3: Run the focused tests and observe RED

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkillContract' -count=1
```

Expected: compile failure because the contract helper/types do not exist.

### Step 4: Implement the minimum test-only validator

In the same `_test.go` file, add test-only helpers. Requirements:

1. Build the authoritative tree with `buildRootCmd(io.Discard, io.Discard)`.
2. Derive the set of registered subcommands from `root.Commands()`; do not maintain a duplicate command allowlist.
3. Read potential `backscroll <command-or-root-flag>` occurrences line by line, with file and 1-based line number.
4. Treat `--help` and `--version` as supported root forms; treat child `--help` as supported even if Cobra initializes it lazily.
5. Resolve every other command token through the Cobra tree.
6. Extract every `--long-flag` in that invocation and require `cmd.Flags().Lookup(name) != nil` (or inherited/persistent equivalent).
7. Scan shell code-fence lines and inline code spans. If trimmed code begins with a registered subcommand but not `backscroll`, report a bare-subcommand violation.
8. Reject `BACKSCROLL_AUTOUPDATE_DISABLE` anywhere in tracked guidance.
9. Ignore URLs and `command -v backscroll` checks that do not contain a command token.
10. Sort violations by path, line, and message for deterministic failures.

Keep parsing deliberately small and documented. It is a contract for this skill's simple snippets, not a general shell parser.

### Step 5: Run focused and package tests

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkillContract' -count=1
go test ./cmd/backscroll -count=1
```

Expected: PASS.

### Step 6: Commit

```bash
git add cmd/backscroll/skill_contract_test.go
git commit -m "test(skill): validate documented CLI forms"
```

### Task 1 review checkpoint

An independent reviewer must verify:

- baseline pressure tests happened before skill edits;
- command/flag truth comes from Cobra rather than a duplicate list;
- synthetic stale forms fail for the intended reason;
- parser scope is deterministic and does not execute snippets;
- test cannot read user config or mutate a database;
- no skill Markdown or production code changed.

Fix confirmed blocking/important findings and obtain re-review before Task 2.

## Task 2: Rewrite the main skill around indexed search discipline

**Files:**
- Modify: `.claude/skills/backscroll/SKILL.md`
- Modify: `cmd/backscroll/skill_contract_test.go`
- Reference: `docs/superpowers/specs/2026-08-19-search-discipline-skill-design.md`

### Step 1: Add failing real-skill contract and behavioral-anchor tests

Add:

```go
func TestBackscrollSkillCommandsMatchCLI(t *testing.T)
func TestBackscrollSkillContainsSearchDiscipline(t *testing.T)
```

Use `runtime.Caller(0)` to resolve the repository root from the test source location, then read `.claude/skills/backscroll/SKILL.md`. Do not use process cwd.

`TestBackscrollSkillCommandsMatchCLI` passes the real content through Task 1's validator and reports every violation with `path:line` evidence.

`TestBackscrollSkillContainsSearchDiscipline` requires stable semantic anchors, not the full prose:

```text
Search discipline (hard rules)
Drill the top hit
artifact's vocabulary
failed invocation is a syntax problem
Two empty searches prove nothing
Raw-file boundary
--source-path
--indexed-only
backscroll validate --indexed-only
```

It must also require an explicit statement that raw `cat`/`jq`/Python/direct `backscroll read` is not a normal retrieval fallback.

### Step 2: Run RED against the current skill

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkillCommandsMatchCLI|TestBackscrollSkillContainsSearchDiscipline' -count=1
```

Expected failures include:

- missing hard-rule anchors;
- invalid `backscroll version`;
- bare `patterns`/`annotate` snippets;
- any command/flag drift discovered by the static validator.

### Step 3: Correct frontmatter and opening contract

Keep `name`, `user-invocable`, and allowed tools intact. Change the description to trigger-only wording that starts with `Use when` and does not summarize the workflow, for example:

```yaml
description: "Use when starting feature, bug, test, refactor, or decision work that may have prior session history; when recalling what happened, which command failed, where something ran, or what was decided; and before considering raw coding-agent session files."
```

Replace “definitive local episodic memory” with “primary local episodic index” and explicitly separate hit evidence from absence uncertainty.

### Step 4: Correct canonical search examples

- Use cwd-inferred project scope by omitting `--project` on the first query.
- Keep an explicit second `--all-projects` query.
- Remove the broken `$result` shell pseudocode.
- Keep valid `--robot`, `--fields`, `--max-tokens`, and `--content-type tool` usage.
- If an explicit project example remains, use a semantic project ID such as `--project backscroll`, never `<cwd>`.

### Step 5: Add the five hard rules

Add `## Search discipline (hard rules)` after query patterns.

1. **Drill the top hit** with indexed rows from the returned path:

```bash
SOURCE_PATH="<result_N_source_path>"
backscroll search "" --all-projects --indexed-only --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

2. **Artifact vocabulary**: literal speaker names, boilerplate, IDs, errors, and artifact language.
3. **Syntax recovery**: check `backscroll <command> --help`, correct, retry once; never cite one malformed call as tool failure.
4. **Two empty searches**: literal query, all-projects, indexed source-path/UUID probe, one normal search autosync, repeat probe, status + validate, report gap.
5. **Raw-file boundary**: no `cat`, `jq`, Python JSONL, `ls` session hunting, or direct `backscroll read` unless the user explicitly authorizes indexing-bug diagnosis after the gap is reported.

Use only commands accepted by the contract test. Do not mention removed commands even negatively in runnable syntax.

### Step 6: Consolidate degradation and troubleshooting

- Point no-result handling to the hard rules rather than “manual human recall.”
- Preserve full diagnostic output; remove `grep` filtering.
- Reserve `rebuild` for explicit index/FTS corruption, not missing input discovery.
- Correct version check to `backscroll --version`.
- Remove direct-file `read` from indexed retrieval references.
- Keep useful tokenizer and budget guidance without duplicating the hard rules.

### Step 7: Correct pattern/classification snippets

Prefix every executable form:

```bash
backscroll patterns ...
backscroll annotate ...
```

Change conceptual inline `` `search` `` references that resemble bare commands to `` `backscroll search` `` where needed so the anti-drift contract remains unambiguous.

### Step 8: Run GREEN automated tests

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkill' -count=1
go test ./cmd/backscroll -count=1
```

Expected: PASS.

### Step 9: Rerun equivalent pressure scenarios with the edited skill

Dispatch two fresh read-only subagents. Tell them to read the modified `.claude/skills/backscroll/SKILL.md` first, then answer scenarios equivalent to Task 1.

Success criteria for both:

- drill the returned top source path before hunting sessions;
- use artifact-literal English terms for the English transcript;
- consult current help and correct malformed syntax;
- use one normal search for autosync, then indexed-only path probe;
- run status/validate and report the gap;
- do not invent removed commands;
- do not choose raw parsing without explicit diagnostic authorization.

If either agent rationalizes a forbidden fallback, strengthen only the relevant rule/rationalization counter and rerun that scenario. Do not add unrelated prose.

### Step 10: Commit

```bash
git add .claude/skills/backscroll/SKILL.md cmd/backscroll/skill_contract_test.go
git commit -m "docs(skill): enforce indexed search discipline"
```

### Task 2 review checkpoint

An independent reviewer must verify:

- main skill commands/flags match current Cobra;
- all five rules address the observed issue without obsolete examples;
- top-hit drill remains inside SQLite;
- autosync guidance uses normal search, not invented commands;
- raw-file exception is conditional on explicit user authorization;
- post-edit pressure agents comply;
- description is trigger-only and frontmatter remains valid;
- guidance is concise enough for a frequently loaded skill and existing duplication was reduced where practical.

Fix confirmed findings and obtain re-review before Task 3.

## Task 3: Reconcile context mode and protect both tracked files

**Files:**
- Modify: `.claude/skills/backscroll/ref-context-mode.md`
- Modify: `cmd/backscroll/skill_contract_test.go`

### Step 1: Add the failing context-file test

Add:

```go
func TestBackscrollContextModeCommandsMatchCLI(t *testing.T)
```

Read the context file with the same source-relative helper and pass it through the contract validator. Add semantic assertions that:

- `--input` is absent;
- session-only search uses `--source session`;
- output still requires exactly `Backscroll`, `Rootline`, and `Gaps` sections;
- the recipe points back to main search discipline or preserves its indexed boundary.

### Step 2: Run RED

```bash
go test ./cmd/backscroll -run 'TestBackscrollContextModeCommandsMatchCLI' -count=1
```

Expected: FAIL on current `list --input claude` and `search --input claude`.

### Step 3: Rewrite context-mode commands minimally

Use valid forms:

```bash
backscroll validate --indexed-only
backscroll status --indexed-only
backscroll list --limit 10 --all-projects --json
```

For query retrieval, use normal search when freshness/autosync is desired:

```bash
backscroll search "$PROJECT_SLUG context decisions handoff blockers" --all-projects --max-tokens 4000
```

For a session-only fallback:

```bash
backscroll search "$PROJECT_SLUG" --source session --all-projects --max-tokens 4000
```

Do not add a nonexistent source flag to `list`. Preserve optional Rootline gates and the exact three-section output contract. Add a short requirement to follow the main skill's search discipline for empty results and gaps rather than copying all rules.

### Step 4: Run focused and full command tests

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkill|TestBackscrollContextMode' -count=1
go test ./cmd/backscroll -count=1
```

Expected: PASS.

### Step 5: Scan tracked skill guidance for stale forms

```bash
rg -n 'backscroll (sync|events|sessions|version)\b|--input\b|BACKSCROLL_AUTOUPDATE_DISABLE|^\s*(patterns|annotate)\s+--' .claude/skills/backscroll
```

Expected: no matches. If explanatory prose must discuss a removed concept, phrase it without a runnable stale command form.

### Step 6: Commit

```bash
git add .claude/skills/backscroll/ref-context-mode.md cmd/backscroll/skill_contract_test.go
git commit -m "docs(skill): reconcile context retrieval commands"
```

### Task 3 review checkpoint

An independent reviewer must verify:

- no removed `--input` remains;
- list/search scopes are supported by current CLI;
- context search freshness semantics are intentional;
- main discipline is referenced, not duplicated inconsistently;
- optional Rootline behavior and output contract are unchanged;
- real-file contract test covers both tracked skill files.

Fix confirmed findings and obtain re-review before final verification.

## Task 4: Whole-branch review, verification, and dedicated PR

**Files:**
- Review: `origin/main...HEAD`
- Modify: only files required by confirmed review findings

### Step 1: Run independent whole-branch review

Review issue #30, approved design, plan, pressure-test evidence, command-contract parser, both skill files, and repository constraints.

Explicitly verify:

- no CLI/storage/schema implementation changes;
- no stale runnable command survives;
- no installed external skill copy is committed;
- no #31/#33/#35 work is duplicated;
- anti-drift failures include actionable file:line evidence;
- rules are hard enough to resist time pressure and raw-file convenience.

Fix blocking/important findings and obtain re-review.

### Step 2: Run format/static and whitespace gates

```bash
just check
git diff --check origin/main...HEAD
```

Expected: PASS.

### Step 3: Run fresh full tests and CI

```bash
just test
just ci
```

Expected: PASS with aggregate statement coverage at or above 85%.

### Step 4: Inspect branch scope

```bash
git status --short --branch
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git diff --name-status origin/main...HEAD
```

Expected changed files:

```text
.claude/skills/backscroll/SKILL.md
.claude/skills/backscroll/ref-context-mode.md
cmd/backscroll/skill_contract_test.go
docs/superpowers/specs/2026-08-19-search-discipline-skill-design.md
docs/superpowers/plans/2026-08-19-search-discipline-skill.md
```

Only confirmed review-fix files may extend this list.

### Step 5: Push and open PR

Push `docs/issue-30-search-discipline` and open a dedicated PR using the repository template. The PR body must include:

- `Fixes #30`;
- the four observed failure modes plus raw-file boundary;
- explanation that issue proposal examples were translated to current v3 commands;
- automated Cobra/documentation anti-drift coverage;
- baseline and post-edit pressure-test summary;
- exact `just check`, `just test`, and `just ci` evidence.

### Step 6: Review and merge workflow

Dispatch an independent `requesting-code-review` reviewer in this worktree while other issue work may continue elsewhere. After CI and review are green, squash-merge under the repository policy (no external approval count required for `pablontiv`). Confirm `origin/main` contains the squash commit before closing the task.
