//go:build windows

package main

// Di Windows CLI ini hanya dipakai untuk `config`/`check` (aplikasi GUI-nya yang
// menangani TUN), jadi tidak ada pemeriksaan root di sini.
func isRoot() bool { return true }
