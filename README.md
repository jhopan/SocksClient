# 🧦 Socks Client by JhopanStore

Client VPN SOCKS5 untuk menghubungkan perangkat ke server SOCKS5 hotspot.

Tersedia untuk **Android** dan **Windows Desktop**.

---

## 📱 Android

APK Socks Client untuk perangkat Android.

### Download

| Arsitektur | Ukuran | Link |
|-----------|--------|------|
| arm64-v8a | ~18 MB | [Release](../../releases) |
| armeabi-v7a | ~17 MB | [Release](../../releases) |
| Universal | ~34 MB | [Release](../../releases) |

### Cara Pakai
1. Pastikan HP server menjalankan **VPN Hospot** app
2. Hubungkan HP client ke hotspot/USB tether server
3. Buka Socks Client, isi Host/IP server dan Port (default 1080)
4. Klik **Connect Socks VPN**
5. Izinkan koneksi VPN
6. Semua traffic HP client kini lewat SOCKS5! ✓

### Build dari Source
```bash
cd android
./gradlew assembleRelease
```

---

## 💻 Desktop (Windows)

Aplikasi desktop ringan untuk Windows, powered by **Go + WebView2 + sing-box**.

### Download

| File | Ukuran | Keterangan |
|------|--------|------------|
| `SocksClientDesktop_Setup.exe` | ~11 MB | Installer (Inno Setup) — **Recommended** |
| `socks-client.exe` | ~11 MB | Portable (single file) |

### System Requirements
- Windows 10/11 (64-bit)
- WebView2 Runtime (sudah built-in di Windows 11)
- Admin privileges (untuk TUN interface)

### Cara Pakai
1. Download dan jalankan installer (atau portable .exe)
2. App otomatis minta **Admin privileges**
3. Isi Host/IP server, Port (default 1080)
4. Klik **Connect Socks VPN**
5. Semua traffic PC kini lewat SOCKS5! ✓

### Fitur Desktop
- 🪟 **Modern UI** — Dark theme, responsive
- 🗂️ **System Tray** — Minimize ke tray, connect/disconnect dari tray menu
- 📊 **Traffic Counter** — Monitor download/upload realtime
- 🔒 **Single .exe** — 11MB, zero dependencies (WebView2 built-in)
- ⚡ **Super Ringan** — RAM ~22MB (Go backend)
- 🪪 **Taskbar Icon** — Logo Socks Client

### Build dari Source

**Requirements:**
- Go 1.21+
- MinGW (GCC) untuk windres

```bash
cd desktop

# Build
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .

# Compress dengan UPX (opsional)
upx --best --lzma socks-client.exe

# Build installer (butuh Inno Setup)
# Compile setup.iss dengan Inno Setup Compiler
```

---

## 🌐 Social Media

- 📱 **Telegram:** [@JhopanStore](https://t.me/JhopanStore)
- 📸 **Instagram:** @jhopanstore
- 🎬 **YouTube:** JhopanStore

## 📦 Related Projects

- [VPN Hospot by JhopanStore](https://github.com/jhopan/VpnHospotByJhopanStore) — Server SOCKS5 untuk Android

## 📄 License

MIT License — Free for personal and commercial use.
