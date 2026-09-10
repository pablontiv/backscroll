# Installing the Backscroll skill

Backscroll owns one canonical skill tree: `.claude/skills/backscroll/` in this
repository. On macOS/Linux, `scripts/install-skills.py` installs absolute symlinks
to that tree in an **operator-selected stable ordinary clone**. It requires Python
3.9+ and Git; these are not dependencies of the Backscroll binary.

| Supported target | Installed entry |
| --- | --- |
| Claude Code | `<home>/.claude/skills/backscroll` |
| Agents-compatible discovery (Codex) | `<home>/.agents/skills/backscroll` |
| OpenCode | `<config-home>/opencode/skills/backscroll` |

`--home` and `--source-root` are mandatory. `--config-home` defaults to the explicit
home's `.config`; it deliberately does not inherit ambient `XDG_CONFIG_HOME` or
`BACKSCROLL_CONFIG_DIR`. If OpenCode uses a different XDG config root, pass that
same root explicitly to **both** plan and apply. All three targets are included
in one approval. No other directory is swept or removed. In particular, old
`backscroll-doctor`, plugin-managed skills, project overrides, and unsupported
copies are outside this installer's ownership; inventory and retire them through
their own owners, with separate approval. This installer is not for Windows.

Neither `install.sh`/`install.ps1` nor `pre-push`/`post-merge` replaces skill
folders. Downloading a binary does not establish a stable skill source. Git hooks
still handle their existing binary/input work, but skill replacement is never a
side effect of a push or merge.

## Inspect, approve, install

Choose an ordinary product clone that you intend to keep. Do not choose a task
copy or a directory scheduled for cleanup. The installer rejects linked Git
worktrees, missing or dirty canonical skills, untracked/ignored overlays, skill-tree
symlinks, and source/destination overlap. An ordinary clone can still be deleted
by its owner: filesystem inspection cannot prove your retention intent.

Run the read-only inventory from the product repository:

```bash
python3 scripts/install-skills.py plan \
  --source-root /absolute/stable/backscroll --home /absolute/destination/home
```

Inspect the full JSON: source root, canonical path, source commit, tree SHA-256,
all three exact destination paths, and recursive `before` preimages. The digest
covers filenames, file contents, modes and lexical symlink targets, not access
times, ACLs or extended attributes. Backups preserve the original filesystem
objects by rename rather than copying their contents. No symlink is traversed
while inventorying a destination. Special files and hard-linked regular files
are refused rather than incompletely inventoried.

**For a real home, obtain explicit approval for this exact inventory digest.**
A historical issue digest, a successful fixture test, or permission to implement
the installer is not rollout permission. Never automatically pipe plan's digest
into apply. After approval, repeat the same arguments:

```bash
python3 scripts/install-skills.py apply \
  --source-root /absolute/stable/backscroll --home /absolute/destination/home \
  --approve APPROVED_PLAN_SHA256
```

A changed source commit/content, path, destination type/content/mode, or action
invalidates approval. Plan writes nothing; an approval mismatch does not even
create installation state. Apply checks again under a per-home advisory lock,
records recovery instructions, renames each previous object to a backup, then
creates the link. Already-correct links are not moved or backed up again; repeated
installation returns `changed: 0`. Backups and destinations must be on the same
filesystem. Symlinked destination/state ancestors are refused.

This is a local, single-operator filesystem tool, not a security boundary against
a hostile process running as the same user. Stop concurrent writers and source
updates during application/restoration. The lock serializes this installer only;
it cannot lock your editor or Git. Do not use it on shared/network filesystems.

## Verify and update

Repeat `plan` with the same paths: every target should report `change: false`,
`before.kind: link`, and `before.target` equal to `source.path`. Inspect both the
lexical link (`readlink <installed-entry>`) and the resolved `SKILL.md` contents.
The three entries resolve to the same source, not separately copied versions.

Update the stable clone through your normal reviewed Git workflow. All three
links then expose the new source without reinstalling. Do not move/delete the
source while links depend on it. Restart runtimes that cache skills. Local edits
to that source are immediately visible too; the links are not immutable release
snapshots. A new `plan` validates the current committed source again.

Runtime checks are distinct from link checks:

- Claude: its [documented personal-skill discovery](https://code.claude.com/docs/en/skills)
  follows symlinked skill folders. Check `/skills` in a fresh session. The CLI
  `claude plugin validate <skills-parent> --json` **skips symlink entries** even
  when it reports success; validate the canonical parent `.claude/skills` as well.
  Validator success alone is not a session-discovery result.
- Codex: [user skills](https://developers.openai.com/codex/skills) are discovered
  under `$HOME/.agents/skills`, including symlinked directories. The app-server
  `skills/list` response reports the resolved `SKILL.md` path and enabled state.
- OpenCode: `opencode debug skill --pure` reports available skills, their lexical
  locations and content. Its [discovery contract](https://opencode.ai/docs/skills/)
  also scans Claude/Agents-compatible paths; checking only a name can mask a broken
  OpenCode-specific entry. Verify the returned location and content.

The installed recipe uses cwd inference first, semantic IDs for explicit
`--project`, and robot `result_N_filepath` / `result_N_content` keys. JSON's
`source_path` / `snippet` keys are not robot field names.

## Receipts, rollback and restoration

Apply returns `receipt` and `restore_approval`. Keep both. The private directory
`<home>/.local/state/backscroll/skill-install/<transaction>/` contains an immutable
`receipt.json`, backups, and append-only `events.jsonl`. The prepared event holds
the restore digest before the first replacement; install/restore intent and
completion events are flushed to disk. The receipt records all planned targets,
so recovery does not rely on the last console line. No automatic backup retention
or deletion policy is imposed.

An ordinary application error attempts rollback of all changed destinations. If
rollback cannot safely reconcile a changed destination or backup, it stops and
preserves the evidence; the error prints the receipt path and restore digest.
After interruption, inspect the transaction receipt and its prepared event, then
use the same explicit restoration command:

```bash
python3 scripts/install-skills.py restore --home /absolute/destination/home \
  --receipt /absolute/path/to/receipt.json --approve RESTORE_SHA256
```

Restore validates **every** destination and backup before restoring any. It moves
backups back without overwriting new operator work, restores originally absent
entries to absence, and is safe to repeat after interruption or successful
restoration. A source clone need not still exist for restoration. Empty parent
directories created by installation may remain. If a destination/backup was edited
since install, obtain a fresh inventory and operator decision; do not force a
restore or hand-delete evidence to make it pass.

For uninstall without restoring the previous copies, use the same two-step
`plan` / `apply` flow with `--action uninstall` on **both** commands. Uninstall
only removes exact links to the selected source, backs those links up, and refuses
unowned entries. Its own receipt can restore the links. To recover the older
copies instead, restore the original installation receipt after reconciling any
later transactions in reverse order. No command deletes the product source.

## Tests and evidence

```bash
just test-skills
# Optional installed-runtime probe, macOS only; blocks network and external writes:
python3 tests/skill_runtime_probe.py
```

Tests create all homes, Git sources, databases and backup state under the ignored
`.local-evidence/` directory. They never replace real installed skills. CI runs
filesystem and installed-recipe E2E tests on Linux and macOS without requiring
agent runtimes or credentials. See [issue #62 evidence](eval/skill-installation.md)
for the versioned native RED, corrected robot oracle, results and runtime limits.
