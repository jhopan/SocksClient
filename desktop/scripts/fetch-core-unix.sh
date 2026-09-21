#!/usr/bin/env bash
# Fetch the minimal sing-box core for Linux/macOS (same `core` release as the
# Windows client) and extract the plain binary.
#
# Usage:
#   SUFFIX=linux-amd64.tar.gz   desktop/scripts/fetch-core-unix.sh /tmp/out
#   SUFFIX=darwin-arm64.tar.gz  desktop/scripts/fetch-core-unix.sh /tmp/out
#   ASSET=sing-box-v1.14.1-darwin-arm64.tar.gz ... (lewati pencarian nama)
set -euo pipefail

DEST="${1:-$(cd "$(dirname "$0")/.." && pwd)/embed-unix}"
GH_REPO="${GH_REPO:-jhopan/SocksClient}"
TAG="${TAG:-core}"
SUFFIX="${SUFFIX:-linux-amd64.tar.gz}"

mkdir -p "$DEST"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -z "${ASSET:-}" ]; then
  if command -v gh >/dev/null 2>&1; then
    ASSET="$(gh release view "$TAG" -R "$GH_REPO" --json assets \
      --jq "[.assets[].name | select(endswith(\"$SUFFIX\"))] | first // empty" 2>/dev/null || true)"
  fi
fi
if [ -z "${ASSET:-}" ]; then
  echo "tidak ada asset dengan akhiran $SUFFIX di release $TAG ($GH_REPO)" >&2
  exit 1
fi

url="https://github.com/${GH_REPO}/releases/download/${TAG}/${ASSET}"
echo "mengambil $url"
curl -fsSL -o "$tmp/core.tar.gz" "$url"
tar -xzf "$tmp/core.tar.gz" -C "$tmp"
name="$(basename "$SUFFIX" .tar.gz)"
out="$DEST/sing-box-$name"
mv -f "$tmp/sing-box" "$out"
chmod +x "$out"
echo "core siap: $out"
