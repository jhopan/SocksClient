package main

import (
	"encoding/base64"
	"fmt"
	"syscall"
	"unsafe"
)

// DPAPI (CryptProtectData/CryptUnprotectData) mengikat data ke akun Windows yang
// sedang login, jadi settings.json tidak lagi menyimpan password SOCKS mentah.
// Tidak ada dependency baru: crypt32.dll sudah bagian dari Windows.

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

func blobBytes(blob *dataBlob) []byte {
	if blob.cbData == 0 || blob.pbData == nil {
		return nil
	}
	return unsafe.Slice(blob.pbData, blob.cbData)
}

// protectPassword mengembalikan base64 blob DPAPI, atau "" bila gagal (pemanggil
// lalu menyimpan tanpa enkripsi daripada kehilangan datanya).
func protectPassword(plain string) string {
	if plain == "" {
		return ""
	}
	in := newBlob([]byte(plain))
	var out dataBlob
	ret, _, _ := pCryptProtectData.Call(
		uintptr(unsafe.Pointer(in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return ""
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return base64.StdEncoding.EncodeToString(blobBytes(&out))
}

// unprotectPassword membalikkan protectPassword. Error dilaporkan supaya UI bisa
// memberi tahu kalau file settings berasal dari akun/mesin lain.
func unprotectPassword(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("blob password tidak valid: %w", err)
	}
	in := newBlob(raw)
	var out dataBlob
	ret, _, _ := pCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return "", fmt.Errorf("DPAPI gagal membuka password (akun/mesin berbeda?)")
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return string(blobBytes(&out)), nil
}
