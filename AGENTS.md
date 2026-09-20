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
- **Git LFS is required**: `desktop/embed/sing-box.exe` and `android/app/libs/libbox.aar` are LFS objects. Run `git lfs pull` after cloning.

## Build & test (exact commands)

Desktop — run from `desktop/`:

```bash
go vet ./...
go test -count=1 ./...            # 3 tests: config validity, system proxy round-trip, proxy-mode traffic
windres -o rsrc_windows_amd64.syso app.rc
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
ISCC.exe setup.iss                # installer -> installer_output/
```

Android — run from `android/`:

```bash
./gradlew :app:assembleDebug      # or :app:assembleRelease (3 ABI splits + universal)
```

## Code layout

- `desktop/main.go` — UI, tray, connection lifecycle, sing-box config builders (`buildTunConfig`, `buildProxyConfig`).
- `desktop/sysproxy_windows.go` — WinINet registry read/apply/restore, admin check, UAC re-launch.
- `desktop/main_test.go` — runnable checks (see above).
- `android/app/src/main/java/com/jhopanstore/socksclient/` — `MainActivity` (UI), `SocksVpnService` (VpnService + libbox platform interfaces + config builder), `SplashActivity`, `DebugLog`.

## Connection modes (desktop)

- `tun` — virtual interface, all traffic, needs Administrator. If not elevated, the app offers UAC re-launch.
- `proxy` — local `mixed` (SOCKS5+HTTP) inbound on `127.0.0.1:<local_port>`, Windows system proxy pointed at it, no admin.
- Runtime state lives in `%LOCALAPPDATA%\SocksClientDesktop` (`settings.json`, `config.json`, `sing-box.log`, extracted `sing-box.exe`). Never write to the install dir.
- `settings.json` field `proxy_backup` holds the user's previous WinINet values; restore it on disconnect **and** on startup (crash recovery). Do not clear it without restoring.
- Any change to proxy/tun config must keep `go test ./...` green — `TestProxyModeCarriesTraffic` is the end-to-end proof of the non-TUN path.

## Release & tag convention (MANDATORY)

| Target | Tag | Workflow |
|--------|-----|----------|
| Android | `v1.2.0` (bumps `versionCode`/`versionName` in `android/app/build.gradle.kts`) | `.github/workflows/build-apk-release.yml` |
| Desktop | `desktop-v1.2.0` (bumps `appVersion` in `desktop/main.go` + `MyAppVersion` in `desktop/setup.iss`) | `.github/workflows/build-desktop-release.yml` |

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
- sing-box v1.12 (desktop) wants `"address"` in the tun inbound; older versions used `inet4_address`. The Android core is v1.10.x and rejects route-rule `"action"` fields — do not copy desktop config into the Android builder.
- TUN needs Administrator; proxy mode must stay usable unelevated — do not re-add `requireAdministrator` to `desktop/app.manifest`.
- `walk` handles must be touched on the UI thread: cross-goroutine updates go through `a.mw.Synchronize`.
- Killing sing-box must use `taskkill /F /T` on the PID or the TUN interface stays behind.
