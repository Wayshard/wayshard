#!/usr/bin/env bash
# Sign SHA256SUMS.txt with a maintainer-owned minisign key (offline keypair).
set -euo pipefail
SUMS="${1:?usage: minisign-sign.sh <SHA256SUMS.txt>}"
test -s "$SUMS"

need() {
  local n="$1"
  if [[ -z "${!n:-}" ]]; then
    echo "missing required GitHub Environment secret $n" >&2
    exit 1
  fi
}
need WAYSHARD_RELEASE_MINISIGN_KEY_BASE64
need WAYSHARD_RELEASE_MINISIGN_PASSWORD

if ! command -v minisign >/dev/null 2>&1; then
  echo "minisign is required on the runner" >&2
  exit 1
fi

WORKDIR="$(mktemp -d "${RUNNER_TEMP:-/tmp}/wayshard-minisign.XXXXXX")"
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT
umask 077
python3 - "$WORKDIR/minisign.key" <<'PY'
import base64, os, sys
open(sys.argv[1], "wb").write(base64.b64decode(os.environ["WAYSHARD_RELEASE_MINISIGN_KEY_BASE64"]))
PY

python3 - "$WORKDIR/minisign.key" "$SUMS" <<'PY'
"""Feed minisign the key password without a TTY helper."""
import os, pty, select, sys, time
key, sums = sys.argv[1], sys.argv[2]
pw = os.environ["WAYSHARD_RELEASE_MINISIGN_PASSWORD"]
comment = os.environ.get("GITHUB_REF_NAME", "release")
pid, fd = pty.fork()
if pid == 0:
    os.execvp("minisign", ["minisign", "-S", "-s", key, "-m", sums, "-t", comment])
sent = False
deadline = time.time() + 60
buf = b""
while time.time() < deadline:
    r, _, _ = select.select([fd], [], [], 0.5)
    if fd in r:
        try:
            chunk = os.read(fd, 4096)
        except OSError:
            break
        if not chunk:
            break
        buf += chunk
        if (not sent) and (b"Password:" in buf):
            os.write(fd, pw.encode() + b"\n")
            sent = True
            buf = b""
# Secret-key KDF is intentionally slow; drain output and wait for exit.
end = time.time() + 60
while time.time() < end:
    r, _, _ = select.select([fd], [], [], 0.25)
    if fd in r:
        try:
            os.read(fd, 4096)
        except OSError:
            pass
    wpid, status = os.waitpid(pid, os.WNOHANG)
    if wpid == pid:
        if os.WEXITSTATUS(status) != 0:
            raise SystemExit(f"minisign -S failed with {status}")
        break
else:
    os.kill(pid, 9)
    os.waitpid(pid, 0)
    raise SystemExit("minisign -S timed out")
PY

test -s "${SUMS}.minisig"
echo "signed ${SUMS}.minisig"
