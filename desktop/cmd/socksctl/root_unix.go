//go:build !windows

package main

import "os"

// TUN butuh hak root di Linux dan macOS (bikin interface + atur route).
func isRoot() bool { return os.Geteuid() == 0 }
