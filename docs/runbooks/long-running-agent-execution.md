# Long-Running Agent Execution Runbook

## 1) Purpose

Use this runbook when a task is long-running, multi-step, or likely to need handoffs, reviews, retries, and durable state.

Do not use it for tiny one-file fixes, trivial edits, or work that can be finished safely in one pass.

## 2) Bounded task-review state machine

- A task gets at most five fix-plus-scoped re-review rounds.
- Rounds 1–3 resume the original implementer.
- Rounds 4–5 dispatch a fresh, more capable implementer.
- After round 5 there is no ordinary round 6. Adjudicate every residual finding and record `Ruling: <decision> — <reason> — <cost if wrong>`; ask the human only if one of the four stop conditions applies.
- A wrong-worktree attempt that is stopped and quarantined before review does not count as a reviewed fix round.

## 3) Preconditions and safety boundaries

**Before starting:**

- Confirm the exact worktree.
- Confirm the task brief, plan, optional spec, and current ledger.
- Start a durable ledger before editing.
- Exactly one implementation writer may be active globally, including work on disjoint files.
- Prefer read-only analysis first.
- Never rely on memory for current state.

**Boundaries:**

- Do not edit outside the assigned worktree.
- Do not stage, commit, push, merge, publish, or run destructive git commands unless the task explicitly allows it.
- Do not spawn subagents from workers unless the controller explicitly permits them.
- Do not let reviewers write code.
- Parallel activity is limited to read-only analysis that does not review an artifact currently being mutated.
- Freeze/stop mutation before independent review begins; mutation may resume only after review findings return.
- Review of an artifact must not overlap mutation of that artifact.

## 4) Validated loop

| Stage | What to do | Invariant |
|---|---|---|
| Recover context | Re-read the plan, optional spec, ledger, and latest progress/report notes | Fresh session can rebuild state from files alone |
| Isolate worktree | Verify resolved cwd, branch, worktree identity, and git status before mutation | No cross-worktree edits |
| Establish durable ledger | Create/update the ledger with plan identity, task state, owners, checkpoints, risks, and stop conditions | Ledger outranks memory |
| One global implementation writer | Keep exactly one implementation writer active globally; no second implementation writer, even on disjoint files | No write contention |
| Task brief | Give each writer one crisp objective and one file scope | No drift |
| Implement | Make the smallest change that advances the brief | Change stays local |
| Initial evidence | Before review, report exact focused test commands/results for the initial implementation | Claims are backed by output |
| Freeze package | Freeze tracked changes against BASE plus all new/untracked content, focused-test output, and ledger entry | Review reads a stable package, not the mutable worktree |
| Independent review | Use a separate read-only reviewer that checks spec and quality separately | Review is independent |
| Bounded fix loop | Apply only review-approved corrections | Fixes are narrow |
| Adjudicate residuals | Record `Ruling: <decision> — <reason> — <cost if wrong>` for every residual finding; ask the human only if one of the exact four stop conditions triggers | Every residual is closed or escalated |
| Commit checkpoint | Commit a coherent slice only when COMMIT_POLICY permits and after approval or cap adjudication; otherwise record frozen checkpoint/hash/diff identity without committing | A known-good checkpoint exists |
| Continue without polling | Move to the next queued item; do not wait for human narration | Execution stays flowing |

Per-task lifecycle:

1. Record BASE.
2. Assign one fresh implementer and record active/original implementer identity.
3. Make the initial implementation.
4. Run focused tests and report exact commands/results before review.
5. Freeze the candidate package.
6. Run independent read-only review against the frozen package with separate spec and quality verdicts.
7. Fix only the scoped findings and re-review only that scope, up to the five-round cap.
8. Adjudicate every residual finding in the ledger with `Ruling: <decision> — <reason> — <cost if wrong>`.
9. Commit only when COMMIT_POLICY permits and only after approval or cap adjudication; when commits are forbidden, record the frozen checkpoint/hash/diff identity without committing.
10. Mark the ledger complete.
11. Proceed to the next task.

