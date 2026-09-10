# Install the exact local commit

Run this only after the intended change has been merged and the local branch
contains that approved commit. This source-build path installs locally; it does
not tag, release, publish, deploy, or change Codex configuration.

From the clean Backscroll repository root, with Go installed:

```bash
# Stop if tracked files or the index differ from HEAD.
git diff --quiet && git diff --cached --quiet || exit 1
expected_commit="$(git rev-parse HEAD)"
install_dir="${BACKSCROLL_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"
go build -buildvcs=true -o "$install_dir/backscroll" ./cmd/backscroll

# Match the binary's runtime manifest directory, including on macOS.
config_dir="${BACKSCROLL_CONFIG_DIR:-$HOME/.config}"
mkdir -p "$config_dir/backscroll/inputs"
if [ ! -e "$config_dir/backscroll/inputs/codex.inputs.toml" ]; then
  cp inputs/codex.inputs.toml "$config_dir/backscroll/inputs/codex.inputs.toml"
fi

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

A pre-existing Codex manifest is preserved, so inspect `backscroll config --json`
and confirm it is active with the intended roots. For custom `CODEX_HOME`, edit
only the Backscroll manifest roots. See [Codex verification and limits](../input-contract.md#complete-codex-example).
Operational Backscroll commands synchronize its SQLite index; they never modify
the Codex transcript store.

The task worker validates the build and version commands using a destination
inside its isolated copy. Installation into the operator's actual `~/.local/bin`
and active input directory is a separate, post-merge authorized operation.
