#!/usr/bin/env bash
# Fetch libbox.aar (built by .github/workflows/build-core.yml) into
# android/app/libs/libbox.aar. The AAR is not tracked in git.
set -euo pipefail

GH_REPO="${GH_REPO:-jhopan/SocksClient}"
TAG="${TAG:-core}"
DEST="${DEST:-$(cd "$(dirname "$0")/.." && pwd)/app/libs}"

# curl is a native binary on Windows: hand it a native path, not /c/...
to_native() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1" | tr '\\' '/'
  else
    printf '%s' "$1"
  fi
}

mkdir -p "$DEST"
url="https://github.com/${GH_REPO}/releases/download/${TAG}/libbox.aar"
echo "fetching $url"
curl -fsSL -o "$(to_native "$DEST/libbox.aar")" "$url"
ls -l "$DEST/libbox.aar"
