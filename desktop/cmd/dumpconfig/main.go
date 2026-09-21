// Command dumpconfig writes the client's sing-box configs to disk.
//
// CI uses it to run `sing-box check` on the exact config the app runs; the
// scripts in desktop/scripts/ use it to test TUN on a real machine. Flags
// mirror what the UI lets the user change - which today is only the server
// coordinates, since stack and MTU are pinned.
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
	variants := flag.Bool("variants", true, "also write the named variants used by CI")
	host := flag.String("host", "10.12.132.225", "SOCKS server")
	port := flag.Int("port", 1080, "SOCKS port")
	user := flag.String("user", "user", "SOCKS username")
	pass := flag.String("pass", "pass", "SOCKS password")
	mtu := flag.Int("mtu", 0, "TUN MTU (0 = default 1400)")
	iface := flag.String("iface", "sb-tun", "TUN interface name")
	logLevel := flag.String("log-level", "info", "sing-box log level")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}

	cfgs := map[string]map[string]interface{}{
		"tun.json": boxcfg.Tun(*host, *port, *user, *pass, boxcfg.TunOptions{
			Stack: boxcfg.StackGVisor, MTU: *mtu, InterfaceName: *iface, LogLevel: *logLevel,
		}),
	}
	if *variants && *host == "10.12.132.225" && *iface == "sb-tun" {
		cfgs["tun-mtu1400.json"] = boxcfg.Tun(*host, *port, *user, *pass,
			boxcfg.TunOptions{MTU: 1400, LogLevel: *logLevel})
		cfgs["tun-mixed.json"] = boxcfg.Tun(*host, *port, *user, *pass,
			boxcfg.TunOptions{Stack: boxcfg.StackMixed, LogLevel: *logLevel})
		cfgs["tun-system.json"] = boxcfg.Tun(*host, *port, *user, *pass,
			boxcfg.TunOptions{Stack: boxcfg.StackSystem, LogLevel: *logLevel})
		cfgs["tun-hostname.json"] = boxcfg.Tun("server.example.com", *port, *user, *pass,
			boxcfg.TunOptions{LogLevel: *logLevel})
	}

	for name, cfg := range cfgs {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fail(err)
		}
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
