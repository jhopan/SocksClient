//go:build !windows

package main

import "os/exec"

// bukaURL membuka URL di browser default. Cross-platform: macOS dan Linux.
func bukaURL(url string) {
	exec.Command("xdg-open", url).Start()
}
