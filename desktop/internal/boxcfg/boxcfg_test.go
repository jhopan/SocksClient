package boxcfg

import "testing"

// sing-box `check` validates syntax only: a route rule or DNS detour pointing at
// a tag that does not exist passes check and then kills the service at runtime
// ("outbound detour not found"). That bug shipped once - this test is the gate.
func TestReferencedTagsExist(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"tun":          Tun("10.12.132.225", 1080, "user", "pass", TunOptions{}),
		"tun-gvisor":   Tun("10.12.132.225", 1080, "user", "pass", TunOptions{Stack: StackGVisor}),
		"tun-mixed":    Tun("10.12.132.225", 1080, "user", "pass", TunOptions{Stack: StackMixed}),
		"tun-system":   Tun("10.12.132.225", 1080, "user", "pass", TunOptions{Stack: StackSystem}),
		"tun-hostname": Tun("server.example.com", 1080, "", "", TunOptions{}),
	}

	for name, cfg := range cases {
		tags := map[string]bool{}
		outbounds, _ := cfg["outbounds"].([]map[string]interface{})
		for _, ob := range outbounds {
			if tag, ok := ob["tag"].(string); ok {
				tags[tag] = true
			}
		}
		if len(tags) == 0 {
			t.Fatalf("%s: no outbound tags", name)
		}

		check := func(where, tag string) {
			if tag == "" {
				t.Errorf("%s: %s references an empty outbound tag", name, where)
				return
			}
			if !tags[tag] {
				t.Errorf("%s: %s references undefined outbound %q (defined: %v)", name, where, tag, tags)
			}
		}

		var dnsServers []map[string]interface{}
		if dns, ok := cfg["dns"].(map[string]interface{}); ok {
			dnsServers, _ = dns["servers"].([]map[string]interface{})
			serverTags := map[string]bool{}
			for _, srv := range dnsServers {
				if tag, ok := srv["tag"].(string); ok {
					serverTags[tag] = true
				}
			}
			// dns.final points at a DNS server tag, everything else at an outbound
			if final, ok := dns["final"].(string); ok && !serverTags[final] {
				t.Errorf("%s: dns.final references unknown DNS server %q (defined: %v)", name, final, serverTags)
			}
			for _, srv := range dnsServers {
				if detour, ok := srv["detour"].(string); ok {
					check("dns server "+srv["tag"].(string), detour)
				}
			}
		}

		route, _ := cfg["route"].(map[string]interface{})
		if route == nil {
			continue
		}
		if final, ok := route["final"].(string); ok {
			check("route.final", final)
		}
		if resolver, ok := route["default_domain_resolver"].(map[string]interface{}); ok {
			if tag, ok := resolver["server"].(string); ok {
				found := false
				for _, srv := range dnsServers {
					if srv["tag"] == tag {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: default_domain_resolver references unknown DNS server %q", name, tag)
				}
			}
		}
		rules, _ := route["rules"].([]map[string]interface{})
		for _, rule := range rules {
			if outbound, ok := rule["outbound"].(string); ok {
				check("route rule", outbound)
			}
		}
	}
}

// The anti-loop rule must be present, otherwise the tunnel's own packets try to
// enter the tunnel.
func TestTunHasAntiLoopRule(t *testing.T) {
	for _, host := range []string{"10.12.132.225", "server.example.com"} {
		cfg := Tun(host, 1080, "", "", TunOptions{})
		route := cfg["route"].(map[string]interface{})
		rules := route["rules"].([]map[string]interface{})
		found := false
		for _, rule := range rules {
			if rule["outbound"] != "direct" {
				continue
			}
			if cidrs, ok := rule["ip_cidr"].([]string); ok && len(cidrs) == 1 && cidrs[0] == host+"/32" {
				found = true
			}
			if domains, ok := rule["domain"].([]string); ok && len(domains) == 1 && domains[0] == host {
				found = true
			}
		}
		if !found {
			t.Errorf("no anti-loop rule for %s", host)
		}
	}
}

// DNS must never be answered outside the tunnel in TUN mode.
func TestTunHijacksDNS(t *testing.T) {
	cfg := Tun("10.12.132.225", 1080, "", "", TunOptions{})
	rules := cfg["route"].(map[string]interface{})["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if rule["action"] == "hijack-dns" {
			return
		}
	}
	t.Fatal("tun config does not hijack DNS")
}

// Each stack must land in the config, and the default MTU must stay at the
// hotspot-safe value - the desktop defaults are the answer to "TUN does not
// work on some laptops", so they are worth pinning down.
func TestTunStacksAndMTU(t *testing.T) {
	for _, stack := range []string{StackGVisor, StackMixed, StackSystem} {
		cfg := Tun("10.12.132.225", 1080, "user", "pass", TunOptions{Stack: stack})
		inbound := cfg["inbounds"].([]map[string]interface{})[0]
		if inbound["stack"] != stack {
			t.Fatalf("stack %q rendered as %v", stack, inbound["stack"])
		}
		if inbound["mtu"] != DefaultTunMTU {
			t.Fatalf("default mtu rendered as %v, want %d", inbound["mtu"], DefaultTunMTU)
		}
	}
	// unknown stack falls back to gvisor, not to the stack that breaks on laptops
	cfg := Tun("10.12.132.225", 1080, "user", "pass", TunOptions{Stack: "nonsense"})
	if got := cfg["inbounds"].([]map[string]interface{})[0]["stack"]; got != StackGVisor {
		t.Fatalf("unknown stack fell back to %v, want %s", got, StackGVisor)
	}
}
