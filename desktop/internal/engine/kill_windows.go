//go:build windows

package engine

import (
	"os/exec"
	"strconv"
)

// terminate mematikan core beserta anak prosesnya. Di Windows harus taskkill
// /F /T: kalau tidak, interface TUN (wintun) bisa tertinggal.
func terminate(pid int) {
	exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
