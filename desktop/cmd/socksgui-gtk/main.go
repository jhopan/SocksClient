//go:build linux && cgo

// socksgui - GUI Linux (GTK3) untuk Socks Client.
//
// TUN butuh root, jadi jalankan lewat launcher paket (pkexec) atau:
//
//	sudo socksgui -core /usr/lib/socksclient/sing-box
//
// Widget-nya GTK3 milik sistem: binary ~1 MB, RAM rendah karena libgtk sudah
// dimuat sesi desktop. Config tetap dari internal/boxcfg dan ping dari
// internal/ping - sama dengan klien Windows dan CLI.
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
	dataFlag := flag.String("data", "", "direktori kerja (default: $XDG_RUNTIME_DIR/socksclient)")
	flag.Parse()

	core, err := engine.FindCore(*coreFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "socksgui:", err)
		os.Exit(1)
	}

	dataDir := *dataFlag
	if dataDir == "" {
		base := os.Getenv("XDG_RUNTIME_DIR")
		if base == "" {
			base = os.TempDir()
		}
		dataDir = filepath.Join(base, "socksclient")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "socksgui:", err)
		os.Exit(1)
	}

	eng := engine.New(core, dataDir)
	saved := settings.Load()

	// Widget dulu (GTK hanya boleh disentuh dari main thread), baru diikat.
	uiInit()
	state = guicore.Attach(gtkUI{}, eng, saved)

	if os.Geteuid() != 0 {
		showError("TUN butuh hak root.\n\nJalankan lewat menu aplikasi (launcher paket memakai pkexec), " +
			"atau dari terminal:\n\n    sudo socksgui")
		os.Exit(1)
	}

	uiRun()
}
