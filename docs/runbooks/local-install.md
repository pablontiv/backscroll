# Install the exact local commit

Run this only after the intended change has been merged and the local branch
contains that approved commit. Obtain operator authorization for the exact
destination binary and configuration home before installation. This source-build
path installs locally; it does not tag, release, publish or deploy.

From the clean Backscroll repository root, with Go installed:

```bash
# Stop if tracked files or the index differ from HEAD.
git diff --quiet && git diff --cached --quiet || exit 1
expected_commit="$(git rev-parse HEAD)"
install_dir="${BACKSCROLL_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"
go build -buildvcs=true -o "$install_dir/backscroll" ./cmd/backscroll

# Use the runtime configuration HOME explicitly, including on macOS.
# This is the parent of backscroll/inputs, not that inputs directory itself.
config_dir="${BACKSCROLL_CONFIG_DIR:-$HOME/.config}"
"$install_dir/backscroll" presets install --config-home "$config_dir"

# Inspect the installed artifact, not a temporary build or a shadowing PATH entry.
"$install_dir/backscroll" --version
go version -m "$install_dir/backscroll"
installed_commit="$(go version -m "$install_dir/backscroll" | awk '$2 ~ /^vcs.revision=/ {sub(/^vcs.revision=/, "", $2); print $2}')"
test "$installed_commit" = "$expected_commit" || exit 1
go version -m "$install_dir/backscroll" | grep -q 'vcs.modified=false' || exit 1
printf 'Verified installed Backscroll commit: %s\n' "$installed_commit"
command -v backscroll
```

`--version` intentionally reports `dev` for this source build. The installed
binary's **Go build-version metadata** (`go version -m`, `vcs.revision`) is the
exact commit proof; a release tag or the `dev` string alone cannot prove it.
Keeping `main.version=dev` also preserves the existing intrinsic development-build
autoupdate exemption; do not invent an environment opt-out or stamp a fake release
version. A dirty build reports `vcs.modified=true` and must not count as verified.

`presets install --config-home "$config_dir"` installs the six shipped presets
into `$config_dir/backscroll/inputs` and preserves existing files. It uses this
explicit configuration home rather than guessing from the platform's native
configuration directory. Review the resolved active definitions with the same
runtime environment (`BACKSCROLL_CONFIG_DIR="$config_dir"`). An existing active
OMP manifest is already activation; retain its roots and reasoning choice.
Refreshing existing presets with `--overwrite` replaces all six files and
requires separate operator approval of that exact scope. Obtain approval before
editing an existing individual manifest too.

