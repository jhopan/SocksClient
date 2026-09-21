package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Windows helpers the app still needs now that TUN is the only mode:
// an Administrator check and the UAC re-launch.

func isAdmin() bool {
	ok, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	return ok != 0
}

func relaunchAsAdmin() error {
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
		0, // no arguments: the app reads none
		uintptr(unsafe.Pointer(dir)),
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW gagal (kode %d)", ret)
	}
	return nil
}
