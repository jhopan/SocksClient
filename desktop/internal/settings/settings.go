// Package settings menyimpan pengaturan klien (host, port, user, password, ping)
// untuk CLI dan GUI Linux/macOS. Stdlib saja.
//
// Lokasi: ${XDG_CONFIG_HOME:-~/.config}/socksclient/settings.json, mode 0600.
// Di macOS password disimpan di Keychain (perintah `security`), bukan di file.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Settings adalah isi file pengaturan.
type Settings struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	// Pass hanya dipakai di Linux (file 0600). Di macOS password diambil dari
	// Keychain, jadi field ini kosong saat disimpan.
	Pass string `json:"pass,omitempty"`
	Ping bool   `json:"ping"`
}

// Dir mengembalikan direktori konfigurasi, dibuat bila belum ada.
// Windows: %LOCALAPPDATA%\SocksClientDesktop - folder yang sama dengan aplikasi
// Windows lama, dan di luar direktori instalasi.
func Dir() (string, error) {
	if dir := os.Getenv("LOCALAPPDATA"); runtime.GOOS == "windows" && dir != "" {
		out := filepath.Join(dir, "SocksClientDesktop")
		if err := os.MkdirAll(out, 0o700); err != nil {
			return "", err
		}
		return out, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "socksclient")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Path adalah lokasi file settings.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// pathCadangan adalah salinan setelan terakhir yang isinya masih lengkap.
// Dipakai kalau file utama hilang atau kehilangan alamat server (mis. karena
// form kosong pernah tersimpan).
func pathCadangan() (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "settings.json.bak"), nil
}

// Load membaca settings; file yang belum ada menghasilkan nilai default.
func Load() Settings {
	s := Settings{Port: 1080}
	path, err := Path()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// file utama hilang: pakai salinan cadangan kalau ada
		if bak, err2 := pathCadangan(); err2 == nil {
			if data2, err3 := os.ReadFile(bak); err3 == nil {
				json.Unmarshal(data2, &s)
			}
		}
	}
	json.Unmarshal(data, &s)
	if s.Host == "" {
		// alamat server hilang di file utama: ambil dari cadangan
		if bak, err := pathCadangan(); err == nil {
			if data2, err := os.ReadFile(bak); err == nil {
				var b Settings
				if json.Unmarshal(data2, &b) == nil && b.Host != "" {
					s.Host, s.User = b.Host, b.User
					if b.Port >= 1 && b.Port <= 65535 {
						s.Port = b.Port
					}
				}
			}
		}
	}
	if s.Port < 1 || s.Port > 65535 {
		s.Port = 1080
	}
	// macOS (Keychain) dan Windows (DPAPI) tidak menyimpan password di file.
	if KeychainAvailable() {
		if pass, err := LoadSecret(); err == nil && pass != "" {
			s.Pass = pass
		} else if s.Pass == "" {
			// migrasi dari aplikasi Windows lama: {"pass_enc": "<blob DPAPI>"}
			var lama struct {
				PassEnc string `json:"pass_enc"`
			}
			if json.Unmarshal(data, &lama) == nil && lama.PassEnc != "" {
				if pass, err := unprotectLegacy(lama.PassEnc); err == nil {
					s.Pass = pass
				}
			}
		}
	}
	return s
}

// Save menulis settings dengan mode 0600 (berisi host/port/user; password hanya
// di Linux, dan itu pun terbatas pada user yang sama).
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if KeychainAvailable() {
		if err := StoreSecret(s.Pass); err != nil {
			return err
		}
		s.Pass = "" // jangan tulis password ke settings.json
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Simpan salinan isi lama dulu: kalau nanti file utama rusak/kosong, setelan
	// pengguna (alamat server) masih bisa dipulihkan.
	if lama, err := os.ReadFile(path); err == nil && len(lama) > 0 {
		if bak, err := pathCadangan(); err == nil {
			_ = os.WriteFile(bak, lama, 0o600)
		}
	}
	return os.WriteFile(path, data, 0o600)
}
