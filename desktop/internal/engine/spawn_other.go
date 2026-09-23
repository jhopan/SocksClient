//go:build !windows

package engine

import "os/exec"

// Di Linux/macOS tidak ada console terpisah: core jalan sebagai proses biasa.
func spawnHidden(*exec.Cmd) {}
