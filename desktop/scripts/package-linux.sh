#!/usr/bin/env bash
# Paket Linux untuk socksctl: .deb (amd64/arm64) + tarball.
# Dijalankan di CI (ubuntu-latest) atau lokal di Linux.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DESKTOP="$(cd "$HERE/.." && pwd)"
OUT="${OUT:-$DESKTOP/dist}"
VERSION="${VERSION:-$(grep 'appVersion' "$DESKTOP/main.go" | head -1 | sed 's/.*"\(.*\)".*/\1/')}"
mkdir -p "$OUT" "$OUT/bin" "$OUT/core"

echo "== build socksctl $VERSION"
(cd "$DESKTOP" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksctl-amd64" ./cmd/socksctl)
(cd "$DESKTOP" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksctl-arm64" ./cmd/socksctl)

# GUI GTK3: cgo hanya bisa dibangun untuk arsitektur host, jadi GUI masuk ke
# paket amd64. Paket arm64 tetap dapat CLI (+ UI browser) - dicatat di rilis.
echo "== build socksgui (GTK3, amd64)"
(cd "$DESKTOP" && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$OUT/bin/socksgui-amd64" ./cmd/socksgui-gtk)

for arch in amd64 arm64; do
  echo "== ambil core linux-$arch"
  SUFFIX="linux-$arch.tar.gz" bash "$HERE/fetch-core-unix.sh" "$OUT/core"
done

for arch in amd64 arm64; do
  pkg="$OUT/deb-$arch"
  rm -rf "$pkg"
  mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/lib/socksclient" \
           "$pkg/usr/share/applications" "$pkg/lib/systemd/system" \
           "$pkg/usr/share/doc/socksclient"

  install -m 0755 "$OUT/bin/socksctl-$arch" "$pkg/usr/bin/socksctl"
  install -m 0755 "$OUT/core/sing-box-linux-$arch" "$pkg/usr/lib/socksclient/sing-box"
  if [ "$arch" = "amd64" ]; then
    install -m 0755 "$OUT/bin/socksgui-amd64" "$pkg/usr/bin/socksgui"
  fi

  cat > "$pkg/usr/bin/socksclient-gui" <<'EOS'
#!/bin/sh
# GUI native (GTK3) dengan hak root: TUN butuh root untuk membuat interface + route.
# Kalau binary GUI tidak ada (mis. paket arm64), jatuh ke UI browser lewat CLI.
if [ -x /usr/bin/socksgui ]; then
  if command -v pkexec >/dev/null 2>&1; then
    exec pkexec /usr/bin/socksgui "$@"
  fi
  exec sudo /usr/bin/socksgui "$@"
fi
if command -v pkexec >/dev/null 2>&1; then
  exec pkexec /usr/bin/socksctl gui "$@"
fi
exec sudo /usr/bin/socksctl gui "$@"
EOS
  chmod 0755 "$pkg/usr/bin/socksclient-gui"

  cat > "$pkg/usr/share/applications/socksclient.desktop" <<'EOS'
[Desktop Entry]
Type=Application
Name=Socks Client
Comment=SOCKS5 tunnel (TUN) - semua trafik lewat server SOCKS5 kamu
Exec=socksclient-gui
Terminal=false
Categories=Network;
EOS

  cat > "$pkg/lib/systemd/system/socksclient.service" <<'EOS'
[Unit]
Description=Socks Client - tunnel SOCKS5 (TUN)
After=network-online.target
Wants=network-online.target
# Kredensial TIDAK ditulis di command line (bisa terlihat di `ps`), tapi
# diambil dari environment berikut.
EnvironmentFile=-/etc/socksclient.conf

[Service]
Type=simple
# -mtu sengaja tidak dipakai di sini: default CLI sudah 1400 (sama dengan klien
# lain). Kalau perlu diubah, tambahkan mis. -mtu 1350 di baris ExecStart.
ExecStart=/usr/bin/socksctl up -host ${SOCKS_HOST} -port ${SOCKS_PORT}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOS

  cat > "$pkg/usr/share/doc/socksclient/socksclient.conf.example" <<'EOS'
# Salin ke /etc/socksclient.conf lalu isi. Dipakai systemd unit socksclient.
SOCKS_HOST=10.0.0.1
SOCKS_PORT=1080
# MTU default 1400 (sama dengan klien Windows/Android); tidak perlu diisi kecuali
# jaringanmu butuh lebih kecil.
# opsional, kalau server memerlukan autentikasi
#SOCKS_USER=user
#SOCKS_PASS=password
EOS

  cat > "$pkg/DEBIAN/control" <<EOS
Package: socksclient
Version: $VERSION
Section: net
Priority: optional
Architecture: $arch
Maintainer: JhopanStore <jhopanstore.my.id>
Homepage: https://github.com/jhopan/SocksClient
Depends: libc6
Description: SOCKS5 tunnel client (TUN) - Socks Client
 Klien SOCKS5 untuk Linux. Seluruh trafik (TCP, UDP, DNS) dialirkan keluar
 melalui server SOCKS5, tanpa DNS leak dan tanpa IPv6 bocor.
 .
 Isi paket: /usr/bin/socksgui (GUI GTK3, di paket amd64), /usr/bin/socksctl
 (CLI + UI browser), core sing-box minimal di /usr/lib/socksclient/sing-box,
 unit systemd socksclient.service (tidak
 diaktifkan otomatis), dan /etc/socksclient.conf.example.
 .
 Pakai: sudo socksctl up -host 10.0.0.1 -port 1080
       sudo socksctl gui -host 10.0.0.1 -port 1080
EOS

  dpkg-deb --root-owner-group --build "$pkg" "$OUT/socksclient_${VERSION}_${arch}.deb" >/dev/null
  echo "   -> socksclient_${VERSION}_${arch}.deb"

  # tarball portable (tanpa root untuk dipasang, tanpa dpkg)
  tar_dir="$OUT/tar-$arch"
  rm -rf "$tar_dir"
  mkdir -p "$tar_dir/socksclient"
  install -m 0755 "$OUT/bin/socksctl-$arch" "$tar_dir/socksclient/socksctl"
  install -m 0755 "$OUT/core/sing-box-linux-$arch" "$tar_dir/socksclient/sing-box"
  if [ "$arch" = "amd64" ] && [ -f "$OUT/bin/socksgui-amd64" ]; then
    install -m 0755 "$OUT/bin/socksgui-amd64" "$tar_dir/socksclient/socksgui"
  fi
  cat > "$tar_dir/socksclient/install.sh" <<'EOS'
#!/bin/sh
# Pasang ke /usr/local (butuh root).
set -e
dir=$(dirname "$0")
install -m 0755 "$dir/socksctl" /usr/local/bin/socksctl
mkdir -p /usr/local/lib/socksclient
install -m 0755 "$dir/sing-box" /usr/local/lib/socksclient/sing-box
if [ -f "$dir/socksgui" ]; then
  install -m 0755 "$dir/socksgui" /usr/local/bin/socksgui
  echo "GUI GTK3 terpasang: sudo socksgui"
fi
echo "terpasang: /usr/local/bin/socksctl"
echo "pakai: sudo socksctl up -host <IP> -port <PORT>"
echo "       sudo socksctl gui -host <IP> -port <PORT>   (UI di browser)"
EOS
  chmod 0755 "$tar_dir/socksclient/install.sh"
  (cd "$tar_dir" && tar -czf "$OUT/SocksClient-linux-${arch}-${VERSION}.tar.gz" socksclient)
  echo "   -> SocksClient-linux-${arch}-${VERSION}.tar.gz"
done

echo "== checksum"
(cd "$OUT" && sha256sum socksclient_*.deb SocksClient-linux-*.tar.gz > SHA256SUMS-linux.txt && cat SHA256SUMS-linux.txt)
