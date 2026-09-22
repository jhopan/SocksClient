//go:build darwin && cgo

// socksgui - GUI macOS (AppKit) untuk Socks Client.
//
// TUN butuh admin:
//
//	sudo socksgui -core /usr/local/lib/socksclient/sing-box
//
// atau klik dua kali SocksClient.app / SocksClient.command di dalam paket.
// Widget-nya AppKit milik OS: binary ~1 MB, RAM rendah karena kerangka kerja itu
// sudah dimuat sistem. Config tetap dari internal/boxcfg.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"socks-client-desktop/internal/engine"
	"socks-client-desktop/internal/guicore"
	"socks-client-desktop/internal/settings"
)

const appVersion = "1.6.0"

func main() {
	coreFlag := flag.String("core", os.Getenv("SINGBOX_BIN"), "path binary sing-box")
	dataFlag := flag.String("data", "", "direktori kerja (default: ~/Library/Caches/socksclient)")
	flag.Parse()

	core, err := engine.FindCore(*coreFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "socksgui:", err)
		os.Exit(1)
	}

	dataDir := *dataFlag
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "socksgui:", err)
			os.Exit(1)
		}
		dataDir = filepath.Join(home, "Library", "Caches", "socksclient")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "socksgui:", err)
		os.Exit(1)
	}

	eng := engine.New(core, dataDir)
	saved := settings.Load()

	// NSApplication boleh dibangun sebelum root-check; dialog peringatannya
	// memakai NSAlert yang butuh NSApp hidup.
	createDelegate()
	uiInit()
	state = guicore.Attach(macUI{}, eng, saved)

	if os.Geteuid() != 0 {
		showError("TUN butuh hak admin.\n\nJalankan dari paket (SocksClient.command akan meminta " +
			"password admin), atau dari terminal:\n\n    sudo socksgui")
		os.Exit(1)
	}

	uiRun()
}
