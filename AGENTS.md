# AGENTS.md

Project context for coding agents. Two independent clients (no shared code) plus
their release pipelines. Read this before editing anything.

## What this is

- `android/` — Android SOCKS5 VPN client, Java only + `libbox.aar` (sing-box core). No Kotlin, no external deps.
- `desktop/` — Windows client, Go + `github.com/lxn/walk` (native Win32) + bundled `sing-box.exe`.

## Dev environment

- Go 1.25+ (module `socks-client-desktop` in `desktop/go.mod`), mingw-w64 `windres` for resources.
- JDK 17+, Android SDK 35 for the APK.
- Inno Setup 6 (`ISCC.exe`) only for the Windows installer.
- **No binaries in git.** The sing-box core comes from the `core` release, built by `.github/workflows/build-core.yml`. Fetch it before building:
  `desktop/scripts/fetch-core.sh` -> `desktop/embed/sing-box.exe`, `android/scripts/fetch-core.sh` -> `android/app/libs/libbox.aar`.
  Tests use `SINGBOX_BIN` if set, else `desktop/embed/sing-box.exe` (skipped if absent).

## Build & test (exact commands)

Desktop — run from `desktop/`:

```bash
bash scripts/fetch-core.sh       # core into embed/ (skip only if SINGBOX_BIN is set)
go vet ./...
go test -count=1 ./...            # config validity (TUN/gvisor) + boxcfg unit tests
windres -o rsrc_windows_amd64.syso app.rc
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
ISCC.exe setup.iss                # installer -> installer_output/
```

Android — run from `android/`:

```bash
bash scripts/fetch-core.sh        # libbox.aar from the "core" release
./gradlew :app:assembleDebug      # or :app:assembleRelease (3 ABI splits + universal)
```

## Code layout

- `desktop/main.go` — UI, tray, connection lifecycle, core supervision, dialogs.
- `desktop/winutil_windows.go` — Administrator check + UAC re-launch (the only Windows plumbing left).
- `desktop/main_test.go` — runnable checks (see above); needs the core binary.
- `desktop/scripts/fetch-core.sh`, `android/scripts/fetch-core.sh` — pull the core from the `core` release.
- `.github/workflows/build-core.yml` — the only place the core is built (no optional tags; Android keeps `with_gvisor`). Inputs: `version`, `slim` (default on), `upx`.
- `core/slim-registry.py` — trims sing-box `include/registry.go` to socks/http/mixed/tun/direct/block/local-DNS. Runs in every core job; it exits non-zero if upstream renames a required registration, which skips the release instead of shipping a broken core. The `verify` job then builds the trimmed core and runs `sing-box check` on the configs emitted by `desktop/cmd/dumpconfig`.
- `desktop/internal/boxcfg` — single source of truth for the sing-box config (pure stdlib, no windows imports) so both the app and the core workflow use the same JSON. `desktop/cmd/dumpconfig` writes it out for CI.
- `android/app/src/main/java/com/jhopanstore/socksclient/` — `MainActivity` (UI), `SocksVpnService` (VpnService + libbox platform interfaces + config builder), `SplashActivity`, `DebugLog`.

## Connection modes (desktop)

