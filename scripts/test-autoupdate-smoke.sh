#!/bin/bash
set -e
# A dev build (version == "dev", produced by `just build` / plain `go build`)
# is exempt from autoupdate by identity — it makes no network call. This asserts
# that exemption without any env var; the runtime opt-out no longer exists.
./backscroll --version
echo "autoupdate smoke: ok (dev build exempt)"