Review packaging for uncommitted diffs: package the candidate as exact tracked changes against BASE plus all new/untracked content, focused-test output, and the current ledger entry. Reviewers inspect the frozen candidate package, not the mutable working copy, and never overlap mutation of the reviewed artifact. When COMMIT_POLICY forbids staging/committing, build the package without staging (for example, by saving `git diff`, `git ls-files --others --exclude-standard`, copied untracked file contents, command output, and ledger excerpts under an allowed artifact path). After all task loops, package the whole branch/range for independent final review.

Mandatory final gates: run every final command mandated by PLAN plus repository gates (format/check/tests/coverage as applicable, using commands discovered from the repository rather than invented), record exact commands and outcomes, and if final review finds issues, use one bounded final fix wave and one scoped re-review. After that final fix wave, rerun every PLAN/repository final gate on the resulting final state and record fresh exact commands/results. Do not claim completion until the post-fix gates and final review have current evidence.

This runbook intentionally uses review-before-commit because that is the observed lifecycle; keep that ordering explicit.

## 5) Safe parallelism

Safe to overlap:

- Read-only analysis that does not review an artifact currently being mutated.
- Test exploration that does not write shared state.
- Parallel checks that remain read-only and stop before any independent review of a frozen artifact.

Not safe to overlap:

- Any second implementation writer, even on disjoint files.
- Any task that depends on another task’s uncommitted output.
- Review of an artifact while it is still being mutated.
- Independent review before mutation has been frozen and stopped.

Rule of thumb: serialize all mutation; parallelize only read-only work that cannot conflict with the single active writer.

## 6) Instruction patterns that worked

### Bootstrap prompt

