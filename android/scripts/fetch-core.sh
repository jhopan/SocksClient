#!/usr/bin/env bash
# Fetch libbox.aar (built by .github/workflows/build-core.yml) into
# android/app/libs/libbox.aar. The AAR is not tracked in git.
set -euo pipefail

GH_REPO="${GH_REPO:-jhopan/SocksClient}"
TAG="${TAG:-core}"
DEST="${DEST:-$(cd "$(dirname "$0")/.." && pwd)/app/libs}"

mkdir -p "$DEST"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="https://github.com/${GH_REPO}/releases/download/${TAG}/libbox.aar"
echo "fetching $url"
curl -fsSL -o "$DEST/libbox.aar" "$url"
ls -l "$DEST/libbox.aar"
