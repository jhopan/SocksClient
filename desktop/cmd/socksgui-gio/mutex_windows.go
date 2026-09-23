//go:build windows && cgo

package main

import (
	"syscall"
	"unsafe"
)

// Nama mutex ini dipakai installer (AppMutex di setup.iss): installer tahu
// aplikasi sedang berjalan tanpa memanggil apa pun ke proses kita.
const mutexName = "SocksClientDesktopMutex"

var handleMutex uintptr

// instanceTunggal mengembalikan false bila aplikasi sudah berjalan.
func instanceTunggal() bool {
	name, _ := syscall.UTF16PtrFromString(mutexName)
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // gagal bikin mutex: jangan blokir aplikasi
	}
	handleMutex = h
	const ERROR_ALREADY_EXISTS = 183
	if errno, ok := err.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		return false
	}
	return true
}
