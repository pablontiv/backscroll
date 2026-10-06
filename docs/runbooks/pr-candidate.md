# Pull request candidate artifacts

Pull request candidates are automatic CI snapshots and required review gates.
They are not GitHub Releases, do not create tags, and expire after seven days.

## Automatic candidate gate

Every pull request first runs the normal `ci`, `gitleaks`, and `skill-install`
gates. The `candidate-build` job then calls Crossbeam's reusable Go candidate
workflow from its full stable merge SHA
`d8c1cd73ac11e43c2079147f61a86f0e5ceaed65` (Crossbeam v2.1.0/v2). It passes
the exact source repository, pull request head SHA, base SHA, pull request
number, binary name, and Go patch version.

Crossbeam validates the pull request and base graph, reads the release
configuration from the trusted base SHA, builds the untrusted source in a
separate job, statically validates all archives, runs its isolated Linux amd64
smoke, and publishes only the validated final artifact. Backscroll's local
`candidate` job runs with `always()`, requires every preceding gate and
`candidate-build` to have succeeded, and remains the required check name. It
validates the artifact ID, digest, name, and source SHA through the GitHub REST
API, downloads that exact artifact ID, verifies its inventory and hashes, and
runs `validate --json` with fresh isolated SQLite state.

A later push emits a `synchronize` event and repeats this flow for the new head
SHA. The latest `candidate` check must pass before the pull request is merged.
The workflow also supports public fork pull requests with read-only access. It
does not use repository secrets, OIDC, or publishing credentials.

## Download and verify

Open the successful **CI** workflow run, find **Artifacts**, and download the
artifact named `go-candidate-pr-<PR>-<full-SHA>`. From the CLI, the equivalent
is:

```sh
gh run download RUN_ID \
  --name "go-candidate-pr-PR_NUMBER-FULL_SHA" \
  --dir backscroll-candidate
cd backscroll-candidate
```

Confirm that `candidate.json` names the intended PR and exact head SHA before
running any binary. Candidate versions have the SemVer-compatible form
`<next-patch>-pr.<PR_NUMBER>.g<SHORT_SHA>`; the `g` keeps an all-numeric short
SHA with a leading zero from becoming an invalid numeric prerelease identifier.

```sh
jq . candidate.json
jq -e \
  --argjson pr PR_NUMBER \
  --arg sha FULL_SHA \
  '.schema == 1 and
   .pr == $pr and .sha == $sha and
   (.base | length == 40) and
   (.base_version | length > 0) and
   (.archives | length == 6) and
   (.candidate_version |
     test(
       "^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\." +
       "(0|[1-9][0-9]*)-pr\\.[1-9][0-9]*\\.g[0-9a-f]{7}$"
     )) and
   (.candidate_version | contains("dev") | not)' candidate.json
```

Verify every packaged archive against the uploaded checksum file:

```sh
sha256sum --check checksums.txt       # Linux
# or
shasum -a 256 --check checksums.txt   # macOS
```

A complete artifact has exactly eight files: `candidate.json`, `checksums.txt`,
and six archives for Linux, macOS, and Windows on amd64 and arm64. The reusable
workflow validates the matrix, checksums, archive contents, exact compiler and
VCS revision, and non-development candidate version before final publication.
The local `candidate` check independently verifies the final allowlist,
manifest names and hashes, and checksum file.

## Install side by side

Never extract or copy a candidate over `~/.local/bin/backscroll` or another
stable installation. Give each candidate its own directory and invoke it by its
full path:

```sh
version=$(jq -r .candidate_version candidate.json)
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

The required CI wrapper uses a fresh database and empty environment beneath a
temporary root. For an operator smoke against representative data, never point
a candidate at the live database. Make a transactionally consistent copy with
SQLite's backup command; unlike copying only the main file, this also works when
the source uses WAL:

```sh
source_db="$HOME/.backscroll.db"       # adjust to the active configured path
smoke_root=$(mktemp -d)
smoke_db="$smoke_root/db/backscroll.db"
mkdir -p \
  "$smoke_root/home" \
  "$smoke_root/config" \
  "$smoke_root/cache" \
  "$smoke_root/data" \
  "$smoke_root/state" \
  "$smoke_root/db" \
  "$smoke_root/tmp" \
  "$smoke_root/work"
sqlite3 "$source_db" ".backup '$smoke_db'"

output=$(cd "$smoke_root/work" && env -i \
  PATH=/usr/bin:/bin \
  HOME="$smoke_root/home" \
  TMPDIR="$smoke_root/tmp" \
  XDG_CONFIG_HOME="$smoke_root/config" \
  XDG_CACHE_HOME="$smoke_root/cache" \
  XDG_DATA_HOME="$smoke_root/data" \
  XDG_STATE_HOME="$smoke_root/state" \
  BACKSCROLL_CONFIG_DIR="$smoke_root/config" \
  BACKSCROLL_DATABASE_PATH="$smoke_db" \
  "$candidate_root/backscroll" validate --json)
printf '%s\n' "$output" | jq -e '.valid == true'
test -s "$smoke_db"
```

The copied database is disposable: startup may migrate or synchronize it. Keep
all candidate HOME, temporary, XDG, and database paths under the smoke root so
the stable installation and live data remain untouched. To reproduce the CI
case instead, omit the SQLite backup and let `validate --json` create the fresh
database at the isolated path.

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
