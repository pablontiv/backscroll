# Shipped Guidance Integrity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every `backscroll <verb>` invocation in shipped skills, presets, and distributed documentation resolves and can be exercised safely through the real Cobra tree, while adding four mandatory search-discipline guardrails.

**Architecture:** Build a test-only asset scanner that discovers shipped consumer assets by distribution roots, extracts shell-like Backscroll argv with source locations, classifies examples and destructive invocations, resolves every command/flag against `buildRootCmd`, and executes only through a hermetic safe harness. The validator derives command truth from Cobra and never maintains a command-name allowlist; production compatibility code remains untouched.

**Tech Stack:** Go 1.26.2, Cobra 1.10.2, standard-library Markdown/TOML line scanning and argv lexer, temporary filesystem/config/database harness, table-driven Go tests, Just

**Spec:** `docs/superpowers/specs/2026-08-18-systemic-index-compatibility-design.md`

## Global Constraints

- This plan runs last in the exact chain Plan 1 → Plan 3 → Plan 2 → Plan 4. It depends on Plan 3’s final command names/effects and Plan 2’s repaired manifest presets.
- Scan consumer `.md`, `.toml`, and `.sh` assets under `.claude/skills/**`, `inputs/*.toml`, root `README.md`, and distributed `docs/**/*.md`. Exclude only implementation-history trees `docs/roadmap/**`, `docs/research/**`, and `docs/superpowers/**`; these are contributor records, not shipped operating guidance. `.claude/skills/backscroll-doctor/assets/gather.sh` is shipped and must be discovered.
- Extract `backscroll <verb>` invocations from prose, fenced code, inline code, and TOML comments. Report asset path, 1-based line, parsed argv, and Cobra/execution failure.
- Do not use a hand-maintained command or flag allowlist. Resolve against `buildRootCmd` and each command’s actual Cobra flags.
- Classify angle-bracket/metavariable examples explicitly and replace them with safe fixture values before execution. Never silently skip them after syntax resolution.
- Classify command effects from a canonical Cobra annotation on every root command constructor, not from verb strings. Missing or unknown effects fail closed before execution; there is no default effect. Exercise write/replace commands against disposable temporary data only.
- Never execute shell operators, substitutions, pipelines, redirects, external commands, or arbitrary prose. The scanner validates one parsed Backscroll invocation at a time.
- Every harness execution sets HOME, BACKSCROLL_CONFIG_DIR, and BACKSCROLL_DATABASE_PATH to `t.TempDir()` paths, disables ambient input discovery by supplying temporary manifests, and uses the dev version path so autoupdate performs no network access.
- Four search guardrails are mandatory: inspect ranked hits before concluding; use vocabulary from retrieved artifacts in follow-up queries; treat malformed/failed calls as tool failures rather than evidence of absence; reproduce suspected indexing gaps with a minimal fixture before claiming a gap.
- Issue #30 closes only when discovery, effect completeness/refusal, `TestAllShippedBackscrollCommandsAreExecutable`, and `TestShippedSearchGuidanceGuardrails` pass. The stale #33 preset is closed by Plan 2’s ingestion tests plus this plan’s command validation.
- Follow strict RED → GREEN → TRIANGULATE → REFACTOR. Run focused tests before `just check`, `just test`, and `just ci`.
- Commit commands below are future implementation instructions only. Do not stage or commit during planning.

---

## File map

| Path | Responsibility |
|---|---|
| `cmd/backscroll/shipped_assets_test.go` | Discover consumer assets, extract located invocations, resolve Cobra syntax, substitute fixtures, and execute safely. |
| `cmd/backscroll/shipped_guidance_test.go` | Assert the four semantic search guardrails across the primary Backscroll skill. |
| `cmd/backscroll/main.go` | Define the canonical effect annotation key/values, annotate the root, and register only explicitly annotated child commands. |
| `cmd/backscroll/{search,read,list,patterns,rebuild,purge,validate,status,config,annotate,recover}.go` | Declare each command’s maximum asset-execution effect explicitly at its canonical constructor. |
| `.claude/skills/backscroll/SKILL.md` | Correct commands and add the four search-discipline guardrails. |
| `.claude/skills/backscroll/ref-context-mode.md` | Replace stale flags/commands with real current invocations. |
| `.claude/skills/backscroll-doctor/SKILL.md` | Keep diagnostic invocations executable and evidence-disciplined. |
| `inputs/decisions.inputs.toml` | Validate Plan 2’s repaired command comments. |
| `README.md` and `docs/**/*.md` | Repair every scanner-reported shipped invocation while preserving intended workflow. |

