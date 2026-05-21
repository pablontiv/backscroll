#!/bin/bash
set -e
BACKSCROLL_AUTOUPDATE_DISABLE=1 ./backscroll --version
echo "autoupdate smoke: ok (disabled mode)"
