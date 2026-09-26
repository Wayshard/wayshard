#!/usr/bin/env bash
# Generate a CycloneDX JSON SBOM for the Wayshard Go module.
# This is the official release SBOM. Do not treat `go version -m` as an SBOM.
#
# License metadata comes from cyclonedx-gomod's license detection, which scans
# the real LICENSE/COPYING files of every module in the build. Detected licenses
# are asserted into `components[].licenses`. If detection cannot identify a
# component's license the build FAILS instead of fabricating a license.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

VERSION="${1:?usage: sbom.sh <version> <outdir>}"
OUTDIR="${2:?usage: sbom.sh <version> <outdir>}"
mkdir -p "$OUTDIR"

TOOL="github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.12.0"
OUT="$OUTDIR/wayshard-${VERSION}-sbom-go.cdx.json"

echo "generating CycloneDX SBOM with $TOOL"
GOBIN="${TMPDIR:-/tmp}/wayshard-sbom-bin"
mkdir -p "$GOBIN"
GOBIN="$GOBIN" go install "$TOOL"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GOBIN/cyclonedx-gomod" app \
  -json \
  -output "$OUT" \
  -main cmd/wayshard-server \
  -licenses \
  -assert-licenses \
  "$ROOT"

test -s "$OUT"
python3 - "$OUT" <<'PY'
import json, re, sys

p = sys.argv[1]
with open(p, encoding="utf-8") as f:
    doc = json.load(f)

bom = doc.get("bomFormat") or doc.get("bomformat")
if bom != "CycloneDX":
    raise SystemExit(f"{p} is not CycloneDX (bomFormat={bom!r})")
if "components" not in doc and "metadata" not in doc:
    raise SystemExit(f"{p} missing CycloneDX components/metadata")

comps = doc.get("components", [])
if not comps:
    raise SystemExit(f"{p} has no components")

# Every component must carry a license detected from its real license file. We
# never assert a license that was not detected; an undetected license fails the
# release instead of being guessed.
unknown = []
for c in comps:
    declared = c.get("licenses") or []
    if not declared:
        unknown.append(c.get("name") or "<unnamed>")
        continue
    for entry in declared:
        lic = entry.get("license") or {}
        ident = lic.get("id") or lic.get("name")
        if not ident or not re.fullmatch(r"[A-Za-z0-9.+-]+", str(ident)):
            unknown.append(f"{c.get('name')} ({ident!r})")

if unknown:
    print("no detected license evidence for: " + ", ".join(unknown), file=sys.stderr)
    print(
        "refusing to publish an SBOM with unknown licenses; resolve the dependency "
        "license (add a mapped license or review the dependency) instead of fabricating one",
        file=sys.stderr,
    )
    raise SystemExit(1)

print(f"validated CycloneDX SBOM: {len(comps)} components, all with detected licenses")
PY

# Separate Go buildinfo (not an SBOM).
if [[ -f bin/wayshard-server-linux-amd64 ]]; then
  go version -m bin/wayshard-server-linux-amd64 \
    > "$OUTDIR/wayshard-${VERSION}-buildinfo-server-linux-amd64.txt"
fi

echo "SBOM written to $OUT"
