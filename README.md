<div align="center">

# Socks Client

**v1.2.0**

SOCKS5 client for connecting a device to a SOCKS5 hotspot server.
Two connection modes: **TUN** (full tunnel) and **Proxy** (no TUN, no admin).

Available for **Android** and **Windows Desktop**.

[![Download APK](https://img.shields.io/badge/Download-APK%20v1.2.0-green?style=for-the-badge&logo=android&logoColor=white)](../../releases/latest)
[![Download Desktop](https://img.shields.io/badge/Download-Installer%20v1.2.0-blue?style=for-the-badge&logo=windows&logoColor=white)](../../releases/latest)

[![Release](https://img.shields.io/github/v/release/jhopan/SocksClient?style=for-the-badge&color=blue)](../../releases)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

</div>

---

## Modes

| Mode | How it works | Needs admin | Use when |
|------|--------------|-------------|----------|
| **TUN** | sing-box creates a virtual interface and routes every IP packet into it (wintun is embedded in the core, nothing to install) | Yes | Normal case — all apps including UDP/games go through the tunnel |
| **Proxy** | sing-box opens a local SOCKS5 + HTTP inbound and three proxy layers are pointed at it: WinINet (browsers, Electron, Edge), user environment variables (`HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` — Go, Python, Node, curl, git) and WinHTTP when elevated (Windows Update, installers) | No (WinHTTP layer only when elevated) | TUN does not start (AV blocks the wintun driver, no admin, route conflict) |

Apps that keep their own network stack (Steam, most games, torrent clients) ignore all
three layers and stay direct in proxy mode — point them at `127.0.0.1:2080` manually or
use TUN when UDP is needed.

The **Diagnosa** button reports: active mode, admin state, core path/version, whether the
local port is free, the current WinINet/env/WinHTTP values, the tail of `sing-box.log`
and an advice line derived from it. Turns "TUN does not work" into a reason.

Proxy mode is crash-safe: your previous WinINet proxy values are saved before being replaced and restored on disconnect — and on the next launch if the app was killed while connected.

---

## Core (minimal sing-box)

The repo ships **no binaries**. The sing-box core is built by GitHub Actions
(`.github/workflows/build-core.yml`) with **zero optional build tags** — only the
features a SOCKS5 client uses (socks, http, mixed, tun, direct, block, dns). No
quic, wireguard, utls, clash-api, tailscale, naive, usbip, openvpn, acme or dhcp.

Sizes (windows/amd64, `-s -w -trimpath`): **81.9 MB** upstream full build →
**36.0 MB** no optional tags → **27.6 MB** with the protocol registry trimmed
(`core/slim-registry.py`) → **8.6 MB** when the UPX variant is published.
The trim drops vless/vmess/trojan/shadowsocks/shadowtls/snell/ssh/tor/anytls/naive/
bridge/selector-urltest/clash-api/mdns-fakeip-hosts-resolved — nothing the client uses.

Everything is published to the [**core** release](../../releases/tag/core):

| Asset | Target |
|-------|--------|
| `sing-box-<ver>-windows-386.zip` | Windows 32-bit |
| `sing-box-<ver>-windows-amd64.zip` | Windows 64-bit |
| `sing-box-<ver>-darwin-amd64.tar.gz` | macOS Intel |
| `sing-box-<ver>-darwin-arm64.tar.gz` | macOS Apple Silicon |
| `sing-box-<ver>-linux-amd64.tar.gz` | Linux x86_64, static (Debian/Ubuntu/Alpine/OpenWrt x86) |
| `sing-box-<ver>-linux-arm64.tar.gz` | Linux arm64 |
| `sing-box-<ver>-linux-armv7.tar.gz` | Linux armv7 |
| `sing-box-<ver>-linux-mipsle-softfloat.tar.gz` | OpenWrt / mipsel routers |
| `libbox.aar` | Android: arm64-v8a, armeabi-v7a, x86, x86_64 |

All binaries are CGO-free and static — drop them on any machine, no runtime needed.
Rebuild with **Actions → Build Core → Run workflow**: inputs are the sing-box tag,
`slim` (registry trim, default on) and `upx` (also publish `-upx` variants, default off).
A `verify` job builds the trimmed core and runs `sing-box check` on the exact configs
the client runs (`desktop/cmd/dumpconfig`) before anything is published.

Build artifacts (installer, raw exe, APK set, core archives) are attached to every
workflow run under **Actions** -> run -> *Artifacts*; released builds also land in
[**Releases**](../../releases).

Fetch into a checkout:

```bash
desktop/scripts/fetch-core.sh     # -> desktop/embed/sing-box.exe
android/scripts/fetch-core.sh     # -> android/app/libs/libbox.aar
```

---

## Windows Desktop

Go + Walk (native Win32, no WebView2) + bundled sing-box v1.12.2.

### Features
- Mode selector: TUN / Proxy
- Proxy mode needs no Administrator rights and no TUN interface
- Automatic fallback offer when TUN fails right after start
- System proxy set/restore via WinINet, with a persisted backup
- System tray, minimize-to-tray, single instance (Windows mutex)
- Process tree cleanup on exit (no orphan sing-box)
- Inno Setup installer with silent auto-upgrade
- Runtime files live in `%LOCALAPPDATA%\SocksClientDesktop`

### Download
Grab the installer from [**Releases**](../../releases).

### Build from source

```bash
cd desktop
# prerequisites: Go 1.25+, windres (mingw-w64), Inno Setup 6 (installer only)
bash scripts/fetch-core.sh   # pulls the core from the "core" release
go vet ./...
go test -count=1 ./...
windres -o rsrc_windows_amd64.syso app.rc
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
```

The installer is built from `setup.iss` with `ISCC.exe` (Inno Setup 6), output in `desktop/installer_output/`.

> `desktop/embed/sing-box.exe` is downloaded from the `core` release (never committed).

---

## Android

APK built on sing-box (libbox), Java only, zero external dependencies.

### Features
- SOCKS5 VPN via sing-box core (TCP + UDP)
- Anti DNS leak, anti routing loop (`bind_interface` + bypass rule)
- Protocol sniffing (HTTP/TLS/QUIC), IPv4 only
- Traffic counter, splash screen, Info Developer dialog

### Build from source

```bash
cd android
bash scripts/fetch-core.sh  # pulls libbox.aar from the "core" release
./gradlew :app:assembleRelease
# output: android/app/build/outputs/apk/release/
```

Requirements: JDK 17+, Android SDK 35.

### Configuration notes
- DNS remote `tcp://8.8.8.8` through the SOCKS tunnel, strategy `ipv4_only`
- Route rules: server IP direct (anti loop), everything else to the SOCKS outbound
- sing-box v1.10.x on Android accepts no `action` field in route rules

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Agents working in this repo: read [AGENTS.md](AGENTS.md).

Release tags:

| Target | Tag | Workflow |
|--------|-----|----------|
| Android APK | `v1.2.0` | `.github/workflows/build-apk-release.yml` |
| Windows desktop | `desktop-v1.2.0` | `.github/workflows/build-desktop-release.yml` |

CI (`ci-desktop.yml`) runs `go vet` and `go test` on every push to `main` and on pull requests. All workflows live in the repo root `.github/workflows/`.

---

## Developer

**JhopanStore**

- Telegram: [@jhopan_05](https://t.me/jhopan_05)
- Website: [jhopanstore.my.id](https://jhopanstore.my.id)
- Support: [trakteer.id/jhopan](https://trakteer.id/jhopan)

---

## License

MIT — see [LICENSE](LICENSE).
