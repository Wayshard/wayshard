#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
if ! command -v minisign >/dev/null 2>&1; then
  echo "minisign not installed; skipping live sign/verify (CI ubuntu installs it)"
  exit 0
fi
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
python3 - "$TMP" <<'PY'
import os, pty, re, select, sys, time

d = sys.argv[1]
os.chdir(d)
pid, fd = pty.fork()
if pid == 0:
    os.execvp("minisign", ["minisign", "-G", "-p", "p.pub", "-s", "p.sec"])

prompt = re.compile(br"Password(?: \(one more time\))?:")
sent = 0
buf = b""
deadline = time.time() + 60
while time.time() < deadline and sent < 2:
    r, _, _ = select.select([fd], [], [], 0.5)
    if fd not in r:
        continue
    try:
        chunk = os.read(fd, 4096)
    except OSError:
        break
    if not chunk:
        break
    buf += chunk
    if prompt.search(buf):
        os.write(fd, b"test-pass\n")
        sent += 1
        buf = b""

# Secret-key KDF is intentionally slow; drain output and wait for exit.
deadline = time.time() + 60
while time.time() < deadline:
    r, _, _ = select.select([fd], [], [], 0.25)
    if fd in r:
        try:
            os.read(fd, 4096)
        except OSError:
            pass
    wpid, status = os.waitpid(pid, os.WNOHANG)
    if wpid == pid:
        if os.WEXITSTATUS(status) != 0:
            raise SystemExit(f"minisign -G exited {status}")
        break
else:
    os.kill(pid, 9)
    os.waitpid(pid, 0)
    raise SystemExit("minisign -G timed out")
PY
test -s "$TMP/p.pub"
test -s "$TMP/p.sec"
echo hello > "$TMP/SHA256SUMS.txt"
export WAYSHARD_RELEASE_MINISIGN_KEY_BASE64
WAYSHARD_RELEASE_MINISIGN_KEY_BASE64="$(python3 -c 'import base64,sys; print(base64.b64encode(open(sys.argv[1],"rb").read()).decode())' "$TMP/p.sec")"
export WAYSHARD_RELEASE_MINISIGN_PASSWORD=test-pass
bash "$ROOT/scripts/release/minisign-sign.sh" "$TMP/SHA256SUMS.txt"
bash "$ROOT/scripts/release/minisign-verify.sh" "$TMP/SHA256SUMS.txt" "$TMP/p.pub"
echo "minisign sign/verify test ok"
