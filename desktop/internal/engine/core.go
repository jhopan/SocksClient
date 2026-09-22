// Package engine mengendalikan tunnel (proses core sing-box) untuk GUI Linux
// (GTK3) dan macOS (AppKit). Logika config tetap dari internal/boxcfg dan HTTP
// ping dari internal/ping, jadi tiga desktop memakai aturan yang sama.
package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindCore mencari binary sing-box: hint (env -core / $SINGBOX_BIN) dulu, lalu
// di sebelah binary, lalu lokasi paket Linux, terakhir dari PATH.
func FindCore(hint string) (string, error) {
	if hint != "" {
		if st, err := os.Stat(hint); err == nil && st.Size() > 1024 {
			return hint, nil
		}
		return "", fmt.Errorf("core tidak ditemukan di %s", hint)
	}
	self, _ := os.Executable()
	candidates := []string{
		filepath.Join(filepath.Dir(self), "sing-box"),
		"/usr/lib/socksclient/sing-box",
		"/usr/local/lib/socksclient/sing-box",
		filepath.Join(filepath.Dir(self), "sing-box.exe"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.Size() > 1024 {
			return c, nil
		}
	}
	if found, err := exec.LookPath("sing-box"); err == nil {
		return found, nil
	}
	return "", fmt.Errorf("binary sing-box tidak ditemukan - pakai -core PATH atau set SINGBOX_BIN")
}
