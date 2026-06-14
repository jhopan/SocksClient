# 🧦 Socks Client by JhopanStore

Client VPN SOCKS5 untuk menghubungkan perangkat ke server SOCKS5 hotspot.

Tersedia untuk **Android** dan **Windows Desktop**.

---

## 📱 Android

APK Socks Client untuk Android.

### Build
```bash
cd android
./gradlew assembleRelease
```

### Download
Cek [Releases](../../releases) untuk APK terbaru.

---

## 💻 Desktop (Windows)

Aplikasi desktop ringan menggunakan **Go + Walk (Win32 Native)** + **sing-box** sebagai core VPN.

### Fitur
- 🧦 SOCKS5 VPN via sing-box core (bundled)
- 🖥️ UI native Win32 (Walk) — ringan, tanpa WebView2
- 🗂️ System tray — close window masuk ke tray
- 🔒 Single instance — tidak bisa buka 2x
- 📊 Traffic counter
- ⚡ RAM ~12MB, file ~12MB
- ✅ Kompatibel Windows 32-bit & 64-bit

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
│   ├── sing-box.exe     ← Core VPN (Git LFS)
│   ├── app.ico          ← Logo socks (embedded)
│   └── logo_store.png   ← Banner JhopanStore
└── .gitignore
```

### Build dari source
```bash
cd desktop

# 64-bit
go build -ldflags="-s -w -H windowsgui" -o socks-client.exe .

# 32-bit (kompatibel semua Windows)
GOOS=windows GOARCH=386 CGO_ENABLED=1 go build -ldflags="-s -w -H windowsgui" -o socks-client-32.exe .

# Compress (opsional)
upx --best --lzma socks-client-32.exe
```

### Build Installer
1. Install [Inno Setup 6](https://jrsoftware.org/isdl.php)
2. Buka `desktop/setup.iss`
3. Build → Compile (Ctrl+F9)
4. Output: `installer_output/SocksClientDesktop_Setup_v1.0.0.exe`

### Install
Download installer dari [Releases](../../releases) atau build dari source.

> ⚠️ **Butuh Admin** — App memerlukan hak admin untuk membuat TUN interface (sing-box).

---

## 📄 Lisensi

Open Source — JhopanStore