### Task 1: Discover shipped assets and extract located Backscroll argv

**Files:**
- Create: `cmd/backscroll/shipped_assets_test.go`

**Current candidate asset inventory:**
- `.claude/skills/backscroll/SKILL.md`
- `.claude/skills/backscroll/ref-context-mode.md`
- `.claude/skills/backscroll-doctor/SKILL.md`
- `.claude/skills/backscroll-doctor/assets/gather.sh`
- `inputs/categories.toml`
- `inputs/claude.inputs.toml`
- `inputs/decisions.inputs.toml`
- `inputs/opencode.inputs.toml`
- `inputs/pi.inputs.toml`
- `README.md`
- `docs/audit-integration.md`
- `docs/backlog.md`
- `docs/configuration.md`
- `docs/eval/README.md`
- `docs/eval/corrections-calibration.md`
- `docs/eval/corrections-labeling-2026-07-20.md`
- `docs/eval/queries.toml`
- `docs/input-contract.md`
- `docs/intention-agentic-input-definitions.md`
- `docs/patterns.md`
- `docs/read.md`
- `docs/search.md`
- `docs/sync.md`

This inventory makes the current scan surface reviewable; discovery remains authoritative and catches future assets automatically. A valid discovered file is not modified unless its invocation fails extraction, Cobra resolution, safe execution, or the required guidance checks.

**Interfaces:**
- Consumes: repository filesystem rooted deterministically from `runtime.Caller(0)` at `cmd/backscroll/shipped_assets_test.go` and walking `../..`; asset content is read with `os.DirFS` and tests do not require Git metadata.
- Produces: `type assetInvocation struct { Path string; Line int; Raw string; Argv []string; HasMetavariables bool; HasShellSyntax bool }`, `func shippedConsumerAssets(root string) ([]string, error)`, and `func extractBackscrollInvocations(path string, data []byte) ([]assetInvocation, error)`.

- [ ] **Step 1: Write extraction tests covering prose, fences, TOML, and malformed calls**

