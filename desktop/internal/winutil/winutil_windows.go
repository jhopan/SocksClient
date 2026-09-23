//go:build windows

// Package winutil: cek Administrator dan relaunch lewat UAC.
// Dipakai GUI Gio (dan nanti bisa dipakai aplikasi Windows lama).
package winutil

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// IsAdmin melaporkan apakah proses berjalan dengan hak Administrator.
func IsAdmin() bool {
	ok, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	return ok != 0
}

// RelaunchElevated menjalankan ulang executable ini lewat UAC (verb "runas").
// Pemanggil sebaiknya keluar setelah ini: kita mengembalikan error hanya bila
// ShellExecuteW ditolak.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	ret, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		uintptr(unsafe.Pointer(dir)),
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW gagal (kode %d)", ret)
	}
	return nil
}
