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
go test -count=1 ./...            # 3 tests: config validity, system proxy round-trip, proxy-mode traffic
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

- `desktop/main.go` — UI, tray, connection lifecycle, sing-box config builders (`buildTunConfig`, `buildProxyConfig`).
- `desktop/sysproxy_windows.go` — WinINet registry read/apply/restore, admin check, UAC re-launch.
- `desktop/main_test.go` — runnable checks (see above); needs the core binary.
- `desktop/scripts/fetch-core.sh`, `android/scripts/fetch-core.sh` — pull the core from the `core` release.
- `.github/workflows/build-core.yml` — the only place the core is built (no optional tags; Android keeps `with_gvisor`). Inputs: `version`, `slim` (default on), `upx`.
- `core/slim-registry.py` — trims sing-box `include/registry.go` to socks/http/mixed/tun/direct/block/local-DNS. Runs in every core job; it exits non-zero if upstream renames a required registration, which skips the release instead of shipping a broken core. The `verify` job then builds the trimmed core and runs `sing-box check` on the configs emitted by `desktop/cmd/dumpconfig`.
- `desktop/internal/boxcfg` — single source of truth for the sing-box config (pure stdlib, no windows imports) so both the app and the core workflow use the same JSON. `desktop/cmd/dumpconfig` writes it out for CI.
- `android/app/src/main/java/com/jhopanstore/socksclient/` — `MainActivity` (UI), `SocksVpnService` (VpnService + libbox platform interfaces + config builder), `SplashActivity`, `DebugLog`.

## Connection modes (desktop)

- `tun` — virtual interface, all traffic, needs Administrator. If not elevated, the app offers UAC re-launch.
- `proxy` — local `mixed` (SOCKS5+HTTP) inbound on `127.0.0.1:<local_port>`, Windows system proxy pointed at it, no admin.
- Runtime state lives in `%LOCALAPPDATA%\SocksClientDesktop` (`settings.json`, `config.json`, `sing-box.log`, extracted `sing-box.exe`). Never write to the install dir.
- `settings.json` fields `proxy_backup` / `env_backup` / `winhttp_backup` hold the previous WinINet, environment-variable and WinHTTP values; restore them on disconnect **and** on startup (crash recovery). Do not clear them without restoring.
- Proxy mode writes three layers (`sysproxy_windows.go`): WinINet registry, `HKCU\Environment` proxy vars, and `netsh winhttp set proxy` (elevated only). Every layer is covered by a round-trip test in `main_test.go` — keep them green.
- TUN needs no external driver: `sing-tun` embeds `wintun.dll` (amd64/arm/arm64/386) into the core binary. When TUN still fails, it is admin rights or AV/EDR blocking the extracted driver — the app's Diagnosa dialog spells that out.
- TUN defaults in `boxcfg`: stack `gvisor`, MTU `1400` (`DefaultTunMTU`), DNS `ipv4_only`. Rationale: hotspot MTUs are often smaller (PMTU blackhole with big MTU) and gvisor does not depend on the machine's NIC driver stack. Keep `dumpconfig` variants in sync when these change.
- TUN config rules that are load-bearing and tested in `desktop/internal/boxcfg/boxcfg_test.go`: an explicit `{"type":"direct","tag":"direct"}` outbound (route rule + bootstrap DNS reference it; without it sing-box dies with `outbound detour not found: direct`), `{"protocol":"dns","action":"hijack-dns"}` so DNS cannot leak to a DHCP resolver, `{"server":"local"}` for bootstrap only (a `direct`-detoured DNS server is rejected inside `auto_route`), and the anti-loop rule for the server IP/hostname. `sing-box check` does NOT catch missing tag references — that is why these unit tests exist.
- The core CLI builds use `-tags with_gvisor` (+4 MB) so the app's `gvisor` TUN stack actually exists at runtime; without the tag selecting gvisor fails at start.
- `desktop/scripts/tun-selftest.sh` (Windows, admin) validates TUN end to end on a real machine: local SOCKS5 upstream, real TUN config, TCP + DNS assertions, teardown check. Run it after touching `boxcfg`.
- Any change to proxy/tun config must keep `go test ./...` green — `TestProxyModeCarriesTraffic` is the end-to-end proof of the non-TUN path.

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
- The desktop config targets sing-box >= 1.13: `sniff` moved out of the tun inbound into a route action (`{"action":"sniff"}`). The old `inet4_address` field is gone too. Do not copy the desktop config into the Android builder — libbox there is 1.10.x and rejects route-rule `"action"`.
- Never commit `desktop/embed/sing-box.exe` or `android/app/libs/libbox.aar`; both are gitignored and rebuilt by CI.
- TUN needs Administrator; proxy mode must stay usable unelevated — do not re-add `requireAdministrator` to `desktop/app.manifest`.
- `walk` handles must be touched on the UI thread: cross-goroutine updates go through `a.mw.Synchronize`.
- Killing sing-box must use `taskkill /F /T` on the PID or the TUN interface stays behind.
- On Windows git-bash, `./gradlew` dies with `Could not find or load main class org.gradle.wrapper.GradleWrapperMain` (MSYS path handed to native `java`). Use `cmd //c gradlew.bat ...` or invoke the wrapper directly:
  `java -classpath C:/<repo>/android/gradle/wrapper/gradle-wrapper.jar org.gradle.wrapper.GradleWrapperMain :app:assembleRelease`.