```go
func TestExtractBackscrollInvocations(t *testing.T) {
    data := []byte("text `backscroll status`\n```bash\nbackscroll search --text \"two words\" --all-projects\n```\n# Then: backscroll recover --from <stranded.db> --dry-run\n")
    got, err := extractBackscrollInvocations("asset.md", data)
    if err != nil { t.Fatal(err) }
    if len(got) != 3 { t.Fatalf("invocations=%+v", got) }
    if got[1].Line != 3 || !reflect.DeepEqual(got[1].Argv, []string{"search", "--text", "two words", "--all-projects"}) {
        t.Fatalf("second=%+v", got[1])
    }
    if !got[2].HasMetavariables { t.Fatalf("expected metavariable: %+v", got[2]) }
}
```

Add cases for single/double quotes, escaped spaces, comments after argv, multiple invocations on separate lines, unclosed quotes, `$()`, pipes, redirects, `&&`, and ellipsis. Add a shell-asset case with the exact canonical wrapper declaration `BS="${BACKSCROLL_BIN:-backscroll}"` followed by `"$BS" search "query" --all-projects --json | jq ...`; assert the scanner extracts only `search query --all-projects --json` and never executes the pipeline.

- [ ] **Step 2: Run RED**

Run: `go test ./cmd/backscroll -run '^TestExtractBackscrollInvocations$'`

Expected: FAIL with `undefined: extractBackscrollInvocations`.

- [ ] **Step 3: Implement deterministic discovery and a narrow argv lexer**

`shippedConsumerAssets` walks the exact roots in Global Constraints, accepts `.md`, `.toml`, and `.sh` consumer assets, sorts slash-normalized paths, and applies only the three documented implementation-history exclusions. It must discover files rather than enumerate individual paths. The discovery test asserts every path in the current candidate inventory, including `gather.sh`, is present.

The lexer starts at a literal command-position `backscroll`. For `.sh` only, it also recognizes the exact repository wrapper assignment `BS="${BACKSCROLL_BIN:-backscroll}"` and command-position `"$BS"`; it does not evaluate arbitrary variables or shell expansions. It parses quoting/escapes into argv, stops the Backscroll invocation before a shell operator, marks shell metacharacters without executing them, and returns a located parse error for malformed quotes. Quoted prose such as the query string `"backscroll search"` is not an invocation. It never invokes a shell.

- [ ] **Step 4: GREEN and triangulate complete asset-root coverage**

Run: `go test ./cmd/backscroll -run '^(TestExtractBackscrollInvocations|TestShippedConsumerAssetDiscovery)$'`

Expected: PASS; discovery includes the complete current candidate inventory and excludes only `docs/roadmap/**`, `docs/research/**`, and `docs/superpowers/**`.

- [ ] **Step 5: Refactor lexer state names and rerun focused tests**

Run: `go test ./cmd/backscroll -run '^(TestExtractBackscrollInvocations|TestShippedConsumerAssetDiscovery)$'`

Expected: PASS.

- [ ] **Step 6: Commit the scanner as one reviewable test utility**

```bash
git add cmd/backscroll/shipped_assets_test.go
git commit -m "test(guidance): extract commands from shipped assets"
```

### Task 2: Resolve every invocation against real Cobra and execute through a safe harness

**Files:**
- Modify: `cmd/backscroll/shipped_assets_test.go`
- Modify: `cmd/backscroll/main.go:59-86`
- Modify: `cmd/backscroll/search.go:18-96`
- Modify: `cmd/backscroll/read.go:12-47`
- Modify: `cmd/backscroll/list.go:14-60`
- Modify: `cmd/backscroll/patterns.go:18-85`
- Modify: `cmd/backscroll/rebuild.go:15-29`
- Modify: `cmd/backscroll/purge.go:13-32`
- Modify: `cmd/backscroll/validate.go:14-35`
- Modify: `cmd/backscroll/status.go:15-42`
- Modify: `cmd/backscroll/config.go:14-42`
- Modify: `cmd/backscroll/annotate.go:13-46`
- Modify: `cmd/backscroll/recover.go` (created by Plan 3)

**Interfaces:**
- Consumes: `buildRootCmd(io.Writer, io.Writer) *cobra.Command` and discovered `assetInvocation` values.
- Produces: constants `assetEffectAnnotation = "backscroll.io/effect"`, `assetEffectRead = "read"`, `assetEffectWrite = "write"`, and `assetEffectReplace = "replace"`; `func declaredAssetEffect(cmd *cobra.Command) (string, error)`; `func resolveInvocation(root *cobra.Command, argv []string) (*cobra.Command, error)`; `func materializeExample(inv assetInvocation, env safeAssetEnv) ([]string, error)`; and `func executeInvocation(inv assetInvocation) error`.

- [ ] **Step 1: Write completeness and focused real-Cobra harness tests**

Add `TestEveryRootCommandDeclaresAssetEffect`. Build `buildRootCmd`, inspect the root and every direct child, and fail with command path plus raw annotation when the key is missing, empty, or not exactly `read`, `write`, or `replace`. Add a synthetic unannotated command and an unknown-value command to prove `declaredAssetEffect` rejects both.

```go
func TestAssetInvocationResolvesAgainstRealCobra(t *testing.T) {
    tests := []struct{ raw string; wantErr bool }{
        {"backscroll status --json --indexed-only", false},
        {"backscroll search --text sentinel --all-projects", false},
        {"backscroll recover --from <stranded.db> --dry-run", false},
        {"backscroll removed-command", true},
        {"backscroll search --removed-flag", true},
    }
    for _, tt := range tests {
        inv := mustExtractOne(t, tt.raw)
        err := executeInvocation(inv)
        if (err != nil) != tt.wantErr { t.Fatalf("%q error=%v", tt.raw, err) }
    }
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./cmd/backscroll -run '^(TestEveryRootCommandDeclaresAssetEffect|TestAssetInvocationResolvesAgainstRealCobra)$'`

Expected: FAIL because constructors do not declare effects and `executeInvocation` is undefined.

- [ ] **Step 3: Implement syntax resolution, fixture substitution, and fail-closed effects**

Build a fresh `buildRootCmd` for each invocation. Resolve command and flags through Cobra’s `Find`, `Args`, and `Flag` definitions; do not compare the verb to a list. Define the annotation vocabulary once in `main.go` and set it in every constructor listed in Files:

- root, `read`, `validate`, `status`, and `config`: `read`;
- `search`, `list`, and `patterns`: `write`, because their normal path may auto-sync before reading;
- `rebuild`, `purge`, and `annotate`: `write`;
- `recover`: `replace`.

`declaredAssetEffect` returns an error for missing, empty, or unknown annotation values. `executeInvocation` calls it after Cobra resolution and before fixture setup or command execution; no branch defaults to `read`.

`materializeExample` replaces recognized metavariable shapes by semantic flag position: query/name text → `sentinel`; project → `all-projects` fixture ID; file/path → a temp JSONL fixture; date → `2030-01-01`; UUID → seeded temp record UUID; stranded database → a temp recovery fixture. Unknown metavariable shapes fail with path/line rather than skip. Shell syntax is accepted only when the extracted Backscroll argv is complete before the operator; never execute the operator or neighboring command.

`executeInvocation` sets temp HOME/config/database, installs minimal valid manifests, seeds disposable state required by the command, sets `version = "dev"`, executes Cobra directly, and accepts domain errors only after command/flag/argument resolution when the example intentionally references absent user data. Write/replace annotations require disposable seeded paths and a postcondition that no path outside the temp root changed.

- [ ] **Step 4: GREEN and triangulate destructive/example safety**

Run: `go test ./cmd/backscroll -run '^(TestEveryRootCommandDeclaresAssetEffect|TestAssetInvocationResolvesAgainstRealCobra|TestAssetHarnessRejectsMissingOrUnknownEffect|TestAssetHarnessNeverExecutesShellSyntax|TestAssetHarnessContainsWriteCommandsToTempRoot|TestAssetHarnessRejectsUnknownMetavariable)$'`

Expected: PASS; every root constructor is explicit, missing/unknown effects refuse execution, write/replace examples mutate only temporary state, unknown example variables remain hard failures, and removed commands/flags fail through Cobra rather than an allowlist.

- [ ] **Step 5: Refactor harness setup and run the CLI package**

Run: `go test ./cmd/backscroll`

Expected: PASS with no network or real HOME/config access.

- [ ] **Step 6: Commit the real-Cobra safety harness**

```bash
git add cmd/backscroll/shipped_assets_test.go cmd/backscroll/main.go cmd/backscroll/search.go cmd/backscroll/read.go cmd/backscroll/list.go cmd/backscroll/patterns.go cmd/backscroll/rebuild.go cmd/backscroll/purge.go cmd/backscroll/validate.go cmd/backscroll/status.go cmd/backscroll/config.go cmd/backscroll/annotate.go cmd/backscroll/recover.go
git commit -m "test(guidance): execute shipped commands through cobra"
```

### Task 3: Repair shipped commands and add the four search guardrails

**Files:**
- Create: `cmd/backscroll/shipped_guidance_test.go`
- Modify: `.claude/skills/backscroll/SKILL.md`
- Modify: `.claude/skills/backscroll/ref-context-mode.md`
- Modify: `inputs/decisions.inputs.toml`
- Modify: `inputs/opencode.inputs.toml`
- Modify: `inputs/pi.inputs.toml`
- Modify: `README.md`
- Modify: `docs/audit-integration.md`
- Modify: `docs/configuration.md`
- Modify: `docs/eval/README.md`
- Modify: `docs/eval/corrections-calibration.md`
- Modify: `docs/input-contract.md`
- Modify: `docs/intention-agentic-input-definitions.md`
- Modify: `docs/patterns.md`
- Modify: `docs/read.md`
- Modify: `docs/sync.md`

These are the current known stale scanner failures. Do not modify valid discovered assets merely because they are in the candidate inventory. If discovery finds a future failing path not listed here, stop and add that exact path to the task’s Files list before editing it; never use a wildcard staging command or a generic “all discovered files” instruction.

**Interfaces:**
- Consumes: final command/flag contracts from Plan 3, repaired presets from Plan 2, and the failing path/line/argv report from Task 2.
- Produces: executable shipped guidance and `func searchGuidanceGuardrails(data []byte) []string`, where an empty result means all four semantic requirements are present.

- [ ] **Step 1: Write the all-assets and semantic guardrail assertions first**

```go
func TestAllShippedBackscrollCommandsAreExecutable(t *testing.T) {
    root := repositoryRoot(t)
    assets, err := shippedConsumerAssets(root)
    if err != nil { t.Fatal(err) }
    for _, path := range assets {
        data, err := os.ReadFile(filepath.Join(root, path))
        if err != nil { t.Fatal(err) }
        invocations, err := extractBackscrollInvocations(path, data)
        if err != nil { t.Fatal(err) }
        for _, inv := range invocations { t.Run(fmt.Sprintf("%s:%d", inv.Path, inv.Line), func(t *testing.T) {
            if err := executeInvocation(inv); err != nil { t.Fatalf("%s:%d argv=%q: %v", inv.Path, inv.Line, inv.Argv, err) }
        }) }
    }
}
func TestShippedSearchGuidanceGuardrails(t *testing.T) {
    data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".claude/skills/backscroll/SKILL.md"))
    if err != nil { t.Fatal(err) }
    if missing := searchGuidanceGuardrails(data); len(missing) != 0 { t.Fatalf("missing search guardrails: %v", missing) }
}
```

Implement the guardrail helper as four independent phrase-family checks so harmless wording changes are allowed but each required behavior remains explicit: ranked-hit inspection before conclusion; follow-up query vocabulary from retrieved artifacts; malformed/failed call is not absence evidence; minimal fixture reproduction before index-gap claim.

- [ ] **Step 2: Run RED and capture real drift**

Run: `go test ./cmd/backscroll -run '^(TestAllShippedBackscrollCommandsAreExecutable|TestShippedSearchGuidanceGuardrails)$'`

Expected: FAIL with located stale invocations such as `inputs/decisions.inputs.toml`’s historical `backscroll sync`, `.claude/skills/backscroll/SKILL.md`’s `backscroll version`, and `ref-context-mode.md`’s removed `--input` flag, plus absent guardrails. Preserve path/line/argv in every command failure.

- [ ] **Step 3: Repair guidance from scanner output and add explicit guardrails**

For each located failure in the Files list, use `backscroll <command> --help` from the current Cobra tree to select the real replacement. Required known repairs include: use `backscroll --version`, not `backscroll version`; replace removed `--input` filters with current `--source` or `--source-path` semantics; replace removed `sync`, `inputs`, `sessions`, `events`, `resume`, and `topics` workflows with current `search`, `list`, `config`, `status`, `validate`, `read`, or `rebuild` commands according to the surrounding intent; replace unsupported audit/example flags rather than teaching the harness to accept domain failures; retain Plan 2’s decisions preset `config` plus `search --source decision` instructions.

Add a concise “Evidence discipline” section to the Backscroll skill with all four imperative rules. Do not imply a zero-result search proves absence, a failed command proves an index gap, or direct `read` proves index freshness.

- [ ] **Step 4: GREEN and triangulate every shipped asset**

Run: `go test ./cmd/backscroll -run '^(TestShippedSearchGuidanceGuardrails|TestAllShippedBackscrollCommandsAreExecutable)$'`

Expected: PASS with zero stale command/flag reports.

- [ ] **Step 5: Refactor prose for scanability and rerun tests**

Keep commands adjacent to their purpose, put the guardrails before troubleshooting, and remove contradictory stale instructions rather than appending caveats. Run: `go test ./cmd/backscroll -run '^(TestShippedSearchGuidanceGuardrails|TestAllShippedBackscrollCommandsAreExecutable)$'`

Expected: PASS.

- [ ] **Step 6: Commit each coherent guidance repair with its validator**

```bash
git add .claude/skills/backscroll/SKILL.md .claude/skills/backscroll/ref-context-mode.md inputs/decisions.inputs.toml inputs/opencode.inputs.toml inputs/pi.inputs.toml README.md docs/audit-integration.md docs/configuration.md docs/eval/README.md docs/eval/corrections-calibration.md docs/input-contract.md docs/intention-agentic-input-definitions.md docs/patterns.md docs/read.md docs/sync.md cmd/backscroll/shipped_guidance_test.go
git commit -m "docs: align shipped guidance with executable commands"
```

### Task 4: Close issue #30 and the stale-preset portion of #33

**Files:**
- Modify: `cmd/backscroll/shipped_assets_test.go`
- Modify: `cmd/backscroll/shipped_guidance_test.go`

**Interfaces:**
- Consumes: all Plan 4 validation plus Plan 2 repaired preset and Plan 3 `recover` registration.
- Produces: complete #30 closure evidence and final command-integrity evidence for #33.

- [ ] **Step 1: Run the named discovery, effect, execution, and guidance closure tests**

Run: `go test ./cmd/backscroll -run '^(TestShippedConsumerAssetDiscovery|TestEveryRootCommandDeclaresAssetEffect|TestAssetHarnessRejectsMissingOrUnknownEffect|TestAllShippedBackscrollCommandsAreExecutable|TestShippedSearchGuidanceGuardrails)$'`

Expected: PASS; discovery includes every current candidate and `gather.sh`; missing/unknown effects refuse execution; invocation failures include asset path, 1-based line, parsed argv, and Cobra or safe-harness cause.

- [ ] **Step 2: Triangulate examples and destructive invocations**

Run: `go test ./cmd/backscroll -run '^(TestEveryRootCommandDeclaresAssetEffect|TestAssetHarnessRejectsMissingOrUnknownEffect|TestAssetHarnessNeverExecutesShellSyntax|TestAssetHarnessContainsWriteCommandsToTempRoot|TestAssetHarnessRejectsUnknownMetavariable|TestAllShippedBackscrollCommandsAreExecutable)$'`

Expected: PASS; no invocation is accepted by string matching alone, no unknown metavariable is skipped, missing/unknown effects refuse execution, and no write/replace command escapes the disposable root.

- [ ] **Step 3: Run repository gates**

Run: `just check`

Expected: PASS.

Run: `just test`

Expected: PASS.

Run: `just ci`

Expected: PASS.

- [ ] **Step 4: Record exact issue closure evidence**

The implementation PR states: #30 closes because the five named discovery/effect/execution/guidance tests pass, every current root constructor declares a known effect, missing/unknown effects fail closed, every invocation across every discovered shipped consumer asset resolves against `buildRootCmd`, safe execution uses hermetic fixture substitution, and all four guardrails are present. The stale-preset portion of #33 closes because `inputs/decisions.inputs.toml` contains only executable commands; #33 itself also requires Plan 2’s `TestActiveManifestsPreflightBeforeSync` and `TestDecisionManifestMarkdownSectionsEndToEnd`.

- [ ] **Step 5: Commit only if closure assertions changed**

```bash
git add cmd/backscroll/shipped_assets_test.go cmd/backscroll/shipped_guidance_test.go
git commit -m "test(guidance): prove shipped command integrity"
```
