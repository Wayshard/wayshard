#!/usr/bin/env bash
set -euo pipefail
SUMS="${1:?usage: minisign-verify.sh <SHA256SUMS.txt> <pubkey>}"
PUB="${2:?}"
test -s "$SUMS"
test -s "${SUMS}.minisig"
test -s "$PUB"
minisign -V -p "$PUB" -m "$SUMS"
echo "minisign verified $SUMS"