- `tun` — virtual interface, all traffic, needs Administrator. If not elevated, the app offers UAC re-launch.
- Runtime state lives in `%LOCALAPPDATA%\SocksClientDesktop` (`settings.json`, `config.json`, `sing-box.log`; the core itself stays next to the exe in the install directory). Never write to the install dir.
- TUN needs no external driver: `sing-tun` embeds `wintun.dll` (amd64/arm/arm64/386) into the core binary. When TUN still fails, it is admin rights or AV/EDR blocking the extracted driver — the app's Diagnosa dialog spells that out.
- TUN defaults: stack `gvisor` on **both** clients (desktop `boxcfg`, Android `SocksVpnService`), MTU `1400` (`DefaultTunMTU`), DNS `ipv4_only`. The desktop app pins both - no UI knob, no setting - and falls back to gvisor for an unknown stack. `boxcfg` keeps the knobs so `dumpconfig` and the core CI can render `mixed`/`system`/other MTUs for testing. Desktop has a single mode (TUN); the manifest requests `requireAdministrator`, so there is no unelevated path.
- TUN config rules that are load-bearing and tested in `desktop/internal/boxcfg/boxcfg_test.go`: an explicit `{"type":"direct","tag":"direct"}` outbound (route rule + bootstrap DNS reference it; without it sing-box dies with `outbound detour not found: direct`), `{"protocol":"dns","action":"hijack-dns"}` so DNS cannot leak to a DHCP resolver, `{"server":"local"}` for bootstrap only (a `direct`-detoured DNS server is rejected inside `auto_route`), and the anti-loop rule for the server IP/hostname. `sing-box check` does NOT catch missing tag references — that is why these unit tests exist.
- The core CLI builds use `-tags with_gvisor` (+4 MB) so the app's `gvisor` TUN stack actually exists at runtime; without the tag selecting gvisor fails at start.
- `desktop/scripts/tun-selftest.sh` (Windows, admin) validates TUN end to end on a real machine: local SOCKS5 upstream, real TUN config, TCP + DNS assertions, teardown check. Run it after touching `boxcfg`.
- Desktop leak/stability guards, all asserted in `desktop/internal/boxcfg/boxcfg_test.go`: `strict_route: true` (WFP blocks non-tunnel traffic, including DNS, on Windows), `route_exclude_address` for the three RFC1918 ranges plus link-local so the LAN/hotspot stays reachable, DNS primary `1.1.1.1` with `8.8.8.8` kept as the backup entry (sing-box has no automatic failover - promote it by changing `final`), IPv6 none/blocked.
- Host must be an IP literal on **both** clients: a hostname forces a bootstrap lookup outside the tunnel (the app process is excluded from the VPN on Android, and on Windows it loops back into the tunnel).
- `desktop/watchdog_windows.go` + `watchNetwork()` in `main.go`: every 8s `GetBestInterfaceEx` says which interface would reach the server; when it changes, the core is restarted (max 4 times, then the UI asks for a manual reconnect). No probe traffic, so it works under `strict_route`.
- Android: `{"action":"sniff","sniffer":["dns"]}` makes hijack-dns cover resolvers other than the VPN DNS address; the sniff action in 1.14 carries no destination override, so it cannot rewrite where a connection goes.
- HTTP ping (On/Off radio, both clients): one `GET /generate_204` every 30 s, shown next to the status. Desktop probes directly (its traffic enters the TUN). Android probes through the loopback `mixed` inbound `127.0.0.1:2081` (`ping-in`) because the app is excluded from its own VPN - a direct probe there would test the wrong path and report OK with a dead tunnel. Never remove that inbound while the ping loop exists.
- Android network change: `SocksVpnService.registerNetworkWatchdog()` reacts to `registerDefaultNetworkCallback`; a change reloads sing-box through `startOrReloadService` (15s debounce, full reconnect as the fallback). Do not remove it - a hotspot handover otherwise leaves a "Connected" tunnel that carries nothing.
- Stored credentials are encrypted on both clients: Windows uses DPAPI (`desktop/dpapi_windows.go`, `pass_enc` in settings.json, covered by `dpapi_test.go`), Android uses an Android Keystore AES-GCM key via `SecurePrefs` (`pass_enc` pref, legacy `pass` is read once and then removed).
- Android CI (`ci-android.yml`) runs `assembleDebug` + `lint` on every Android push; the APK workflow additionally gates on the signature.
- Android battery: heartbeat and traffic poll are 10s each, the notification is only re-posted when the counters change, and `MainActivity` accepts a 45s-old heartbeat as alive (3 missed beats) - do not tighten these back to 2-3s.
- Any change to TUN config must keep `go test ./...` green; `desktop/scripts/tun-selftest.sh` is the end-to-end proof on a real machine.

## Release notes and retention (MANDATORY)

- Release notes are never bare. Both release workflows carry the full template in
  their `Create GitHub Release` step: what the build is, a download table, the
  feature list for this build, install steps, how to verify the checksum/signature,
  requirements, and links to the sibling releases. Update that body whenever a
  feature lands - a release with a one-line body is a bug.
- Only **three** releases exist at any time: `core`, the newest APK (`v*`) and the
  newest Windows installer (`desktop-v*`). Both app workflows delete superseded
  releases right before publishing (`Delete superseded releases`, tags are kept).
  Do not create a fourth release; if one appears, the retention step is broken.
- The `core` release is never deleted - the app builds fetch from it.

## Three desktop GUIs (deliberately platform-native)

Each desktop uses its own OS toolkit - do not replace any of them with a
cross-platform Go GUI toolkit:

| Platform | Package | Toolkit | Measured binary | Measured RAM (idle) |
|---|---|---|---|---|
| Windows | `desktop/` (package main) | `lxn/walk` (Win32) | ~10 MB | ~27 MB |
| Linux | `desktop/cmd/socksgui-gtk` | GTK3 via cgo | ~5.9 MB | ~77 MB (mostly libgtk shared with the session) |
| macOS | `desktop/cmd/socksgui-mac` | AppKit via cgo/ObjC | ~5.5 MB | not measured |