Check the installed `vcs.revision` against the intended merged fixes, not only
`--version`. For OMP coexistence this includes the Pi native events (#199), Codex
native events (#200), bounded search (#201), OpenCode ownership (#159), and
native recovery identity correction (#205). In a source checkout, confirm each
required merged commit is an ancestor of the installed revision with
`git merge-base --is-ancestor <required-merge-commit> "$installed_commit"`.

Operational Backscroll commands synchronize its SQLite index and require
authorization for the exact index and source scope; they read provider transcript
stores without modifying them. For the separate global installed mixed-target
verification, after isolated mixed-provider acceptance passes, use the authorized
installed binary and explicit configuration home for bounded recall and
tool-pattern checks against known operator-owned anchors. Record the resolved
OMP format, roots, active state and reasoning choice, returned ownership and
counts privately. Missing eligible sources or anchors are unavailable evidence,
not successful ingestion. A synthetic PASS or a matching binary revision alone
does not establish installed-target ingestion. This global index verification is
independent of the scoped OMP proof below, which uses its own private index.

OMP defaults to `~/.omp/agent/sessions/*/*.jsonl`; custom agent directories or
profiles require explicit manifest roots. For custom `CODEX_HOME`, edit only the
approved Backscroll manifest roots. See [OMP verification and limits](../input-contract.md#complete-omp-example)
and [Codex verification and limits](../input-contract.md#complete-codex-example).

The isolated integration regression uses temporary HOME/config/SQLite state and
the actual CLI (`TestMixedProviderNativeOwnershipReplayAndRecovery`). A working
tree smoke build must record `vcs.modified=true` honestly. Final artifact proof
uses the captured clean revision and re-exercises the mixed CLI path. Installation
into the operator's actual `~/.local/bin`, active input directory and index is a
separate authorized operation; it remains unverified until those checks occur.

## Actual OMP headless source and private consumer proof

OMP 18.12.6 supports the following bounded headless procedure with an already
configured provider. Run from a new, operator-owned scratch working directory,
using the normal OMP HOME so native canonical session logging remains enabled.
Keep credentials in the existing provider setup; record neither credentials nor
private paths/session IDs in published evidence. No global configuration change
is needed.

```bash
omp --print --mode json --no-tools --no-extensions --no-skills \
  --no-rules --no-title --no-lsp --no-pty --max-time 120 \
  --system-prompt 'Reply to the synthetic test exactly as requested. Do not access files or network except inference.' \
  'Reply with exactly OMP_HEADLESS_PROOF' > inference.jsonl
```

Read the emitted native `type=session` header to obtain its `id` and `cwd`.
Resolve the newly created canonical JSONL under
`~/.omp/agent/sessions/<project-bucket>/` and verify its native session header
matches those fields. Set `session_file` to that verified absolute path, privately.
An assistant's response text is an inference anchor, never a session identity.
Retain the original provider-written JSONL unchanged; stdout is not a substitute
for the canonical session file.

For native tool evidence, resume that same session with only the native `bash`
tool allowed. The scope below authorizes only two synthetic `printf` calls:
success (exit 0) and controlled failure (exit 7). Use `--auto-approve` only with
operator authorization for this exact bounded scope.

```bash
omp --print --mode json --resume "$session_file" --tools bash \
  --no-extensions --no-skills --no-rules --no-title --no-lsp --no-pty \
  --auto-approve --max-time 120 \
  --system-prompt 'Use only native bash for exactly the two commands authorized in the prompt. Do not access files, use networking, or run other commands. Report their actual outputs and exit codes.' \
  'Execute exactly two separate native bash calls, once each, in the current working directory: first printf "OMP_NATIVE_TOOL_OK\n"; exit 0 ; second printf "OMP_NATIVE_TOOL_ERROR\n"; exit 7 . Then summarize.' \
  > tools.jsonl
```

Verify the canonical log contains the native call/result pairs and actual result
exit codes, including 7; model summaries alone do not establish tool outcomes.
The tool's expected exit 7 is distinct from the outer OMP process exit status.

Consume this actual source using the candidate binary in a separate private
Backscroll environment. Set `backscroll_bin` to its absolute path first. Create
a fresh private root and an empty working directory so local `backscroll.toml`
cannot widen the input scope:

```bash
proof_root="$(mktemp -d)"
mkdir -p "$proof_root"/{home,config,db,work}
cd "$proof_root/work"
private_backscroll() {
  env -i PATH=/usr/bin:/bin HOME="$proof_root/home" \
    BACKSCROLL_CONFIG_DIR="$proof_root/config" \
    BACKSCROLL_DATABASE_PATH="$proof_root/db/index.db" \
    "$backscroll_bin" "$@"
}
private_backscroll presets install --config-home "$proof_root/config"
```

Edit only the freshly installed private manifests under
`$proof_root/config/backscroll/inputs`: keep `omp` active and set the other
presets inactive. In `omp.inputs.toml`, retain the shipped `omp` decoder and
reasoning choice, set `roots` to the absolute canonical sessions directory, and
set `include` to only the verified `<project-bucket>/<session-file>.jsonl`
relative path. This preserves native discovery depth without copying or editing
provider records. Inspect the private resolved configuration before proceeding.
This private index is independent of normal active inputs; it is not a way to
filter startup ingestion in the operator's normal environment.

```bash
private_backscroll config --json
private_backscroll status --json
private_backscroll list --all-projects --json
private_backscroll search --text OMP_HEADLESS_PROOF --all-projects --json --fields full
private_backscroll search --text OMP_NATIVE_TOOL_OK --all-projects --robot
# Set project to the actual project returned by list, not an assumed cwd label.
private_backscroll patterns --kind commands --project "$project" --json
private_backscroll patterns --kind failures --project "$project" --json
private_backscroll patterns --kind sequences --project "$project" --min-support 1 --min-length 2 --json
# Set uuid to an actual returned native-block UUID, not assistant-generated text.
private_backscroll context --uuid "$uuid" --before 2 --after 2 --json
private_backscroll context --uuid "$uuid" --before 2 --after 2 --robot
private_backscroll status --json
```

Compare returned UUIDs, roles/origins, cwd/project ownership, call/result pairing
and exit 7 with the canonical source. Repeated startups must preserve identities
and counts. If testing reasoning replay, toggle only the private OMP manifest's
`index_reasoning`, repeat recall/pattern checks, then restore its original value.
Record source preservation and exact commands/results privately.

Issue #205 acceptance covers isolated mixed-provider native coexistence and the
actual OMP headless source/private candidate-consumer proof. The real OMP 18.12.6
source and candidate consumer proof passed independently: eight native blocks,
two Bash calls, preserved identities/origins/cwd, expected failure exit 7,
JSON/robot recall, context, patterns and replay. See the
[published headless proof](https://github.com/pablontiv/backscroll/issues/205#issuecomment-6330448280)
for current evidence alongside the mixed-provider regression above.

Global installed mixed-target verification is a separate follow-up scope, not
established by this private proof. Its prerequisite is recovery of the original
four missing native OpenCode session records referenced by 52 orphan messages
and 246 parts; those original records remain unavailable after the reachable
backups and exports were exhausted. Preserve production active inputs and
provider data while that prerequisite is unresolved. Once the original records
are recovered, the distinct global check requires the authorized installed
binary and its complete active-input scope to synchronize successfully, followed
by the bounded recall and tool-pattern checks above with recorded ownership,
identities and counts. Ordinary production startup processes every active input;
the private OMP proof does not establish that global result.
