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

// Load membaca settings; file yang belum ada menghasilkan nilai default.
func Load() Settings {
	s := Settings{Port: 1080}
	path, err := Path()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	json.Unmarshal(data, &s)
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
	return os.WriteFile(path, data, 0o600)
}