Measured comparison that justifies this (same hello-world window): GTK3 1.07 MB /
77 MB RAM, AppKit 1.06 MB, Gio 6.4 MB / 129 MB, Fyne 22.8 MB / 168 MB, Wails-Tauri
needs WebKitGTK (92.6 MB installed on Debian). Native bindings win on both size
and memory because the toolkit is already loaded by the desktop.

Rules:

- Both GUI packages are behind `//go:build linux && cgo` and
  `//go:build darwin && cgo`: they only compile on their own platform, so
  `ci-desktop.yml` has `gui-linux` and `gui-macos` jobs and
  `build-desktop-release.yml` has the matching `linux`/`macos` jobs. Do not
  remove them; a broken GUI is invisible otherwise.
- All form/tunnel behaviour lives in `internal/guicore` (UI interface + state) and
  `internal/engine` (core supervision). The platform packages only own widget
  code and the thin `guicore.UI` adapter. Keep it that way - it is what makes the
  GUI logic testable without a display.
- `//export` functions must live in a file whose cgo preamble contains declarations
  only; the C/ObjC definitions belong in the other file (`gtk_c.go`, `cocoa_c.go`).
- GTK3 needs `libgtk-3-dev` to build and `libgtk-3-0` to run (declared in the .deb).
  cgo cannot cross-compile the GUI, so the arm64 .deb ships the CLI only - that is
  intentional and noted in the release.
- macOS password goes to the Keychain (`security` CLI, see
  `internal/settings/keychain_darwin.go`); on Linux it stays in
  `~/.config/socksclient/settings.json` mode 0600.
- Never call GTK/AppKit from a Go goroutine: all widget work happens in the
  callbacks/timer on the main thread, and the `guicore` calls reached from there
  are the only ones allowed to touch widgets.

## CLI for Linux and macOS (`socksctl`)

- `desktop/cmd/socksctl/` is the same client for platforms where `lxn/walk` does
  not exist: TUN through the shared config, no native GUI. Subcommands: `up`
  (foreground, root), `gui` (browser UI on 127.0.0.1, root), `config`, `check`,
  `version`. Stdlib only - do not add a GUI toolkit here.
- Config comes from `internal/boxcfg` and the HTTP ping from `internal/ping`, both
  shared with the Windows app. Any rule change belongs in `boxcfg`, never inlined
  in the CLI.
- Interface name: Linux/Windows `sb-tun`, macOS nothing (`TunOptions.AutoInterfaceName`)
  because Darwin only allows `utunN`. `boxcfg.DefaultInterfaceName(goos)` encodes this.
- Root is required for TUN on both platforms; `socksctl up/gui` check and print the
  `sudo` line instead of failing obscurely.
- Credentials must not appear on the command line (visible in `ps`): the CLI reads
  `SOCKS_USER`/`SOCKS_PASS` from the environment, and the packaged systemd unit
  uses `EnvironmentFile=/etc/socksclient.conf`.
- Packaging: `desktop/scripts/package-linux.sh` (.deb amd64/arm64 + tarballs) and
  `desktop/scripts/package-macos.sh` (one universal tarball via `lipo`). Both run in
  the `linux`/`macos` jobs of `build-desktop-release.yml`, which attach their assets
  to the same `desktop-v*` release - there is no fourth release. `ci-desktop.yml`
  cross-compiles the CLI for linux/darwin on every push so a broken build tag is
  caught before a tag push.
- macOS binaries are unsigned/notarized-less: the README tells users about
  `xattr -dr com.apple.quarantine`.

## Version policy

- A change that adds a feature or alters behaviour bumps the **normal** version: `1.3.0` -> `1.4.0`.
- A rebuild that adds no feature (core bump, small fix) increments the **fourth** segment: `1.3.0.1`, `1.3.0.2`, ... and the tag follows (`v1.3.0.1` / `desktop-v1.3.0.1`).
- Android `versionCode` = `major*100000 + minor*10000 + patch*100 + build` (this is what the app actually ships: `1.4.0` -> `140000`, `1.4.0.1` -> `140001`, `1.5.0` -> `150000` - always increasing).
- Version numbers live in three places: `desktop/main.go` (`appVersion`), `desktop/setup.iss` (`MyAppVersion`), `android/app/build.gradle.kts` (`versionName` + `versionCode`).

