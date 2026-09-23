//go:build !windows

package engine

import (
	"os"
	"syscall"
)

// terminate mengirim SIGTERM; pemanggil sudah menunggu sebentar sebelum Kill.
func terminate(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	p.Signal(syscall.SIGTERM)
}
