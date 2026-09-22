//go:build darwin

package settings

import (
	"os/exec"
	"strings"
)

// Di macOS password disimpan di Keychain, bukan di settings.json.
const (
	keychainService = "socksclient"
	keychainAccount = "socksclient"
)

// KeychainAvailable selalu true di macOS: perintah `security` bagian dari OS.
func KeychainAvailable() bool {
	_, err := exec.LookPath("security")
	return err == nil
}

// StoreSecret menyimpan/memperbarui password di Keychain.
func StoreSecret(pass string) error {
	if pass == "" {
		return DeleteSecret()
	}
	return exec.Command("security", "add-generic-password", "-U",
		"-a", keychainAccount, "-s", keychainService, "-w", pass).Run()
}

// LoadSecret membaca password; string kosong kalau belum pernah disimpan.
func LoadSecret() (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-a", keychainAccount, "-s", keychainService, "-w").Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// DeleteSecret menghapus item Keychain.
func DeleteSecret() error {
	exec.Command("security", "delete-generic-password",
		"-a", keychainAccount, "-s", keychainService).Run()
	return nil
}
