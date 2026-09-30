//go:build windows

package main

import "os/exec"

// bukaURL membuka URL di browser default. Windows: pakai cmd /c start.
func bukaURL(url string) {
	exec.Command("cmd", "/c", "start", "", url).Start()
}