```text
You are the fresh session controller for this task.

Operational placeholders, to be filled before execution:
- WORKTREE=<assigned worktree>
- PLAN=<plan path>
- SPEC=<optional spec path or "none">
- LEDGER=<ledger path>
- COMMIT_POLICY=<allowed/forbidden and checkpoint rule>
- QUARANTINE_PATH=<authorized quarantine path>

1. Enter and verify the assigned checkout before any mutation:
   - Run `cd "$WORKTREE" && pwd -P` and verify the resolved cwd equals the intended WORKTREE.
   - Verify expected branch/worktree identity with `git branch --show-current`, `git rev-parse --show-toplevel`, and `git worktree list` (or the repository's equivalent identity check).
   - Run `git status --short --branch` and record the result.
   - If cwd, branch, worktree identity, or status is not the expected assigned state, do not mutate. Use the wrong-worktree incident procedure below.

2. Read durable context before editing:
   - Read PLAN.
   - If SPEC is not "none", read SPEC.
   - Read LEDGER if it exists. If LEDGER is absent, create it before mutation with the PLAN identity, SPEC identity, WORKTREE, COMMIT_POLICY, BASE placeholder, task list placeholder, and current timestamp/session identity.
   - Ledger outranks memory.

3. Maintain these global rules:
   - Exactly one implementation writer may be active globally, including work on disjoint files.
   - Parallel work is read-only only, and must not review an artifact currently being mutated.
   - Freeze/stop mutation before independent review begins; mutation may resume only after review findings return.
   - Reviewers are read-only and review the frozen package, not the mutable worktree.
   - Do not spawn subagents unless explicitly permitted.
   - Never push, merge, publish, edit outside WORKTREE, or cause another outside-worktree side effect without explicit permission; this is one of the stop conditions.

4. Use the exact five-round task state machine:
   - Each task gets at most five fix-plus-scoped re-review rounds.
   - Rounds 1–3 resume the original implementer.
   - Rounds 4–5 dispatch a fresh, more capable implementer.
   - After round 5 there is no ordinary round 6. Adjudicate every residual finding and record `Ruling: <decision> — <reason> — <cost if wrong>`; ask the human only if one of the four stop conditions applies.
   - A wrong-worktree attempt that is stopped and quarantined before review does not count as a reviewed fix round.

5. For every task, persist these fields in LEDGER:
   - BASE (`git rev-parse HEAD` or the approved base identifier before task mutation).
   - Active implementer identity and original implementer identity.
   - Current fix round number.
   - Review spec verdict and review quality verdict.
   - Every finding and its disposition.
   - Focused test commands/results, including exact initial implementation focused test commands/results before review.
   - Frozen candidate package path or identity.
   - Commit checkpoint, or when COMMIT_POLICY forbids commits, the frozen checkpoint/hash/diff identity without a commit.
   - Rulings in the exact form `Ruling: <decision> — <reason> — <cost if wrong>`.

6. Implement and collect evidence:
   - Record BASE before mutation.
   - Assign exactly one implementer for one bounded task.
   - Make the smallest local change that satisfies the brief.
   - Before any review, run and report exact focused test commands/results for the initial implementation, even if they fail or are intentionally not run with a stated reason.

7. Freeze the candidate for review:
   - Stop mutation.
   - Package exact tracked changes against BASE plus all new/untracked content, focused-test output, and the current LEDGER entry.
   - The independent reviewer reads this frozen candidate package, not the mutable worktree.
   - If COMMIT_POLICY forbids staging/committing, package safely without staging: save `git diff "$BASE"`, `git ls-files --others --exclude-standard`, copies or archived contents of each untracked file, focused-test output, and the LEDGER excerpt under an allowed artifact path.

8. Review and fix:
   - Run independent read-only review with separate spec and quality verdicts.
   - Apply only scoped reviewer findings.
   - After each fix, collect exact commands/results and focused tests, update LEDGER, freeze a new package, and run only the scoped re-review needed for the changed scope.
   - At the cap, adjudicate every residual finding with a Ruling.

9. Enforce COMMIT_POLICY:
   - Commit only when COMMIT_POLICY permits it, and only after review approval or cap adjudication.
   - When COMMIT_POLICY forbids commits, do not stage or commit; record the frozen checkpoint/hash/diff identity in LEDGER instead.
   - Never push, merge, or publish without explicit permission; treat that as an outside-worktree stop.

10. Wrong-worktree incident procedure:
   - Stop the writer immediately.
   - Inventory both the intended WORKTREE and the accidental checkout: resolved cwd, branch, `git status --short --branch`, and relevant changed/untracked paths.
   - Never broad reset/clean; preserve pre-existing user changes.
   - Quarantine only positively identified agent-created output under QUARANTINE_PATH.
   - Do not touch files whose ownership is uncertain. If ownership is uncertain but cleanup/proceeding does not require an irreversible/destructive operation, record a Ruling and continue without touching uncertain files. Stop only when cleanup/proceeding would require an irreversible/destructive operation.
   - Verify the intended candidate in WORKTREE before resuming.
   - Record the incident, inventory, quarantine path, uncertainty disposition, and verification in LEDGER.

11. Final sequence:
   - After all task loops, package the whole branch/range for independent final review.
   - Run every final command mandated by PLAN plus repository gates discovered from the repository (format/check/tests/coverage as applicable), and record exact commands/results.
   - If final review finds issues, use one bounded final fix wave and one scoped re-review, then adjudicate residuals.
   - After that final fix wave, rerun every PLAN/repository final gate on the resulting final state and record fresh exact commands/results.
   - Do not claim success until final review and the post-fix gates are fresh and recorded in LEDGER.

12. Stop only for these four conditions:
   - irreversible/destructive operation;
   - security-sensitive action;
   - outside-worktree side effect normally requiring permission (merge/push/publish);
   - plan so broken every path is a guess.
   Ordinary ambiguity does not stop the run. Record `Ruling: <decision> — <reason> — <cost if wrong>` and continue.
```

### Implementer prompt

```text
You are the implementer for one bounded task.
Work only in the assigned worktree.
Do not spawn subagents unless the controller assigns them.
Do not edit outside the assigned file scope.
Read the task brief, ledger, and relevant plan notes first.
Make the smallest change that satisfies the brief.
Report exact commands/results, focused tests, and any residual risk.
```

### Reviewer prompt

```text
You are the independent reviewer.
Read only; do not edit files.
Check the frozen candidate package against the task brief, ledger, plan, and optional spec.
Return separate spec and quality verdicts, plus concrete evidence.
```

### Fix-round prompt

```text
Apply only the scoped reviewer findings.
Keep the patch minimal and local.
Do not rework unrelated code.
Re-run the narrow tests that prove the fix.
State clearly what changed and what was revalidated.
```

