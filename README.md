<div align="center">

# Socks Client

**SOCKS5 Tunnel Client** - Android - Windows - Linux - macOS

Route semua trafik (TCP, UDP, DNS) melalui server SOCKS5 kamu.
Anti DNS leak, hemat baterai, satu GUI untuk semua desktop.

[![Android](https://img.shields.io/badge/Android-APK-3ddc84?style=for-the-badge&logo=android&logoColor=white)](../../releases/latest)
[![Windows](https://img.shields.io/badge/Windows-Installer-0078d4?style=for-the-badge&logo=windows&logoColor=white)](../../releases/latest)
[![Linux](https://img.shields.io/badge/Linux-.deb-fcc624?style=for-the-badge&logo=linux&logoColor=black)](../../releases/latest)
[![macOS](https://img.shields.io/badge/macOS-Universal-000000?style=for-the-badge&logo=apple&logoColor=white)](../../releases/latest)
[![Version](https://img.shields.io/github/v/release/jhopan/SocksClient?style=for-the-badge&color=blue)](../../releases)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

</div>

---

## Fitur

| Fitur | Detail |
|---|---|
| **Anti DNS Leak** | Semua query DNS di-tunnel melalui SOCKS (`hijack-dns` + WFP blocking di Windows) |
| **TUN gvisor** | Userspace network stack — tidak bergantung driver/kernel perangkat |
| **MTU 1400** | Dioptimalkan untuk hotspot mobile (mencegah PMTU blackhole) |
| **Hemat baterai** | Android FGS `systemExempted` — tidak mati saat layar off |
| **No console popup** | Core sing-box di-spawn dengan `CREATE_NO_WINDOW` (Windows) |
| **HTTP ping** | Cek koneksi tiap 30 detik via `generate_204` (opsional, checkbox) |
| **Preflight check** | Validasi IP → TCP connect → SOCKS5 auth sebelum tunnel naik |
| **Cross-platform GUI** | Satu kode Gio untuk Windows, Linux, macOS |
| **Password terenkripsi** | Android Keystore (AES-GCM) · Windows DPAPI · Linux file 0600 |

## Download

| Platform | File | Info |
|---|---|---|
| **Android** | `app-universal-release.apk` | arm64 + armv7, Android 7.0+ |
| **Windows** | `SocksClientDesktop_Setup_v*.exe` | Installer + core, Start Menu, uninstaller |
| **Linux** | `socksclient_*_amd64.deb` | Debian/Ubuntu/Mint, CLI + GUI |
| **macOS** | `SocksClient-macos-universal-*.tar.gz` | Intel + Apple Silicon |

→ [**Latest Release**](../../releases/latest)

## Quick Start

### Android
1. Download APK → install → buka
2. Isi **Host** (IP server) dan **Port** (default 1080)
3. Username & Password opsional
4. Tap **Connect** → izinkan VPN

### Windows
1. Download installer `.exe` → install
2. Buka aplikasi (UAC muncul — butuh admin untuk TUN)
3. Isi **Host** dan **Port** → klik **Connect**

### Linux
```bash
sudo dpkg -i socksclient_*_amd64.deb
sudo socksgui                          # GUI (Gio)
# atau headless:
sudo socksctl up -host 10.0.0.1 -port 1080
```

### macOS
```bash
tar -xzf SocksClient-macos-universal-*.tar.gz
cd SocksClient && ./install.sh
sudo socksctl up -host 10.0.0.1 -port 1080
```

> ⚠️ **Host wajib IP literal** (contoh `10.0.0.1`), bukan hostname.
> Hostname akan bocor DNS di luar tunnel.

## Arsitektur

```
Device → TUN (gvisor, MTU 1400) → sing-box → SOCKS5 → Server → Internet
              │
              ├── DNS hijack: semua query dijawab lewat tunnel
              ├── Anti-loop: trafik ke server sendiri bypass tunnel
              └── strict_route: tidak ada app yang bisa bypass
```

### Repo structure
```
socks-clients/
├── android/          # Java + libbox (Android VPN Service)
├── desktop/          # Go + Gio (Windows, Linux, macOS)
│   ├── cmd/socksgui-gio/   # GUI cross-platform
│   ├── cmd/socksctl/       # CLI headless
│   ├── internal/           # engine, settings, ping, boxcfg
│   └── embed/             # sing-box.exe (dari CI, bukan git)
├── core/             # Config referensi sing-box
└── .github/workflows/     # CI + release
```

## Build dari Source

```bash
# Desktop (Windows/Linux/macOS) — dari desktop/
bash scripts/fetch-core.sh              # unduh sing-box core
CGO_ENABLED=1 go build -trimpath \
  -ldflags="-s -w -H windowsgui" \
  -o SocksClient.exe ./cmd/socksgui-gio

# Android — dari android/
bash scripts/fetch-core.sh              # unduh libbox.aar
./gradlew assembleRelease               # butuh keystore signing

# CLI saja (Linux/macOS)
go build -o socksctl ./cmd/socksctl
```

### Signing APK
```bash
export KEYSTORE_FILE=/path/to/socksclient-release.jks
export KEYSTORE_PASSWORD=... KEY_ALIAS=socksclient KEY_PASSWORD=...
./gradlew assembleRelease
```

GitHub Secrets: `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD`

## Spec

| | Android | Windows | Linux | macOS |
|---|---|---|---|---|
| GUI | Java + libbox | Gio (cgo) | Gio (cgo) | Gio (cgo) |
| TUN | VpnService | wintun | gvisor | gvisor |
| RAM idle | ~50 MB | ~50 MB | ~50 MB | ~50 MB |
| Binary | 16-32 MB | 13 MB | 12 MB | 12 MB |
| DNS | hijack + VPN DNS | hijack + WFP | hijack | hijack |

## Keamanan

| Aspek | Implementasi |
|---|---|
| DNS | `1.1.1.1` (primary) + `8.8.8.8` (backup), via tunnel |
| IPv6 | Diblokir (`ipv4_only`) |
| Password | Android Keystore · Windows DPAPI · Linux 0600 |
| Signing | Keystore RSA 2048, validitas 100 tahun |
| Config | Ditulis mode 0600, dihapus setelah core berhenti |

## QA

```bash
# Test lokal (desktop)
cd desktop && go test -count=1 ./internal/...

# TUN selftest (butuh admin)
bash desktop/scripts/tun-selftest.sh

# DNS leak test
# Buka https://dnsleaktest.com → Extended test
# Semua resolver harus dari SOCKS server, bukan ISP
```

## Versi

| Perubahan | Versi | Contoh |
|---|---|---|
| Fitur baru | naik minor | `1.7.0` → `1.8.0` |
| Bug fix / rebuild | tambah segmen 4 | `1.7.0` → `1.7.0.1` → `1.7.0.7` |
| Core only | tetap | tag `core` |

3 release hidup maksimal: **desktop** · **APK** · **core**

## Developer

**JhopanStore**

[![Telegram](https://img.shields.io/badge/Telegram-@jhopan__05-26A5E4?style=flat-square&logo=telegram&logoColor=white)](https://t.me/jhopan_05)
[![Website](https://img.shields.io/badge/Website-jhopanstore.my.id-4FC3F7?style=flat-square&logo=googlechrome&logoColor=white)](https://jhopanstore.my.id)

## License

MIT — see [LICENSE](LICENSE)
