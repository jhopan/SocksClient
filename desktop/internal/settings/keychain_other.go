//go:build !darwin

package settings

// Linux (dan platform lain) tidak punya Keychain: password ikut di
// settings.json yang mode-nya 0600 dan hanya bisa dibaca user yang sama.
func KeychainAvailable() bool { return false }

// StoreSecret tidak dipakai di luar macOS.
func StoreSecret(string) error { return nil }

// LoadSecret mengembalikan kosong; pemanggil memakai field Pass dari file.
func LoadSecret() (string, error) { return "", nil }

// DeleteSecret tidak dipakai di luar macOS.
func DeleteSecret() error { return nil }
