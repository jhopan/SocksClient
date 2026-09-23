//go:build windows

package settings

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Di Windows password dienkripsi dengan DPAPI (CryptProtectData) dan disimpan
// di pass.enc, bukan sebagai teks di settings.json. Tidak ada dependency baru:
// crypt32.dll bagian dari Windows, seperti yang dipakai aplikasi Windows lama.

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32             = syscall.NewLazyDLL("crypt32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	pCryptProtectData   = crypt32.NewProc("CryptProtectData")
	pCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	pLocalFree          = kernel32.NewProc("LocalFree")
)

func newBlob(data []byte) *dataBlob {
	if len(data) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
}

func blobBytes(b *dataBlob) []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	return unsafe.Slice(b.pbData, b.cbData)
}

func protect(plain string) string {
	if plain == "" {
		return ""
	}
	in := newBlob([]byte(plain))
	var out dataBlob
	ret, _, _ := pCryptProtectData.Call(uintptr(unsafe.Pointer(in)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&out)))
	if ret == 0 {
		return ""
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return base64.StdEncoding.EncodeToString(blobBytes(&out))
}

func unprotect(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("blob password tidak valid: %w", err)
	}
	in := newBlob(raw)
	var out dataBlob
	ret, _, _ := pCryptUnprotectData.Call(uintptr(unsafe.Pointer(in)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&out)))
	if ret == 0 {
		return "", fmt.Errorf("DPAPI gagal membuka password (akun/mesin berbeda?)")
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return string(blobBytes(&out)), nil
}

func secretPath() string {
	dir, err := Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pass.enc")
}

// KeychainAvailable: di Windows selalu bisa (DPAPI bagian dari OS).
func KeychainAvailable() bool { return true }

// StoreSecret menulis password terenkripsi; password kosong menghapus file.
func StoreSecret(pass string) error {
	path := secretPath()
	if path == "" {
		return fmt.Errorf("direktori konfigurasi tidak bisa dibuat")
	}
	if pass == "" {
		return DeleteSecret()
	}
	enc := protect(pass)
	if enc == "" {
		return fmt.Errorf("DPAPI gagal mengenkripsi password")
	}
	return os.WriteFile(path, []byte(enc), 0o600)
}

// LoadSecret membaca password dari blob DPAPI.
func LoadSecret() (string, error) {
	path := secretPath()
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil // belum pernah disimpan
	}
	return unprotect(string(data))
}

// DeleteSecret menghapus blob password.
func DeleteSecret() error {
	path := secretPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// unprotectLegacy membuka "pass_enc" dari settings.json aplikasi Windows lama
// (blob DPAPI base64 yang sama), supaya pengguna yang memperbarui tidak perlu
// mengetik ulang passwordnya.
func unprotectLegacy(enc string) (string, error) { return unprotect(enc) }
