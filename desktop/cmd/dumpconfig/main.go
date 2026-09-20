// Command dumpconfig writes the client's sing-box configs to disk.
//
// CI uses it to run `sing-box check` on the exact config the app runs; the
// scripts in desktop/scripts/ use it to test and benchmark TUN on a real
// machine. Flags mirror what the UI lets the user change.
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
	mode := flag.String("mode", "", "write only this mode: tun | proxy (default: both)")
	variants := flag.Bool("variants", true, "also write the named variants used by CI")
	host := flag.String("host", "10.12.132.225", "SOCKS server")
	port := flag.Int("port", 1080, "SOCKS port")
	user := flag.String("user", "user", "SOCKS username")
	pass := flag.String("pass", "pass", "SOCKS password")
	localPort := flag.Int("local-port", 2080, "proxy-mode listen port")
	stack := flag.String("stack", boxcfg.StackSystem, "TUN stack: system | gvisor")
	mtu := flag.Int("mtu", 0, "TUN MTU (0 = default 9000)")
	iface := flag.String("iface", "sb-tun", "TUN interface name")
	logLevel := flag.String("log-level", "info", "sing-box log level")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}

	opts := boxcfg.TunOptions{Stack: *stack, MTU: *mtu, InterfaceName: *iface, LogLevel: *logLevel}
	proxyOpts := boxcfg.ProxyOptions{LocalPort: *localPort, LogLevel: *logLevel}

	cfgs := map[string]map[string]interface{}{}
	if *mode == "" || *mode == boxcfg.ModeTun {
		cfgs["tun.json"] = boxcfg.Tun(*host, *port, *user, *pass, opts)
	}
	if *mode == "" || *mode == boxcfg.ModeProxy {
		cfgs["proxy.json"] = boxcfg.Proxy(*host, *port, *user, *pass, proxyOpts)
	}
	if *variants && *mode == "" && *host == "10.12.132.225" && *iface == "sb-tun" {
		cfgs["tun-gvisor.json"] = boxcfg.Tun(*host, *port, *user, *pass,
			boxcfg.TunOptions{Stack: boxcfg.StackGVisor, LogLevel: *logLevel})
		cfgs["tun-mtu1400.json"] = boxcfg.Tun(*host, *port, *user, *pass,
			boxcfg.TunOptions{MTU: 1400, LogLevel: *logLevel})
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