## APK signing (MANDATORY for releases)

- The release APK must be signed with the project key, never the Android debug key. `android/app/build.gradle.kts` picks the release keystore from `KEYSTORE_FILE`; the APK workflow decodes `KEYSTORE_BASE64` into that file and **fails the release** if `apksigner verify` reports "Android Debug".
- Secrets: `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD`. The keystore and its backup live outside the repo (`documents/project/keystore/`); never commit them.
- Losing the key means installed users must uninstall before they can take an update.

## Release & tag convention (MANDATORY)

| Target | Tag | Workflow |
|--------|-----|----------|
| Core (sing-box) | `core` (or any `core*` tag) | `.github/workflows/build-core.yml` |
| Android | `v1.2.0` (bumps `versionCode`/`versionName` in `android/app/build.gradle.kts`) | `.github/workflows/build-apk-release.yml` |
| Desktop | `desktop-v1.2.0` (bumps `appVersion` in `desktop/main.go` + `MyAppVersion` in `desktop/setup.iss`) | `.github/workflows/build-desktop-release.yml` |

The core workflow also updates the existing `core` release in place (`--latest=false`
so the newest app release stays "Latest"). Bump the sing-box version by running it
with a new `version` input.

Never tag a desktop release as `v*` — `v*` belongs to Android and would trigger the APK workflow.
Version numbers live in exactly three places: `desktop/main.go`, `desktop/setup.iss`, `android/app/build.gradle.kts`.

## Git push policy (MANDATORY)

Every `git commit` is followed by `git push origin main`. Local-only commits are not backed up and trigger no CI. Commit messages: conventional prefix (`feat:`, `fix:`, `docs:`, `chore:`), Indonesian or English body, no `Co-Authored-By` trailers.

## Conventions

- Desktop code: single-purpose files, no new dependencies unless stdlib cannot do it (`golang.org/x/sys/windows/registry` is the one deliberate exception).
- No stubs, no placeholder configs — every artifact must build and run as delivered.
- Android: Java 17 source level, no new Gradle dependencies (only `libbox.aar`).
- Release notes: English, short, no donation links.
- This repo uses **AGENTS.md only — no CLAUDE.md**.

## Scope boundary

Everything inside this repo is in scope. Sibling projects on the same machine (`PayPan/`, `AgenPulsa/`, `9drive/`, VPN server repos, any systemd unit outside this repo) are **report-only**: describe the bug with file/line and stop. Fixes there belong to that project's own session.

## Pitfalls

- Workflows must sit in the **repo root** `.github/workflows/`. A workflow under `android/.github/` is never read by GitHub and silently does nothing.
- **Both clients now run sing-box 1.14** (`libbox.aar` and the desktop core come from the same `core` release). Config rules that apply to both: no `sniff`/`sniff_override_destination` in the tun inbound (removed in 1.13 - sniffing is a route action), no legacy DNS `address` field (removed in 1.14 - use `{"type":"tcp","server":"8.8.8.8"}`), route rules may use `"action"`. `android/config.example.json` and `android/config.hostname.json` are the fixtures the Android builder emits; the core workflow runs `sing-box check` on them, so keep them in sync with `SocksVpnService.buildSingBoxConfig`.
- Never commit `desktop/embed/sing-box.exe` or `android/app/libs/libbox.aar`; both are gitignored and rebuilt by CI.
- Android anti-DNS-leak design: DNS servers detour through `socks-out`, `openTun` adds NO public fallback resolvers (8.8.8.8/1.1.1.1 at the OS layer are a leak path), and `{"protocol":"dns","action":"hijack-dns"}` catches every query including one aimed at a hardcoded resolver. Do not re-add fallback DNS. Both clients share this shape on purpose.
- TUN is the only mode, so `desktop/app.manifest` requests `requireAdministrator`: launching the app always shows the UAC prompt, and the in-app admin check is only a safety net. Consequence: any autostart entry prompts at logon - do not add one silently.
- `walk` handles must be touched on the UI thread: cross-goroutine updates go through `a.mw.Synchronize`.
- Killing sing-box must use `taskkill /F /T` on the PID or the TUN interface stays behind.
- On Windows git-bash, `./gradlew` dies with `Could not find or load main class org.gradle.wrapper.GradleWrapperMain` (MSYS path handed to native `java`). Use `cmd //c gradlew.bat ...` or invoke the wrapper directly:
  `java -classpath C:/<repo>/android/gradle/wrapper/gradle-wrapper.jar org.gradle.wrapper.GradleWrapperMain :app:assembleRelease`.
