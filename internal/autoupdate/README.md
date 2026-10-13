# Private release updater

This is an independent, narrow implementation of the release contract observed
in the pinned `github.com/pablontiv/picokit@v0.5.1/autoupdate`. It does not copy,
fork, replace, or shim the picokit module. Picokit remains a dependency for
`hashfile`, output formatting, and path security. Its `hashfile.WriteAtomic`
propagates open/copy/close/rename errors and removes incomplete temporary writes.

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
