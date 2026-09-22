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
func Dir() (string, error) {
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
	return s
}

// Save menulis settings dengan mode 0600 (berisi host/port/user; password hanya
// di Linux, dan itu pun terbatas pada user yang sama).
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
