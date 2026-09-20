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
| **TUN** | sing-box creates a virtual interface and routes all traffic into it | Yes (Windows) | Normal case — all apps go through the tunnel, TCP + UDP |
| **Proxy** | sing-box opens a local SOCKS5 + HTTP inbound; the Windows system proxy is pointed at it | No | TUN does not start on this machine (missing wintun, driver/AV block, route conflict) |

Proxy mode is crash-safe: your previous WinINet proxy values are saved before being replaced and restored on disconnect — and on the next launch if the app was killed while connected.

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
go vet ./...
go test -count=1 ./...
windres -o rsrc_windows_amd64.syso app.rc
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
```

The installer is built from `setup.iss` with `ISCC.exe` (Inno Setup 6), output in `desktop/installer_output/`.

> `desktop/embed/sing-box.exe` is tracked with Git LFS — run `git lfs pull` after cloning.

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
git lfs pull                # app/libs/libbox.aar
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