### Session-resume prompt

```text
Resume from the ledger, not from memory.
First read: plan, progress notes, task reports, and the latest git status.
Then restate the current task queue, the active writer, and the next checkpoint.
If anything is ambiguous, record: `Ruling: <decision> — <reason> — <cost if wrong>` and continue, unless a stop condition applies.
```

## 7) Anti-patterns observed

| Anti-pattern | Why it caused churn |
|---|---|
| Wrong cwd / wrong worktree | Edits landed in the wrong checkout and had to be quarantined or restored. |
| Reviewer spawned by worker | Broke independence and blurred accountability. |
| Controller fixes code directly | Collapsed the separation between orchestration and implementation. |
| Unbounded review | Produced repeated nitpicks without a clear stop point. |
| Silent warnings | Deferred problems surfaced later as churn. |
| Trusting memory over ledger | Session state drifted and caused duplicate or conflicting work. |
| Concurrent writers | Created merge conflicts and ambiguous ownership. |

## 8) Failure recovery playbook

1. **Stop the writer.** Pause mutation immediately.
2. **Inventory both checkouts.** Identify the intended worktree and the stray checkout, including resolved cwd, branch, status, and changed/untracked paths.
3. **Do not broad reset/clean.** Preserve pre-existing changes.
4. **Quarantine only positively identified agent output** under the authorized quarantine path.
5. **Do not touch uncertain files.** If ownership is uncertain but cleanup/proceeding does not require an irreversible/destructive operation, record `Ruling: <decision> — <reason> — <cost if wrong>` and continue without touching uncertain files. Stop only when cleanup/proceeding would require an irreversible/destructive operation.
6. **Verify the candidate worktree.** Re-read cwd, branch, plan, spec if any, ledger, and git status.
7. **Record the incident in the ledger.** Include inventories for intended and accidental checkouts, quarantine path, uncertainty disposition, and candidate verification.
8. **Reverify cwd/branch before resuming.**

### Stop conditions

Stop and escalate only for:

- irreversible/destructive operation;
- security-sensitive action;
- outside-worktree side effect normally requiring permission (merge/push/publish);
- plan so broken every path is a guess.

Ordinary ambiguity does not stop the run. Instead, record: `Ruling: <decision> — <reason> — <cost if wrong>` and continue.

## 9) Fresh-session launch checklist

- [ ] Fill the canonical bootstrap placeholders: WORKTREE, PLAN, optional SPEC, LEDGER, COMMIT_POLICY, and QUARANTINE_PATH.
- [ ] Use the single canonical `### Bootstrap prompt` in section 6; do not copy or maintain a second bootstrap block.
- [ ] Confirm the assigned worktree by entering WORKTREE and verifying resolved cwd, branch/worktree identity, and git status before mutation.
- [ ] Read PLAN, SPEC if present, latest progress notes, relevant task reports, and LEDGER.
- [ ] Create LEDGER if absent.
- [ ] Verify exactly one or zero implementation writer globally.
- [ ] Define the next bounded task.
- [ ] Choose the smallest test that proves it.

## 10) Evidence and limitations

Final review packaging and gates: after all task loops, package the whole branch/range for independent final review; run every final command mandated by PLAN plus repository gates (format/check/tests/coverage as applicable, using commands discovered from the repository rather than invented); record exact commands and outcomes. If final review finds issues, use one bounded final fix wave and one scoped re-review, then adjudicate residuals; after that final fix wave, rerun every PLAN/repository final gate on the resulting final state and record fresh exact commands/results. Do not claim completion until final review and the post-fix gates have fresh evidence.

This runbook is evidence-based, but it is derived from **one extended execution**. Treat the patterns here as observed guidance from that session, not universal law.

**Observed facts from the session:**

- Isolated worktrees reduced accidental cross-editing.
- A ledger made session recovery practical.
- Separate review improved correctness.
- Narrow fix loops prevented drift.

**General rules inferred from the session:**

- Serialize shared writes.
- Keep prompts short and explicit.
- Prefer file-backed state over memory.
- Validate before declaring success.
