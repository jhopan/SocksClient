# Socks Client by JhopanStore

Android SOCKS5 VPN Client berbasis **sing-box** (libbox) dengan TUN interface.

## Arsitektur

```
┌─────────────┐     SOCKS5      ┌─────────────┐     Internet
│  HP Client  │ ───────────────→ │  HP Server  │ ──────────────→
│ (App ini)   │   via Hotspot    │ (VPN Hospot)│   via Cellular
│  sing-box   │   TCP + UDP      │  SOCKS5+HTTP│
│  TUN IPv4   │                  │  UDP Relay  │
└─────────────┘                  └─────────────┘
```

## Build

### Prasyarat
- Android Studio Hedgehog+ / Gradle 8.x
- JDK 17
- Android SDK (compileSdk 35, minSdk 24)

### Langkah Build
1. Clone repo ini
2. **Install Git LFS** lalu pull file binary:
   ```bash
   git lfs install
   git lfs pull
   ```
3. Buka di Android Studio atau build via CLI:
   ```bash
   ./gradlew :app:assembleDebug
   ```
4. APK ada di `app/build/outputs/apk/debug/app-debug.apk`

### libbox.aar
File `app/libs/libbox.aar` (72MB) adalah library sing-box untuk Android.
- **Sudah termasuk** di repo ini via Git LFS
- **Source**: [SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- **Build dari source**: Compile sing-box Go library untuk Android (lihat [F-Droid build recipe](https://gitlab.com/fdroid/fdroiddata/-/blob/master/metadata/io.nekohasekai.sfa.yml))

## Konfigurasi

### Server
- IP server SOCKS5 default: `10.12.132.225`
- Port SOCKS5: `1080`
- Port HTTP: `8080`

### DNS
- Remote: `tcp://8.8.8.8` via SOCKS tunnel
- Local: `1.1.1.1` via direct (bootstrap)
- Strategy: `ipv4_only` (IPv6 di-block karena server tidak support IPv6 routing)

### Route Rules
1. DNS protocol → `direct`
2. Server IP (`10.12.132.225/32`) → `direct` (anti routing loop)
3. IPv6 → `reject` (block semua IPv6)
4. Sisanya → `socks-out`

## Struktur

```
app/src/main/java/com/jhopanstore/socksclient/
├── MainActivity.java        # UI: input server, connect/disconnect
├── SocksVpnService.java     # VPN Service + sing-box config builder
└── DebugLog.java            # Logging helper
```

## License

Copyright (C) JhopanStore
