// Package boxcfg builds the sing-box configuration the desktop client runs.
// Pure stdlib on purpose: the "Build Core" workflow compiles these same configs
// (cmd/dumpconfig) and runs `sing-box check` on them before publishing a core.
package boxcfg

import "strings"

// TUN stacks, straight from sing-box: "system" translates L3->L4 with the OS
// network stack, "gvisor" with gVisor's userspace stack, "mixed" uses system for
// TCP and gvisor for UDP. gvisor is our default: it does not depend on the
// machine's NIC driver or filter stack, which is the usual "TUN starts but
// nothing passes" cause on random laptops.
const (
	StackSystem = "system"
	StackGVisor = "gvisor"
	StackMixed  = "mixed"
)

// TunOptions carries what the UI lets the user change.
type TunOptions struct {
	Stack         string // StackGVisor (default) | StackMixed | StackSystem
	MTU           int    // 0 = default (1400)
	InterfaceName string // TUN adapter name; "sb-tun" when empty
	LogLevel      string // sing-box log level; "info" when empty
	// AutoInterfaceName: jangan kirim field interface_name sama sekali, biarkan
	// core memilih namanya sendiri. Wajib di macOS, yang hanya mengizinkan utunN.
	AutoInterfaceName bool
}

// DefaultInterfaceName mengembalikan nama interface yang aman untuk sebuah OS.
// macOS hanya mengenal utunN, jadi di sana kita kembalikan string kosong yang
// berarti "biarkan core memilih".
func DefaultInterfaceName(goos string) string {
	if goos == "darwin" {
		return ""
	}
	return "sb-tun"
}

func (o TunOptions) stack() string {
	switch o.Stack {
	case StackMixed, StackSystem:
		return o.Stack
	default:
		return StackGVisor
	}
}

// DefaultTunMTU is deliberately on the safe side: hotspot networks frequently
// carry a lower MTU, and oversized packets either stall (PMTU blackhole) or get
// fragmented. 1400 leaves room for the SOCKS overhead on any path >= 1480.
const DefaultTunMTU = 1400

func (o TunOptions) mtu() int {
	if o.MTU >= 576 && o.MTU <= 9000 {
		return o.MTU
	}
	return DefaultTunMTU
}

func (o TunOptions) interfaceName() string {
	if o.InterfaceName != "" {
		return o.InterfaceName
	}
	return "sb-tun"
}

func (o TunOptions) logLevel() string {
	if o.LogLevel != "" {
		return o.LogLevel
	}
	return "info"
}

func tunInbound(opts TunOptions) map[string]interface{} {
	in := map[string]interface{}{
		"type": "tun",
		"address": []string{
			"172.19.0.1/30",
		},
		"mtu":          opts.mtu(),
		"auto_route":   true,
		"strict_route": true,
		"stack":        opts.stack(),
		// Keep the local network reachable: the hotspot itself, the phone's admin
		// page and printers are not internet traffic and must not be swallowed by
		// the tunnel.
		"route_exclude_address": []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16"},
	}
	// macOS: field ini harus kosong supaya core membuat utunN sendiri.
	if !opts.AutoInterfaceName {
		in["interface_name"] = opts.interfaceName()
	}
	return in
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
		"log": map[string]interface{}{"level": opts.logLevel()},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				// queries travel the SOCKS tunnel - a LAN resolver handed out
				// by DHCP can never answer them. 1.1.1.1 primary, 8.8.8.8 kept
				// as the backup entry to promote by changing "final" if the
				// primary is blocked on the network you are on (sing-box has no
				// automatic failover between servers).
				{"tag": "remote", "type": "tcp", "server": "1.1.1.1", "detour": "socks-out"},
				{"tag": "remote-udp", "type": "udp", "server": "1.1.1.1", "detour": "socks-out"},
				{"tag": "backup", "type": "tcp", "server": "8.8.8.8", "detour": "socks-out"},
				// systems resolver, used only to bootstrap the SOCKS server's own
				// hostname; "detour: direct" is rejected by sing-box inside
				// auto_route (it would loop back into the tunnel)
				{"tag": "local", "type": "local"},
			},
			"final":    "remote",
			"strategy": "ipv4_only",
		},
		"inbounds": []map[string]interface{}{tunInbound(opts)},
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
