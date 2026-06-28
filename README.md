# 🧦 Socks Client by JhopanStore

**v1.1.0** | APK: **7.4MB** | Installer: **13MB**

Client VPN SOCKS5 untuk menghubungkan perangkat ke server SOCKS5 hotspot.

Tersedia untuk **Android** dan **Windows Desktop**.

[![Download APK](https://img.shields.io/badge/Download-APK-green?style=for-the-badge&logo=android)](../../releases/latest)
[![Download Desktop](https://img.shields.io/badge/Download-Installer-blue?style=for-the-badge&logo=windows)](../../releases/latest)

---

## 📱 Android

APK Socks Client untuk Android — SOCKS5 VPN client yang ringan dan aman.

### ✨ Fitur

- 🧦 **SOCKS5 VPN** via sing-box core v1.10.6 (TCP + UDP)
- 🔒 **Anti DNS Leak** — DNS remote via tunnel
- 🛡️ **Anti Routing Loop** — bind_interface + bypass rule
- ⚡ **Ultra Lightweight** — 7.4MB APK, ~29MB install (57% lebih kecil)
- 🎯 **Protocol Sniffing** — HTTP/TLS/QUIC auto-detect
- 🌐 **IPv4 Support** — IPv6 blocked via DNS strategy
- 📊 **Info Developer** — dialog dengan link Telegram, Website, Trakteer

### 📦 Download

Cek [**Releases**](../../releases) untuk download APK terbaru.

### 🔨 Build dari Source

```bash
cd android
./gradlew assembleRelease
# Output: android/app/build/outputs/apk/release/app-release.apk
```

**Requirements:**
- Android Studio / Gradle 8.0+
- JDK 17+
- Android SDK 34

### 📋 Changelog

#### v1.1.0 (20 Juni 2026)
**Size Optimization & Bug Fixes**
- 🔥 Recompile **sing-box v1.10.6** dengan custom tags (`with_gvisor` only)
- 📉 APK size **-57%**: 17MB → 7.4MB
- 📉 libbox.so **-58%**: 52MB → 21.9MB (arm64-v8a)
- 🐛 Fix disconnect bug — clear last_seen timestamp
- 🐛 Fix connect failed — remove IPv6 action rule untuk v1.10.x compatibility
- ✨ Add Info Developer dialog dengan 3 tombol warna
- 📝 Enable/disable logging untuk production build

#### v1.0.0 (20 Juni 2026)
- 🎉 Initial release
- 🧦 SOCKS5 VPN client dengan TCP + UDP support
- 📊 Traffic counter
- 🎨 Splash screen

---

## 💻 Desktop (Windows)

Aplikasi desktop ringan menggunakan **Go + Walk (Win32 Native)** + **sing-box v1.12.2** sebagai core VPN.

### ✨ Fitur

- 🧦 **SOCKS5 VPN** via sing-box v1.12.2 core (bundled)
- 🖥️ **UI Native Win32** (Walk) — ringan, tanpa WebView2
- 🗂️ **System Tray** — minimize ke tray, tidak keluar
- 🔒 **Single Instance** — tidak bisa buka 2x (Windows mutex)
- 🪟 **Installer** (Inno Setup) — auto-upgrade, auto-close app
- 🛡️ **Process Cleanup** — kill process tree on exit (no orphan TUN)
- 📱 **Info Developer Dialog** — link ke Telegram, Website, Trakteer
- ⚡ **Lightweight** — RAM ~12MB, file size ~13MB
- ✅ **Windows 32-bit & 64-bit** — kompatibel semua Windows

### 📦 Download

Download installer dari [**Releases**](../../releases).

> ⚠️ **Butuh Hak Admin** — App memerlukan hak administrator untuk membuat TUN interface (sing-box).

### 🔨 Build dari Source

```bash
cd desktop

# Prerequisites: Go 1.25+, GCC (CGO), Inno Setup 6

# Build app (64-bit GUI)
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .

# Build installer
# 1. Buka Inno Setup 6
# 2. Open File → setup.iss
# 3. Build → Compile (Ctrl+F9)
# Output: installer_output/SocksClientDesktop_Setup_v1.1.0.exe
```

**Requirements:**
- Go 1.25+
- GCC (MinGW-w64 atau TDM-GCC untuk CGO)
- Inno Setup 6 (untuk build installer)

### 🤖 Build via GitHub Actions

Push tag dengan format `v*-desktop` (contoh: `v1.2.0-desktop`) — GitHub Actions otomatis build dan release installer.

```bash
git tag v1.2.0-desktop
git push origin v1.2.0-desktop
```

### 📁 Struktur Project

```
desktop/
├── main.go              ← Semua kode (UI + logic + tray)
├── go.mod / go.sum
├── app.ico              ← Logo socks (ICO untuk taskbar + tray)
├── app.manifest         ← Admin manifest (UAC)
├── app.rc               ← Resource script
├── rsrc_windows_amd64.syso
├── setup.iss            ← Inno Setup script (installer)
├── embed/
│   ├── sing-box.exe     ← Core VPN v1.12.2 (Git LFS)
│   ├── app.ico          ← Logo socks (embedded)
│   └── logo_store.png   ← Banner JhopanStore
└── .gitignore
```

### 📋 Changelog

#### v1.1.0 (28 Juni 2026)
**🎉 Rilis Perdana Desktop Windows**
- 🧦 SOCKS5 VPN via sing-box v1.12.2 (bundled)
- 🖥️ UI native Win32 (Walk) — ringan, tanpa WebView2
- 🗂️ System tray + minimize to tray
- 🔒 Single instance + Windows mutex
- 🪟 Installer Inno Setup — silent upgrade, auto UAC
- 🛡️ Process cleanup — kill process tree on exit
- 📱 Info Developer dialog
- ⚡ RAM usage ~12MB, total size ~13MB

---

## 🔗 Links

- **GitHub**: [jhopan/SocksClientByJhopanStore](https://github.com/jhopan/SocksClientByJhopanStore)
- **Telegram**: [@jhopan_05](https://t.me/jhopan_05)
- **Website**: [jhopanstore.my.id](https://jhopanstore.my.id)
- **Trakteer**: [trakteer.id/jhopan](https://trakteer.id/jhopan)

---

## 📄 Lisensi

Open Source — JhopanStore

**Made with ❤️ by JhopanStore**
