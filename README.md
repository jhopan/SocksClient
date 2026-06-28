# 🧦 Socks Client by JhopanStore

**v1.1.0** | APK Size: **7.4MB** | Installer Size: **13MB**

Client VPN SOCKS5 untuk menghubungkan perangkat ke server SOCKS5 hotspot.

Tersedia untuk **Android** dan **Windows Desktop**.

---

## 📱 Android

APK Socks Client untuk Android (SOCKS5 VPN Client).

### Fitur
- 🧦 SOCKS5 VPN via sing-box core (TCP + UDP)
- 🔒 DNS remote via tunnel (anti DNS leak)
- 🛡️ Anti routing loop (bind_interface + bypass rule)
- ⚡ Ultra lightweight (7.4MB APK, ~29MB install)
- 🎯 Protocol sniffing (HTTP/TLS/QUIC)
- 🌐 IPv4 support (IPv6 blocked via DNS strategy)
- 📊 Info Developer dialog (Telegram, Website, Trakteer)

### Changelog

#### v1.1.0 (2026-06-20)
**Size Optimization & Bug Fixes**
- 🔥 **Recompile sing-box v1.10.6** dengan custom tags (only `with_gvisor`)
- 📉 APK size reduced **57%**: 17MB → 7.4MB
- 📉 libbox.so reduced **58%**: 52MB → 21.9MB (arm64-v8a)
- 🐛 Fix disconnect bug (clear last_seen timestamp)
- 🐛 Fix connect failed (remove IPv6 action rule for v1.10.x compatibility)
- ✨ Add Info Developer dialog with 3 colored buttons
- 📝 Enable/disable logging for production build

#### v1.0.0 (2026-06-20)
- Initial release
- SOCKS5 VPN client with TCP + UDP support
- Traffic counter
- Splash screen

### Build
```bash
cd android
./gradlew assembleRelease
```

### Download
Cek [Releases](../../releases) untuk APK terbaru.

---

## 💻 Desktop (Windows)

Aplikasi desktop ringan menggunakan **Go + Walk (Win32 Native)** + **sing-box v1.12.2** sebagai core VPN.

### Fitur
- 🧦 SOCKS5 VPN via sing-box core (bundled)
- 🖥️ UI native Win32 (Walk) — ringan, tanpa WebView2
- 🗂️ System tray — close window masuk ke tray
- 🔒 Single instance — tidak bisa buka 2x
- 🪟 Installer (Inno Setup) — auto-upgrade, auto-close app
- ⚡ RAM ~12MB, total ~13MB
- ✅ Butuh Admin (TUN interface)

### Struktur
```
desktop/
├── main.go              ← Semua kode (UI + logic + tray)
├── go.mod / go.sum
├── app.ico              ← Logo socks (ICO untuk taskbar + tray)
├── app.manifest         ← Admin manifest
├── app.rc               ← Resource script
├── rsrc_windows_amd64.syso
├── setup.iss            ← Inno Setup script
├── embed/
│   ├── sing-box.exe     ← Core VPN v1.12.2 (Git LFS)
│   ├── app.ico          ← Logo socks (embedded)
│   └── logo_store.png   ← Banner JhopanStore
└── .gitignore
```

### Build dari source
```bash
cd desktop

# Prerequisites: Go 1.25+, GCC (CGO), Inno Setup 6

# Build app (64-bit GUI)
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .

# Build installer
# Buka setup.iss → Build → Compile (Ctrl+F9)
# Output: installer_output/SocksClientDesktop_Setup_v1.1.0.exe
```

### Build via GitHub Actions
Push tag `v*-desktop` — otomatis build + release installer.

### Install
Download installer dari [Releases](../../releases).

> ⚠️ **Butuh Admin** — App memerlukan hak admin untuk membuat TUN interface (sing-box).

### Changelog

#### v1.1.0 (2026-06-28)
**Rilis perdana Desktop Windows**
- 🧦 SOCKS5 VPN via sing-box v1.12.2 (bundled)
- 🖥️ UI native Win32 (Walk) — ringan
- 🗂️ System tray + minimize to tray
- 🔒 Single instance + Windows mutex
- 🪟 Installer Inno Setup — silent upgrade, auto UAC
- 🛡️ Cleanup — kill process tree on exit
- 📱 Info Developer dialog
- ⚡ RAM ~12MB, total ~13MB

---

## 📄 Lisensi

Open Source — JhopanStore
