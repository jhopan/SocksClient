// Command dumpconfig writes the desktop client configs to disk so CI can run
// `sing-box check` on the exact config the app runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"socks-client-desktop/internal/boxcfg"
)

func main() {
	out := flag.String("out", ".", "output directory")
	flag.Parse()

	cfgs := map[string]map[string]interface{}{
		"tun.json": boxcfg.Tun("10.12.132.225", 1080, "user", "pass",
			boxcfg.TunOptions{Stack: boxcfg.StackSystem}),
		"tun-gvisor.json": boxcfg.Tun("10.12.132.225", 1080, "user", "pass",
			boxcfg.TunOptions{Stack: boxcfg.StackGVisor}),
		"tun-mtu1400.json": boxcfg.Tun("10.12.132.225", 1080, "user", "pass",
			boxcfg.TunOptions{Stack: boxcfg.StackSystem, MTU: 1400}),
		"tun-hostname.json": boxcfg.Tun("server.example.com", 1080, "user", "pass",
			boxcfg.TunOptions{Stack: boxcfg.StackSystem}),
		"proxy.json": boxcfg.Proxy("10.12.132.225", 1080, "user", "pass", 2080),
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, cfg := range cfgs {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
