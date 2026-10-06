# Pull request candidate artifacts

Pull request candidates are opt-in CI snapshots for review. They are not GitHub
Releases, do not create tags, and expire after seven days.

## Trigger a candidate build

Add the `build-candidate` label to an open pull request. CI first runs the normal
`ci`, `gitleaks`, and `skill-install` gates, then builds the exact pull request
head SHA. A later push to the pull request reruns the candidate job while the
label remains present. Remove the label when candidate builds are no longer
needed.

The workflow also runs for fork pull requests with read-only repository access.
It does not use repository secrets, OIDC, or publishing credentials.

## Download and verify

Open the successful **CI** workflow run, find **Artifacts**, and download the
artifact named `backscroll-pr-<PR>-<full-SHA>`. From the CLI, the equivalent is:

```sh
gh run download RUN_ID \
  --name "backscroll-pr-PR_NUMBER-FULL_SHA" \
  --dir backscroll-candidate
cd backscroll-candidate
```

Confirm that `candidate.json` names the intended PR and exact head SHA before
running any binary:

```sh
jq . candidate.json
jq -e \
  --argjson pr PR_NUMBER \
  --arg sha FULL_SHA \
  '.pr == $pr and .sha == $sha and
   (.base_sha | length == 40) and
   (.base_version | length > 0) and
   (.version | contains("-pr.")) and
   (.version | contains("dev") | not)' candidate.json
```

Verify every packaged archive against the uploaded checksum file:

```sh
sha256sum --check checksums.txt       # Linux
# or
shasum -a 256 --check checksums.txt   # macOS
```

A complete artifact has six archives: Linux, macOS, and Windows, each for amd64
and arm64. The workflow checks all six, the checksums, the non-development
candidate version, and the embedded `vcs.revision` before upload.

## Install side by side

Never extract or copy a candidate over `~/.local/bin/backscroll` or another
stable installation. Give each candidate its own directory and invoke it by its
full path:

```sh
version=$(jq -r .version candidate.json)
sha=$(jq -r .sha candidate.json)
candidate_root="$HOME/.local/lib/backscroll-candidates/${version}-${sha}"
mkdir -p "$candidate_root"
tar -xzf "backscroll_${version}_$(go env GOOS)_$(go env GOARCH).tar.gz" \
  -C "$candidate_root"
"$candidate_root/backscroll" --version
```

For Windows, extract the matching zip into a similarly isolated directory and
invoke `backscroll.exe` there. Do not add a candidate directory ahead of the
stable binary in a persistent `PATH`.

## Smoke test with an isolated SQLite copy

Do not point a candidate at the live database. Make a transactionally consistent
copy with SQLite's backup command; unlike copying only the main file, this also
works when the source uses WAL:

```sh
source_db="$HOME/.backscroll.db"       # adjust to the active configured path
smoke_root=$(mktemp -d)
smoke_db="$smoke_root/backscroll.db"
sqlite3 "$source_db" ".backup '$smoke_db'"

mkdir -p "$smoke_root/home" "$smoke_root/config" "$smoke_root/cache"
HOME="$smoke_root/home" \
XDG_CONFIG_HOME="$smoke_root/config" \
XDG_CACHE_HOME="$smoke_root/cache" \
BACKSCROLL_CONFIG_DIR="$smoke_root/config" \
BACKSCROLL_DATABASE_PATH="$smoke_db" \
  "$candidate_root/backscroll" validate --json
```

The copied database is disposable: startup may migrate or synchronize it. Keep
all candidate HOME, config, cache, and database paths under the temporary smoke
directory so the stable installation and live data remain untouched.

## Autoupdate limitation and cleanup

Candidate binaries retain Backscroll's mandatory autoupdate behavior; there is
no supported environment-variable opt-out. A command may contact GitHub and
stage a newer stable release in the isolated cache. A later invocation can
replace the candidate binary in its side-by-side directory if that stable
version compares newer. Re-extract the verified archive before further
candidate testing when exact build identity matters, and check `--version`
again.

Remove downloaded archives, the side-by-side candidate directory, and the smoke
directory when testing is complete:

```sh
rm -rf "$candidate_root" "$smoke_root" backscroll-candidate
```
