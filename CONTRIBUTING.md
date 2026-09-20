# Contributing

Thanks for helping. This repo ships two clients and releases the built artifacts
from GitHub Actions, so a PR only needs source + tests — no binaries.

## Before you start

1. Fetch the core: `desktop/scripts/fetch-core.sh` (and `android/scripts/fetch-core.sh` for the APK). Binaries are never committed — CI builds them in **Build Core** and publishes the `core` release.
2. Read `AGENTS.md` for layout, exact commands and pitfalls.

## Building

Desktop (from `desktop/`):

```bash
bash scripts/fetch-core.sh
go vet ./...
go test -count=1 ./...
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .
```

Android (from `android/`):

```bash
./gradlew :app:assembleDebug
```

## Pull requests

- One topic per PR. Small diffs get reviewed fast.
- Any change to the desktop config builders or the proxy/system-proxy code must keep `go test -count=1 ./...` green.
- Non-trivial logic leaves one runnable check behind (assert-based or a small `go test` case). No new test frameworks.
- Do not commit build output: `socks-client.exe`, `installer_output/`, `app/build/`, `*.apk` are ignored — keep it that way.
- Commit style: conventional prefix (`feat:`, `fix:`, `docs:`, `chore:`), no `Co-Authored-By` trailers.
- Never change the release tag convention: Android = `v*`, desktop = `desktop-v*`.

## Reporting bugs

Include: platform (Windows build / Android version), which connection mode,
the SOCKS server address type (local hotspot or public), and for desktop the
tail of `%LOCALAPPDATA%\SocksClientDesktop\sing-box.log`.

## Ideas that fit

- Proxy mode for Android (no `VpnService`, same local-inbound idea as desktop)
- Per-app split routing, macOS/Linux desktop builds
- Better diagnostics UI for the log

## Not wanted

- New runtime dependencies in `desktop/` (stdlib + `golang.org/x/sys` is the budget)
- Committing binaries (`sing-box.exe`, `libbox.aar`, APKs, installers)
- Gradle/Android dependencies beyond `libbox.aar`
- Reformatting-only PRs
