<div align="center">

# Socks Client

**v1.2.0**

SOCKS5 client for connecting a device to a SOCKS5 hotspot server.
One connection mode: **TUN** (full tunnel, needs Administrator).

Available for **Android** and **Windows Desktop**.

[![Download APK](https://img.shields.io/badge/Download-APK%20v1.2.0-green?style=for-the-badge&logo=android&logoColor=white)](../../releases/latest)
[![Download Desktop](https://img.shields.io/badge/Download-Installer%20v1.2.0-blue?style=for-the-badge&logo=windows&logoColor=white)](../../releases/latest)

[![Release](https://img.shields.io/github/v/release/jhopan/SocksClient?style=for-the-badge&color=blue)](../../releases)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

</div>

---

## Mode

Desktop has exactly one mode: **TUN**. sing-box creates a virtual interface and
routes every IP packet into it — TCP, UDP and DNS alike (wintun is embedded in
the core, nothing to install).

The app declares `requireAdministrator`, so Windows shows the UAC prompt the
moment you launch it — TUN can never fail later for lack of rights.

Want a plain HTTP/SOCKS proxy instead? Point the client straight at your SOCKS
server — the app does not set a system proxy, does not listen on a local port,
and does not touch WinINet, environment variables or WinHTTP.

### TUN defaults (and when to change them)

| Setting | Value | Why | Change when |
|---|---|---|---|
| TUN stack | `gvisor` (pinned) | translates L3->L4 entirely in userspace: not affected by flaky NIC drivers / WFP filters, which is the usual "TUN starts but nothing passes" cause. `mixed`/`system` stay available in `boxcfg` for tooling, but the app no longer exposes them | - |
| MTU | `1400` (pinned) | hotspot paths often carry a smaller MTU; oversized packets stall or fragment (PMTU blackhole). 1400 leaves headroom for SOCKS overhead | change `DefaultTunMTU` in `boxcfg` and rebuild |
| DNS | `1.1.1.1` via SOCKS, `8.8.8.8` as the backup entry, strategy `ipv4_only` | DNS can only travel the tunnel; v6 queries have nowhere sane to go | promote `8.8.8.8` by editing `final` if the primary is blocked on your network |

Apps that keep their own network stack (Steam, most games, torrent clients) ignore all
own network stack still go direct by design.
use TUN when UDP is needed.

### TUN correctness

Three things that used to break TUN on real laptops and are now covered by tests:

1. **Explicit `direct` outbound.** The route rule that keeps the tunnel's own packets out of
   the tunnel (anti-loop) and the bootstrap DNS server both reference the tag `direct`.
   sing-box 1.14 does not create it implicitly: without it the service dies at startup
   with `outbound detour not found: direct`, which looks exactly like "TUN connected but
   dead".
2. **DNS stays in the tunnel.** Every DNS query is hijacked (`hijack-dns` route action) and
   answered by a resolver reached over the SOCKS connection, so a laptop whose DHCP hands
   out a LAN resolver cannot leak queries outside the tunnel.
3. **Bootstrap only.** The system resolver is used *only* to resolve the SOCKS server's own
   hostname (`default_domain_resolver`). A `direct`-detoured DNS server is rejected by
   sing-box inside `auto_route` (it would loop back into the tunnel).

Desktop validation on a real machine:

```bash
# git-bash as Administrator
desktop/scripts/tun-selftest.sh            # or: tun-selftest.sh "Ethernet 2"
```

It starts a local SOCKS5 server, runs the real TUN config against it, proves TCP + DNS
travel the tunnel and that the adapter/routes/internet are restored afterwards.

The **Diagnosa** button reports: active mode, admin state, core path/version, whether the
core version and path, admin state, and the tail of `sing-box.log`
and an advice line derived from it. Turns "TUN does not work" into a reason.

TUN state is crash-safe: leftover routes are released on disconnect and the next launch is clean — if the app was killed while connected.

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
- No mode selector: TUN is the only path
- Automatic fallback offer when TUN fails right after start
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
- Core is sing-box 1.14 (same `core` release as the desktop client), so the config uses the current field set: DNS servers as `{"type":"tcp","server":"8.8.8.8","detour":"socks-out"}`, no `sniff` in the tun inbound, and `"action"` allowed in route rules
- DNS strategy `ipv4_only`; the resolver handed to apps points only at the VPN DNS, never a public fallback
- `{"protocol":"dns","action":"hijack-dns"}` catches every DNS query — including an app that hardcodes a resolver — and answers it through `socks-out`
- Route rules: server IP/hostname direct (anti loop), everything else to the SOCKS outbound
- TUN stack `gvisor`, MTU 1400 — identical defaults to the desktop client, chosen for machines/networks where the OS stack misbehaves
- `android/config.example.json` is the config the builder emits; CI runs `sing-box check` on it

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
