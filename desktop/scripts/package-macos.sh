#!/usr/bin/env bash
# Paket macOS untuk socksctl: satu tarball universal (Intel + Apple Silicon).
# Dijalankan di CI (macos-14) atau lokal di macOS.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DESKTOP="$(cd "$HERE/.." && pwd)"
OUT="${OUT:-$DESKTOP/dist}"
VERSION="${VERSION:-$(grep 'appVersion' "$DESKTOP/main.go" | head -1 | sed 's/.*"\(.*\)".*/\1/')}"
mkdir -p "$OUT/bin" "$OUT/core"

echo "== build socksgui (AppKit) $VERSION (darwin amd64 + arm64)"
for arch in amd64 arm64; do
  (cd "$DESKTOP" && GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksgui-$arch" ./cmd/socksgui-mac)
done

echo "== build socksctl $VERSION (darwin amd64 + arm64)"
for arch in amd64 arm64; do
  (cd "$DESKTOP" && GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksctl-$arch" ./cmd/socksctl)
done

echo "== ambil core darwin (amd64 + arm64, dari release core)"
for arch in amd64 arm64; do
  SUFFIX="darwin-$arch.tar.gz" bash "$HERE/fetch-core-unix.sh" "$OUT/core"
done

# Universal binary: satu file untuk Intel maupun Apple Silicon.
lipo -create "$OUT/bin/socksgui-amd64" "$OUT/bin/socksgui-arm64" -output "$OUT/bin/socksgui-universal"
lipo -create "$OUT/bin/socksctl-amd64" "$OUT/bin/socksctl-arm64" -output "$OUT/bin/socksctl-universal"
lipo -create "$OUT/core/sing-box-darwin-amd64" "$OUT/core/sing-box-darwin-arm64" -output "$OUT/core/sing-box-universal"
chmod +x "$OUT/bin/socksctl-universal" "$OUT/core/sing-box-universal"
lipo -info "$OUT/bin/socksctl-universal"

pkg="$OUT/macos-$VERSION"
rm -rf "$pkg"
mkdir -p "$pkg/SocksClient"
install -m 0755 "$OUT/bin/socksctl-universal" "$pkg/SocksClient/socksctl"
install -m 0755 "$OUT/bin/socksgui-universal" "$pkg/SocksClient/socksgui"
install -m 0755 "$OUT/core/sing-box-universal" "$pkg/SocksClient/sing-box"

app="$pkg/SocksClient.app"
mkdir -p "$app/Contents/MacOS"
install -m 0755 "$OUT/bin/socksgui-universal" "$app/Contents/MacOS/socksgui"
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>Socks Client</string>
    <key>CFBundleDisplayName</key><string>Socks Client</string>
    <key>CFBundleIdentifier</key><string>my.id.jhopanstore.socksclient</string>
    <key>CFBundleExecutable</key><string>socksgui</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleShortVersionString</key><string>$VERSION</string>
    <key>CFBundleVersion</key><string>$VERSION</string>
    <key>LSMinimumSystemVersion</key><string>11.0</string>
    <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST
printf 'APPL????' > "$app/Contents/PkgInfo"
echo "app bundle siap: $app"

cat > "$pkg/SocksClient/install.sh" <<'EOS'
#!/bin/sh
# Pasang ke /usr/local (butuh sudo).
set -e
dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
sudo install -m 0755 "$dir/socksctl" /usr/local/bin/socksctl
sudo mkdir -p /usr/local/lib/socksclient
sudo install -m 0755 "$dir/sing-box" /usr/local/lib/socksclient/sing-box
if [ -f "$dir/socksgui" ]; then
  sudo install -m 0755 "$dir/socksgui" /usr/local/bin/socksgui
  echo "GUI AppKit terpasang: sudo socksgui"
fi
if [ -d "$dir/../SocksClient.app" ]; then
  sudo rm -rf /Applications/SocksClient.app
  sudo cp -R "$dir/../SocksClient.app" /Applications/
  echo "SocksClient.app disalin ke /Applications"
fi
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
# Klik dua kali: meminta password admin, lalu menjalankan GUI AppKit.
dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
if [ -x "$dir/socksgui" ]; then
  osascript -e "do shell script \"$dir/socksgui\" with administrator privileges"
  exit 0
fi
URL="http://127.0.0.1:17800/"
osascript -e "do shell script "$dir/socksctl gui -listen 127.0.0.1:17800" with administrator privileges" &
sleep 3
open "$URL"
EOS
chmod 0755 "$pkg/SocksClient/SocksClient.command"

cat > "$pkg/SocksClient/README-macos.txt" <<'EOS'
Socks Client - macOS
====================

Isi:
  SocksClient.app      GUI AppKit (universal: Intel + Apple Silicon, ~5,5 MB)
  SocksClient/socksgui GUI yang sama sebagai binary lepas
  SocksClient/socksctl CLI (TUN di terminal + UI browser)
  SocksClient/sing-box core minimal
  install.sh, SocksClient.command

Pakai cepat:
  - GUI: klik dua kali SocksClient.command (minta password admin, lalu jendela
    GUI terbuka), atau dari terminal:  sudo ./SocksClient/socksgui
  - Tanpa GUI:  sudo ./SocksClient/socksctl up -host 10.0.0.1 -port 1080
  - Pasang permanen:  ./SocksClient/install.sh  (menyalin socksgui ke
    /usr/local/bin dan SocksClient.app ke /Applications)

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

# Dibuat dari dalam folder paket (BSD tar macOS tidak perlu --transform, dan
# opsinya harus sebelum operand - pelajaran dari run pertama).
(cd "$pkg" && tar -czf "$OUT/SocksClient-macos-universal-${VERSION}.tar.gz" SocksClient SocksClient.app)
echo "isi arsip:"
tar -tzf "$OUT/SocksClient-macos-universal-${VERSION}.tar.gz" | head -12
echo "== checksum"
# macOS tidak punya sha256sum (BSD: shasum -a 256). Format keluarannya sama,
# jadi file ini tetap bisa diverifikasi dengan `sha256sum -c` di Linux/Windows.
sha() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}
(cd "$OUT" && sha SocksClient-macos-universal-*.tar.gz > SHA256SUMS-macos.txt && cat SHA256SUMS-macos.txt)
