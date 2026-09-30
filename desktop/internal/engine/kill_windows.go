//go:build windows

package engine

import (
	"os/exec"
	"strconv"
	"syscall"
)

// terminate mematikan core beserta anak prosesnya. Di Windows harus taskkill
// /F /T: kalau tidak, interface TUN (wintun) bisa tertinggal.
// CREATE_NO_WINDOW + HideWindow: taskkill.exe adalah binary CONSOLE — tanpa
// flag ini, setiap Disconnect memunculkan jendela terminal hitam kedip sebentar.
func terminate(pid int) {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	cmd.Run()
}
