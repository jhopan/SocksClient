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
PATTERN="${PATTERN:-windows-amd64}"

if [ -z "${ASSET:-}" ]; then
  if command -v gh >/dev/null 2>&1; then
    ASSET="$(gh release view "$TAG" -R "$GH_REPO" --json assets \
      --jq "[.assets[].name | select(test(\"$PATTERN\"))] | first // empty" 2>/dev/null || true)"
  fi
fi
ASSET="${ASSET:-sing-box-v1.14.1-windows-amd64.zip}"

mkdir -p "$DEST"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="https://github.com/${GH_REPO}/releases/download/${TAG}/${ASSET}"
echo "fetching $url"
curl -fsSL -o "$tmp/core.zip" "$url"
unzip -o -q "$tmp/core.zip" -d "$tmp"
mv -f "$tmp/sing-box.exe" "$DEST/sing-box.exe"
echo "core ready: $DEST/sing-box.exe"
"$DEST/sing-box.exe" version || true
