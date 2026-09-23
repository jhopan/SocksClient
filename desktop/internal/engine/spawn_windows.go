//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW: sing-box.exe adalah binary CONSOLE, jadi kalau dijalankan
// dari aplikasi GUI (subsystem GUI) tanpa flag ini Windows akan memunculkan
// jendela terminal hitam di atas aplikasi.
const createNoWindow = 0x08000000

func spawnHidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
