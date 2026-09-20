// Package boxcfg builds the sing-box configuration the desktop client runs.
// Pure stdlib on purpose: the "Build Core" workflow compiles these same configs
// (cmd/dumpconfig) and runs `sing-box check` on them before publishing a core.
package boxcfg

import "strings"

// Mode selects which inbound the core opens.
const (
	ModeTun   = "tun"
	ModeProxy = "proxy"
)

// TUN stacks. "system" uses the OS stack (default, fastest); "gvisor" is the
// pure-Go stack, the fallback for laptops where the system stack misbehaves.
const (
	StackSystem = "system"
	StackGVisor = "gvisor"
)

// TunOptions carries what the UI lets the user change.
type TunOptions struct {
	Stack string // StackSystem or StackGVisor
	MTU   int    // 0 = default
}

func (o TunOptions) stack() string {
	if o.Stack == StackGVisor {
		return StackGVisor
	}
	return StackSystem
}

func (o TunOptions) mtu() int {
	if o.MTU >= 576 && o.MTU <= 9000 {
		return o.MTU
	}
	return 9000
}

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

// serverRule keeps traffic to the SOCKS server itself out of the tunnel.
// Without it the packets that carry the tunnel try to enter the tunnel: the
// classic routing loop that makes TUN look "connected but dead".
func serverRule(host string) map[string]interface{} {
	if isIP(host) {
		return map[string]interface{}{"ip_cidr": []string{host + "/32"}, "outbound": "direct"}
	}
	return map[string]interface{}{"domain": []string{host}, "outbound": "direct"}
}

func isIP(host string) bool {
	if host == "" {
		return false
	}
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// Tun routes every IP packet through the SOCKS5 server. Needs a TUN adapter,
// so the app must run as Administrator.
//
// DNS is deliberately part of the tunnel: all DNS traffic is hijacked and sent
// to the remote resolver over the SOCKS connection, so a laptop whose DHCP
// hands out a LAN resolver cannot leak queries outside the tunnel.
func Tun(host string, port int, user, pass string, opts TunOptions) map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				// queries travel the SOCKS tunnel - a LAN resolver handed out
				// by DHCP can never answer them
				{"tag": "remote", "type": "tcp", "server": "8.8.8.8", "detour": "socks-out"},
				{"tag": "remote-udp", "type": "udp", "server": "8.8.8.8", "detour": "socks-out"},
				// systems resolver, used only to bootstrap the SOCKS server's own
				// hostname; "detour: direct" is rejected by sing-box inside
				// auto_route (it would loop back into the tunnel)
				{"tag": "local", "type": "local"},
			},
			"final":    "remote",
			"strategy": "ipv4_only",
		},
		"inbounds": []map[string]interface{}{{
			"type": "tun", "interface_name": "sb-tun",
			"address": []string{"172.19.0.1/30"}, "mtu": opts.mtu(),
			"auto_route": true, "strict_route": false, "stack": opts.stack(),
		}},
		"outbounds": []map[string]interface{}{
			socksOutbound(host, port, user, pass),
			// referenced by the bootstrap DNS server and by the anti-loop route
			// rule; sing-box 1.14 does not create a "direct" outbound implicitly
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			// domain resolution for the SOCKS server itself (bootstrap) only
			"default_domain_resolver": map[string]interface{}{"server": "local"},
			"rules": []map[string]interface{}{
				// sniff used to live in the tun inbound; sing-box 1.13 moved it
				// to a route action.
				{"action": "sniff"},
				// every DNS query (including to a LAN resolver) is answered by
				// the tunnel instead of leaking out of the network interface
				{"protocol": "dns", "action": "hijack-dns"},
				serverRule(host),
			},
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
