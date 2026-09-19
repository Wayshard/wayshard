#!/usr/bin/env bash
# Generate a CycloneDX 1.5 JSON SBOM for the Wayshard Go module.
# This is the official release SBOM. Do not treat `go version -m` as an SBOM.
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
  "$ROOT"

test -s "$OUT"
python3 - "$OUT" <<'PY'
import json, sys
p = sys.argv[1]
with open(p, encoding="utf-8") as f:
    doc = json.load(f)
bom = doc.get("bomFormat") or doc.get("bomformat")
if bom != "CycloneDX":
    raise SystemExit(f"{p} is not CycloneDX (bomFormat={bom!r})")
if "components" not in doc and "metadata" not in doc:
    raise SystemExit(f"{p} missing CycloneDX components/metadata")
print(f"validated CycloneDX SBOM {p}")
PY

# Separate Go buildinfo (not an SBOM).
if [[ -f bin/wayshard-server-linux-amd64 ]]; then
  go version -m bin/wayshard-server-linux-amd64 \
    > "$OUTDIR/wayshard-${VERSION}-buildinfo-server-linux-amd64.txt"
fi

echo "SBOM written to $OUT"
