// Package boxcfg builds the sing-box configuration the desktop client runs.
// Pure stdlib on purpose: the "Build Core" workflow compiles these same configs
// (cmd/dumpconfig) and runs `sing-box check` on them before publishing a core.
package boxcfg

// Mode selects which inbound the core opens.
const (
	ModeTun   = "tun"
	ModeProxy = "proxy"
)

func socksOutbound(host string, port int, user, pass string) map[string]interface{} {
	ob := map[string]interface{}{
		"type": "socks", "tag": "socks-out",
		"server": host, "server_port": port, "version": "5",
	}
	if user != "" {
		ob["username"] = user
		ob["password"] = pass
	}
	return ob
}

// Tun routes every IP packet through the SOCKS5 server. Needs a TUN adapter,
// so the app must run as Administrator.
func Tun(host string, port int, user, pass string) map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"inbounds": []map[string]interface{}{{
			"type": "tun", "interface_name": "sb-tun",
			"address": []string{"172.19.0.1/30"}, "mtu": 9000,
			"auto_route": true, "strict_route": false, "stack": "system",
		}},
		"outbounds": []map[string]interface{}{socksOutbound(host, port, user, pass)},
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			// sniff used to live in the tun inbound; sing-box 1.13 moved it to
			// a route action. The core we ship is built >= 1.13.
			"rules": []map[string]interface{}{{"action": "sniff"}},
			"final": "socks-out",
		},
	}
}

// Proxy is the non-TUN mode: one local mixed inbound (SOCKS5 + HTTP), no
// interface, no routes, no admin rights.
func Proxy(host string, port int, user, pass string, localPort int) map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"inbounds": []map[string]interface{}{{
			"type": "mixed", "tag": "mixed-in",
			"listen": "127.0.0.1", "listen_port": localPort,
		}},
		"outbounds": []map[string]interface{}{socksOutbound(host, port, user, pass)},
	}
}
