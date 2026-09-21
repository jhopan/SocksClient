#!/usr/bin/env bash
# Paket macOS untuk socksctl: satu tarball universal (Intel + Apple Silicon).
# Dijalankan di CI (macos-14) atau lokal di macOS.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DESKTOP="$(cd "$HERE/.." && pwd)"
OUT="${OUT:-$DESKTOP/dist}"
VERSION="${VERSION:-$(grep 'appVersion' "$DESKTOP/main.go" | head -1 | sed 's/.*"\(.*\)".*/\1/')}"
mkdir -p "$OUT/bin" "$OUT/core"

echo "== build socksctl $VERSION (darwin amd64 + arm64)"
for arch in amd64 arm64; do
  (cd "$DESKTOP" && GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksctl-$arch" ./cmd/socksctl)
done

echo "== ambil core darwin (amd64 + arm64, dari release core)"
for arch in amd64 arm64; do
  SUFFIX="darwin-$arch.tar.gz" bash "$HERE/fetch-core-unix.sh" "$OUT/core"
done

# Universal binary: satu file untuk Intel maupun Apple Silicon.
lipo -create "$OUT/bin/socksctl-amd64" "$OUT/bin/socksctl-arm64" -output "$OUT/bin/socksctl-universal"
lipo -create "$OUT/core/sing-box-darwin-amd64" "$OUT/core/sing-box-darwin-arm64" -output "$OUT/core/sing-box-universal"
chmod +x "$OUT/bin/socksctl-universal" "$OUT/core/sing-box-universal"
lipo -info "$OUT/bin/socksctl-universal"

pkg="$OUT/macos-$VERSION"
rm -rf "$pkg"
mkdir -p "$pkg/SocksClient"
install -m 0755 "$OUT/bin/socksctl-universal" "$pkg/SocksClient/socksctl"
install -m 0755 "$OUT/core/sing-box-universal" "$pkg/SocksClient/sing-box"

cat > "$pkg/SocksClient/install.sh" <<'EOS'
#!/bin/sh
# Pasang ke /usr/local (butuh sudo).
set -e
dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
sudo install -m 0755 "$dir/socksctl" /usr/local/bin/socksctl
sudo mkdir -p /usr/local/lib/socksclient
sudo install -m 0755 "$dir/sing-box" /usr/local/lib/socksclient/sing-box
echo "terpasang: /usr/local/bin/socksctl"
echo
echo "pakai:"
echo "  sudo socksctl up  -host <IP> -port <PORT>    # jalan di terminal"
echo "  sudo socksctl gui -host <IP> -port <PORT>    # UI di browser"
EOS
chmod 0755 "$pkg/SocksClient/install.sh"

# Launcher klik-dua-kali: minta hak admin lewat dialog macOS, lalu buka UI browser.
cat > "$pkg/SocksClient/SocksClient.command" <<'EOS'
#!/bin/sh
# Klik dua kali untuk menjalankan UI Socks Client (TUN butuh hak admin).
dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
URL="http://127.0.0.1:17800/"
osascript -e "do shell script "$dir/socksctl gui -listen 127.0.0.1:17800" with administrator privileges" &
sleep 3
open "$URL"
EOS
chmod 0755 "$pkg/SocksClient/SocksClient.command"

cat > "$pkg/SocksClient/README-macos.txt" <<'EOS'
Socks Client - macOS
====================

Isi: socksctl (universal: Intel + Apple Silicon), core sing-box minimal,
install.sh, dan SocksClient.command.

Pakai cepat (tanpa memasang apa pun):
  cd SocksClient
  sudo ./socksctl up -host 10.0.0.1 -port 1080
atau klik dua kali SocksClient.command untuk UI di browser (minta hak admin).

Pasang permanen:
  ./install.sh

Catatan penting
- TUN di macOS butuh hak root, jadi setiap perintah dijalankan dengan sudo.
- Binary ini BELUM ditandatangani/notarized (tidak ada Apple Developer ID).
  Kalau macOS menolak menjalankannya sebagai "tidak dikenal", jalankan sekali:
    xattr -dr com.apple.quarantine SocksClient
  atau klik kanan pada SocksClient.command lalu Open.
- Server wajib berupa IP (hostname akan di-resolve di luar tunnel).
- Log core muncul di terminal; hentikan dengan Ctrl+C.
EOS

(cd "$OUT" && tar -czf "SocksClient-macos-universal-${VERSION}.tar.gz" "macos-$VERSION/SocksClient" --transform "s|macos-${VERSION}/||")
echo "== checksum"
(cd "$OUT" && sha256sum SocksClient-macos-universal-*.tar.gz > SHA256SUMS-macos.txt && cat SHA256SUMS-macos.txt)
