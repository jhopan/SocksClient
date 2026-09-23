<div align="center">

# Socks Client

**v1.7.0** — SOCKS5 tunnel client for Android, Windows, Linux and macOS

Connect a device to your SOCKS5 hotspot server and route **all** traffic through
it: TCP, UDP and DNS. One mode, tuned for networks where other clients break.

[![Download APK](https://img.shields.io/badge/Android-APK%20v1.7.0-3ddc84?style=for-the-badge&logo=android&logoColor=white)](../../releases/latest)
[![Download Desktop](https://img.shields.io/badge/Windows-Installer%20v1.7.0-0078d4?style=for-the-badge&logo=windows&logoColor=white)](../../releases/latest)
[![Release](https://img.shields.io/github/v/release/jhopan/SocksClient?style=for-the-badge&color=blue)](../../releases)
[![Linux](https://img.shields.io/badge/Linux-.deb%20%2B%20tarball-fcc624?style=for-the-badge&logo=linux&logoColor=black)](../../releases/latest)
[![macOS](https://img.shields.io/badge/macOS-universal%20binary-000000?style=for-the-badge&logo=apple&logoColor=white)](../../releases/latest)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

</div>

---

## Why this client

Most SOCKS clients assume a clean network and a cooperating OS. This one assumes
the opposite: mobile hotspots with small MTUs, laptops whose NIC driver fights the
TUN adapter, and networks that quietly answer DNS for you.

| Problem it solves | How |
|---|---|
| "TUN connects but nothing loads" on some laptops | TUN stack pinned to **gvisor** (userspace L3-L4), so the tunnel does not depend on the machine's NIC driver or filter stack |
| Transfers stall while browsing works | MTU pinned to **1400** - hotspot paths are smaller than 1500, and a bigger MTU means PMTU blackholes |
| DNS leaking to the hotspot/ISP resolver | Every query is captured in the tunnel (`hijack-dns`, plus WFP-level DNS blocking on Windows) and answered through SOCKS |
| IPv6 sneaking out | Android routes `::/0` into the tunnel and rejects it; Windows has no IPv6 route and sing-tun blocks v6 at the firewall |
| Apps bypassing the tunnel | Windows runs `strict_route`; the local network stays reachable through explicit route exclusions |
| Tunnel stays "Connected" after switching networks | A watchdog notices the interface change and reconnects by itself |
| Not sure it is working | `Diagnosa` dialog + `desktop/scripts/tun-selftest.sh` prove it end to end, without needing the phone |

## Quick start

**Android**

1. Download the APK for your device (`arm64-v8a` for anything modern) from the
   [latest release](../../releases/latest).
2. Install, open, type the server **IP** and port, tap **Connect**, accept the VPN
   permission prompt.

**Windows**

1. Download `SocksClientDesktop_Setup_<version>.exe` from the
   [latest release](../../releases/latest).
2. Install. The setup copies the app plus the sing-box core; Windows shows a UAC
   prompt when the app starts (TUN needs Administrator).
3. Fill in the server **IP** and port, press **Connect Socks VPN**.

**Linux**

```bash
sudo dpkg -i socksclient_<version>_amd64.deb     # GUI is in the amd64 package
socksclient-gui                                  # or: sudo socksgui
# headless / service, no GUI at all:
sudo socksctl up -host 10.0.0.1 -port 1080
```
No dpkg? Use the tarball: `tar -xzf SocksClient-linux-amd64-<version>.tar.gz && sudo ./socksclient/install.sh`.

**macOS (Intel and Apple Silicon)**

```bash
tar -xzf SocksClient-macos-universal-<version>.tar.gz
cd SocksClient && ./install.sh            # copies into /usr/local
sudo socksctl up -host 10.0.0.1 -port 1080
```
Inside the archive `SocksClient.command` does the same with a double click: it asks
for the admin password and opens the browser UI. The binary is not notarized (no
Apple Developer ID), so if Gatekeeper objects run
`xattr -dr com.apple.quarantine SocksClient` once or right-click → Open.

The server address must be an **IP literal** on both platforms. A hostname would
have to be resolved *before* the tunnel exists - outside the tunnel on Android
(the app is excluded from its own VPN) and inside a feedback loop on Windows - so
the clients refuse it instead of leaking the name.

Verify what you downloaded:

```bash
sha256sum -c SHA256SUMS.txt
```

## HTTP ping (on/off)

Both clients can prove the tunnel end to end while connected: an `HTTP ping` radio
button sends `GET http://www.gstatic.com/generate_204` every 30 seconds and shows the
result next to the status (`ping 204 38ms`, or the error). If gstatic is blocked on
the network you are on, Cloudflare's `generate_204` is used as the fallback.

It is a diagnostic with a side benefit: the request keeps the SOCKS connection
warm, so a NAT or hotspot that drops idle connections does not silently kill the
path between pings. The tunnel itself never depends on it, and switching it off
removes all extra traffic.

It is built to be cheap: HTTP (never HTTPS, so no TLS), `204` with no body, minimal
headers, and a kept-alive connection so the next request is about two small
packets. One request every 30 s when healthy - roughly 1-2 MB per day if left on.
When a probe fails the next one comes after 10 s instead of 30 s, so a recovered
network shows up in the status within seconds.

How the probe travels matters. On Windows the app's own traffic enters the TUN, so
the request goes through the SOCKS server by itself. On Android the app is
deliberately excluded from its own VPN (the core must reach the server directly),
so a probe from the app would test the wrong path; instead the config opens a
loopback `mixed` inbound on `127.0.0.1:2081` and the probe is sent through it, which
means sing-box performs the request over `socks-out`. Both paths were measured on a
real machine: `204` in 0.22-0.39 s, with `strict_route` enabled.

## One GUI, three desktops

All three desktops are the same Go program: [`cmd/socksgui-gio`](desktop/cmd/socksgui-gio)
using [Gio](https://gioui.org). One codebase, one splash, one flow everywhere.

| Platform | Window | Built |
|---|---|---|
| Windows | `SocksClient.exe` (installed by `SocksClientDesktop_Setup_v*.exe`) | `-H windowsgui` + manifest `requireAdministrator` + icon |
| Linux | `socksgui` (inside the amd64 .deb, started from the menu via pkexec) | cgo + X11/EGL |
| macOS | `SocksClient.app` (universal) | cgo, `lipo` x86_64 + arm64 |

The window itself: a 2.5 s splash with the credit and the three usage steps, then
host / port / username / password (both optional), an HTTP ping switch, one
Connect button and one status line. Nothing else.

Connect runs a real preflight before the tunnel exists - host must be an IP,
then a TCP connection, then the SOCKS5 greeting and login - and each stage is
shown as it happens (`internal/engine.Preflight`). Only after that does the core
start with the shared config from `internal/boxcfg`, and the HTTP ping
(`internal/ping`, `GET http://www.gstatic.com/generate_204`, every 30 s) runs
through the tunnel.

The core process is spawned with `CREATE_NO_WINDOW`, so no console window ever
appears behind the app: `sing-box.exe` is a console binary and would otherwise
get its own terminal window when started from a GUI process.

Still shipped alongside: the CLI (`socksctl`, Linux/macOS) which runs the same
tunnel with no GUI at all, for servers and systemd.

## How it works## How it works

```
app -> TUN (gvisor, MTU 1400) -> sing-box -> SOCKS5 -> your server -> internet
                |
                +-- DNS queries are hijacked and resolved through the SOCKS tunnel
                +-- traffic to the SOCKS server itself bypasses the tunnel (anti-loop rule)
```

Both clients build their configuration from the same shape:

```jsonc
{
  "dns": {
    "servers": [
      { "tag": "remote",     "type": "tcp", "server": "1.1.1.1", "detour": "socks-out" },
      { "tag": "remote-udp", "type": "udp", "server": "1.1.1.1", "detour": "socks-out" },
      { "tag": "backup",     "type": "tcp", "server": "8.8.8.8", "detour": "socks-out" },
      { "tag": "local",      "type": "local" }
    ],
    "final": "remote",
    "strategy": "ipv4_only"
  },
  "inbounds": [{ "type": "tun", "mtu": 1400, "auto_route": true, "stack": "gvisor" }],
  "outbounds": [{ "type": "socks", "tag": "socks-out", "server": "<ip>", "server_port": 1080 }],
  "route": {
    "rules": [
      { "action": "sniff", "sniffer": ["dns"] },
      { "protocol": "dns", "action": "hijack-dns" },
      { "ip_cidr": ["<ip>/32"], "outbound": "direct" }
    ],
    "final": "socks-out"
  }
}
```

sing-box has no automatic failover between DNS servers: `1.1.1.1` answers, and
`8.8.8.8` is the backup you promote by pointing `final` at it - one line in
`desktop/internal/boxcfg/boxcfg.go` or `SocksVpnService.java`.

## Usage notes

| | Android | Windows | Linux / macOS |
|---|---|---|---|
| Mode | always TUN (`VpnService`) | always TUN (needs Administrator, UAC on launch) |
| DNS | VPN DNS + `hijack-dns`; no public resolver at the OS layer | same, plus WFP blocks any DNS that tries to leave |
| Local network | routed into the tunnel (phone hotspot case) | kept reachable via route exclusions (10/8, 172.16/12, 192.168/16, 169.254/16) |
| Credentials at rest | password encrypted with an Android Keystore AES-GCM key | password encrypted with DPAPI (tied to the Windows account) |
| Network change | default-network callback reloads sing-box | watchdog (`GetBestInterfaceEx`) restarts the core, max 4 times |
| HTTP ping | `On/Off` radio; the probe goes through a loopback inbound so it really tests the tunnel | `On/Off` radio; the probe is an HTTP GET that enters the TUN |
| Notifications | one ongoing notification (foreground service, type `systemExempted`) | tray icon + `Diagnosa` dialog | terminal output (`Ctrl+C` stops), browser UI with `socksctl gui` |
| Interface name | `sb-tun` | `sb-tun` | Linux `sb-tun`, macOS `utunN` (kernel-assigned, custom names are not allowed) |

Want plain HTTP/SOCKS proxying instead? Point that app straight at your SOCKS
server. This client deliberately does not set a system proxy.

## QA - how to check it actually works

**Any laptop, no phone needed**

```bash
# git-bash as Administrator, from the repo
desktop/scripts/fetch-core.sh          # core into desktop/embed/
desktop/scripts/tun-selftest.sh        # spins up a local SOCKS5, runs the real TUN config
```

What the script proves:

```
adapter sb-tun-selftest   -> up
https=200   http=200      -> the tunnel carries TCP
dns: exchanged ...        -> DNS was answered inside the tunnel, not by the LAN resolver
adapter after stop        -> cleaned up, internet back to normal
```

**On the phone**

```bash
adb logcat -s SocksVpnService          # no FATAL/exception while connecting
```

Then open `https://dnsleaktest.com` -> *Extended test*: every resolver must be the
one your SOCKS server uses, never the mobile ISP or the hotspot.

**Checklist used for every release**

| # | Check | Where |
|---|---|---|
| 1 | `go vet ./...` and `go test -count=1 ./...` green | `desktop/` |
| 2 | `sing-box check` accepts the generated configs | `desktop/cmd/dumpconfig` output |
| 3 | TUN carries TCP + DNS on a real machine | `tun-selftest.sh` |
| 4 | LAN still reachable while the tunnel is up | ping gateway + open the hotspot page |
| 5 | Teardown leaves no adapter/route behind | `tun-selftest.sh`, `netsh interface show interface` |
| 6 | APK builds, signature verified, connects, survives a dark screen | `adb`, `apksigner verify` |
| 7 | No DNS leak | `dnsleaktest.com` extended test |

## Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| TUN does not start / no `sb-tun` adapter | antivirus or EDR blocking the embedded wintun driver, or not elevated | run as Administrator, exclude the install folder and `%LOCALAPPDATA%\SocksClientDesktop`, press **Diagnosa** |
| Connected, adapter up, nothing loads | the SOCKS server itself is unreachable or not forwarding | check IP/port/auth, confirm the phone server runs, read the log tail in **Diagnosa** |
| Pages load, large downloads stall | MTU still too big for the path | default is 1400; try 1350 and re-test with `tun-selftest.sh` |
| First query slow, then normal | DNS warm-up through SOCKS | expected; later queries sit around 15 ms |
| DNS still looks like the ISP | the app was not connected, or something bypassed the VPN | re-run the leak test with the tunnel up; look for `dns: exchanged` in the log |
| Tunnel dead after switching Wi-Fi/hotspot | the socket was bound to the old interface | Windows: automatic (watchdog). Android: reloads on the default-network callback; if it persists, Disconnect -> Connect |
| Lost the signing key | the APK cannot be updated in place | keep the `keystore/` folder and its backup safe; without it users must uninstall first |

Logs and state:

```
Windows : %LOCALAPPDATA%\SocksClientDesktop\{settings.json,config.json,sing-box.log}
Android : adb logcat -s SocksVpnService , plus the log file written by libbox
```

## Core (minimal sing-box)

The tunnel core is sing-box, built by GitHub Actions and published as the
[`core` release](../../releases/tag/core) - never committed to git.

```
Targets  : windows 386/amd64, darwin amd64/arm64, linux amd64/arm64/armv7/mipsle, libbox.aar
Trimming : core/slim-registry.py drops every protocol a SOCKS client never uses
           (vless/vmess/trojan/shadowsocks/snell/ssh/tor/anytls/naive, clash-api,
           mdns/fakeip/hosts/resolved ...) -> ~27 MB instead of ~82 MB
Gate     : a verify job builds the trimmed core and runs sing-box check on the exact
           configs the clients emit before anything is published
UPX      : optional, adds a second set of smaller archives (-upx suffix)
```

Rebuild it: **Actions -> Build Core -> Run workflow** with a `version` input.

## Build from source

```bash
# CLI for Linux/macOS - from desktop/
GOOS=linux   GOARCH=amd64 go build -o socksctl ./cmd/socksctl
bash scripts/package-linux.sh      # .deb + tarballs into desktop/dist/
bash scripts/package-macos.sh      # universal tarball (needs macOS for lipo)

# Windows desktop - from desktop/
bash scripts/fetch-core.sh          # core into embed/
go vet ./... && go test -count=1 ./...
windres -o rsrc_windows_amd64.syso app.rc
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
ISCC.exe setup.iss                  # installer -> installer_output/   (Inno Setup 6)

# Android - from android/
bash scripts/fetch-core.sh          # libbox.aar into app/libs/
./gradlew :app:assembleRelease      # or assembleDebug
# on Windows git-bash use:
# java -classpath gradle/wrapper/gradle-wrapper.jar org.gradle.wrapper.GradleWrapperMain :app:assembleRelease
```

Release signing for the APK (optional locally, required in CI):

```bash
export KEYSTORE_FILE=/path/to/keystore/socksclient-release.jks
export KEYSTORE_PASSWORD=... KEY_ALIAS=socksclient KEY_PASSWORD=...
# build, then confirm the certificate is yours and not the debug key:
$ANDROID_HOME/build-tools/35.0.0/apksigner verify --print-certs app-arm64-v8a-release.apk
```

GitHub secrets used by the APK workflow: `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`,
`KEY_ALIAS`, `KEY_PASSWORD`. Without them the release build fails on purpose.

## Versioning and releases

| What changed | Version | Tags |
|---|---|---|
| New feature / behaviour change | `1.4.0` -> `1.5.0` | `v<ver>` (APK), `desktop-v<ver>` (Windows installer + Linux + macOS assets) |
| Rebuild only (core bump, fix without new features) | `1.3.0.1`, `1.3.0.2`, ... | fourth segment increments |
| Core only | unchanged | `core` |

`versionCode` on Android is `major*100000 + minor*10000 + patch*100 + build`, so
`1.4.0` is `140000` and `1.4.0.1` is `140001` - always increasing, as the platform
requires.

Pushing a tag runs the matching workflow; every release carries `SHA256SUMS.txt`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) for the
conventions a human or an agent needs: build commands, path quirks, and the
load-bearing config rules that must not be edited away.

## Developer

**JhopanStore** - [Telegram](https://t.me/jhopan_05) - [Website](https://jhopanstore.my.id) - [Trakteer](https://trakteer.id/jhopan)

## License

MIT - see [LICENSE](LICENSE).
