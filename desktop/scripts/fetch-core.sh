#!/usr/bin/env bash
# Fetch the minimal sing-box core (built by .github/workflows/build-core.yml)
# into desktop/embed/sing-box.exe.
#
# The core is NOT tracked in git - the repo stays small and GitHub Actions is
# the only builder. The installer bundles whatever this script downloads.
#
# Usage:
#   desktop/scripts/fetch-core.sh                 # windows-amd64 (default)
#   ASSET=sing-box-v1.14.1-windows-386.zip desktop/scripts/fetch-core.sh
#   GH_REPO=jhopan/SocksClient TAG=core desktop/scripts/fetch-core.sh
set -euo pipefail

GH_REPO="${GH_REPO:-jhopan/SocksClient}"
TAG="${TAG:-core}"
DEST="${DEST:-$(cd "$(dirname "$0")/.." && pwd)/embed}"
SUFFIX="${SUFFIX:-windows-amd64.zip}"

if [ -z "${ASSET:-}" ]; then
  if command -v gh >/dev/null 2>&1; then
    ASSET="$(gh release view "$TAG" -R "$GH_REPO" --json assets \
      --jq "[.assets[].name | select(endswith(\"$SUFFIX\"))] | first // empty" 2>/dev/null || true)"
  fi
fi
ASSET="${ASSET:-sing-box-v1.14.1-windows-amd64.zip}"

# curl/unzip are native binaries on Windows: hand them native paths, not
# /c/... MSYS paths (path conversion is off in this shell).
to_native() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1" | tr '\\' '/'
  else
    printf '%s' "$1"
  fi
}

mkdir -p "$DEST"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="https://github.com/${GH_REPO}/releases/download/${TAG}/${ASSET}"
echo "fetching $url"
curl -fsSL -o "$(to_native "$tmp/core.zip")" "$url"
unzip -o -q "$(to_native "$tmp/core.zip")" -d "$(to_native "$tmp")"
mv -f "$tmp/sing-box.exe" "$DEST/sing-box.exe"
echo "core ready: $DEST/sing-box.exe"
"$DEST/sing-box.exe" version || true
