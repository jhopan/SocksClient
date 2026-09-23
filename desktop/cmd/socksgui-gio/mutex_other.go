//go:build (linux || darwin) && cgo

package main

// Di Linux/macOS pemakaian bersamaan dijaga oleh lock file core (sing-box) dan
// pemeriksaan root; GUI tidak perlu menambah penjaga sendiri.
func instanceTunggal() bool { return true }
