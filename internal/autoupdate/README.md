# Private release updater

Release semantics and attribution are based on
`github.com/pablontiv/picokit@v1.0.0/autoupdate`, licensed Apache-2.0
(copyright Pablo Ontiveros). The repository's Apache-2.0 LICENSE applies; source
provenance is also recorded in NOTICE. No PolyForm-only source is incorporated.
The earlier v0.5.1 reference is superseded by the Apache release. This private
implementation does not fork/replace/shim the upstream module. Picokit v1.0.0
remains a dependency for output formatting and path security, but its fixed-name
`hashfile.WriteAtomic` is NOT used for updater publication.

## Why private

The pinned updater suppresses network/API/download/cache/staging errors and all
apply/replace/reexec errors. Released upstream APIs do not expose these errors to
the caller. A warning wrapper cannot recover discarded errors. Backscroll owns
this small backend so normal stderr warnings reflect production failures, not
just injected failures above the swallowing boundary.

## Preserved release contract

- Discovery: `https://api.github.com/repos/pablontiv/backscroll/releases/latest`,
  JSON `tag_name`.
- Numeric major/minor/patch ordering, stripping leading `v` and prerelease/build
  suffixes, as in the pinned updater.
- Assets: `backscroll_<version>_<goos>_<goarch>.tar.gz`, or `.zip` on Windows;
  `checksums.txt` lines contain SHA-256 followed by the exact archive name.
- Downloads: `https://github.com/pablontiv/backscroll/releases/download/<tag>/`.
- Staging: OS user cache directory, `backscroll/staged/<tag>/backscroll`
  (`backscroll.exe` on Windows), with executable mode `0755`.
- SHA-256 is checked before extraction; the binary member is matched by basename.
  Extraction writes atomically and rejects non-regular members/candidates.
- Unix replacement uses same-filesystem rename, then `syscall.Exec` with original
  arguments/environment. Windows moves the running executable aside, installs
  the prepared copy, restores the old executable if installation fails, then
  spawns with original arguments and inherited environment/stdio and exits.
- Updates are mandatory, without a runtime opt-out; `dev` identity alone is exempt.

## Explicit improvements

Every network, API/decode, download/read, checksum, cache, staging, extraction,
replacement, and reexec failure reaches the caller. Missing staged directories,
no valid newer candidate, no newer release, and already-staged regular binaries
are normal success. Requests use the supplied context and a default 60s HTTP
timeout per request. The command layer cancels this context on cancellation,
the post-command 10s staging cap, or return.

Apply prepares a separate temporary copy beside the installed executable rather
than consuming the staged candidate. Thus replacement and reexec failures retain
a valid candidate for retry. The installed binary may already be updated if
reexec fails; the error is visible and the existing process can continue its
command. Successful real reexec does not return.

`Options` exposes explicit HTTP/cache/executable/replace/reexec boundaries for
safe tests. Production uses their default implementations. Tests use temporary
install fixtures and fake transports or a loopback HTTP server only: no live
GitHub access, process replacement, real HOME, or real installation.

## Publication, cancellation, and limits

Each publication owns an O_EXCL/CreateTemp file in the destination directory,
mode 0600 during copying. Context-aware reads and writes complete before chmod,
file Sync, checked Close, an identity check, the final context check, and atomic
rename. Only the attempt's own temporary path is cleaned up. No staging worker
is spawned inside FetchAndStage; the command layer cancels and joins the concrete
backend on its 10s cap/cancellation, including extraction/copy cleanup. The 10s
staging wait can be followed by bounded cancellation cleanup; the command never
returns while the production backend can still mutate staging. Generic injected
non-cooperative test callbacks are not represented as this production guarantee.
There is no directory-fsync/power-loss-durability claim.

Hard byte limits: latest JSON 1 MiB; checksums 4 MiB; compressed archive 128 MiB;
expanded tar scan/gzip stream or summed zip member sizes 512 MiB; target/prepared
executable 128 MiB. Reads allow only one sentinel byte beyond each budget before
returning an explicit limit error. These bounds comfortably exceed current Go
CLI release artifacts (roughly 20 MiB); increasing them requires evidence and
boundary/bomb tests. Archive checksum verification and normal release names are
unchanged. Context reaches HTTP reads, archive iteration/decompression, copying,
and publication.

Candidate and prepared paths are checked with Lstat; opened handles are checked
with f.Stat and SameFile and copied from that handle. Observed symlinks/nonregular
files and identity changes are rejected. These portable checks are NOT atomic
O_NOFOLLOW and do not eliminate adversarial parent-directory or final-name races.
Cache and installation directories must remain trusted to the user. Concurrent
publishers can replace only complete candidates; a benign candidate-identity
change can return a retryable-by-next-invocation warning.

Windows backup removal after a successful install is best-effort and never
suppresses reexec. A mapped backup remains at `.old`; the next process's apply
startup retries removal even when there is no newer candidate. A still-existing
backup prevents another replacement, visibly, rather than overwriting it.

## Maintenance checklist

1. Compare Apache upstream discovery URLs, numeric version policy, archive and
   checksum names, cache layout, and Unix/Windows argv/environment/stdio semantics.
2. Keep fixtures and production-adapter tests pinned to those public semantics.
3. Preserve private divergences: observable errors, mandatory/no opt-out, unique
   synced publication, context propagation/join, conservative byte limits,
   portable symlink/identity checks, staged retention, Windows deferred cleanup.
4. Verify upstream license/NOTICE when updating the reference; do not import
   PolyForm-only code or reintroduce shared predictable temporary files.
5. Run independent interleaving/cancellation probes, multiprocess publication,
   package race/coverage, affected picokit-consumer tests, vet, module verify,
   and Windows/Linux cross-compilation. Native Windows execution still needs
   a Windows host; cross-compilation is not a runtime proof.
