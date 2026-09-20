#!/usr/bin/env python3
"""Trim sing-box's protocol registry down to what a SOCKS5 client needs.

The Socks Client core only ever speaks socks (outbound), tun / mixed (inbound),
direct, block and the local DNS transport. sing-box registers everything by
default, so this script edits include/registry.go before the build:

    python3 core/slim-registry.py <path>/include/registry.go

Verified by the "Build Core" workflow: it builds a linux core with this patch
and runs `sing-box check` on the configs emitted by desktop/cmd/dumpconfig.
Failures are loud - if upstream renames something the script exits non-zero and
the release job is skipped (the previous core release stays in place).

Size on windows/amd64 (sing-box v1.14.1, -s -w -trimpath): 36.0 MB -> 27.6 MB.
"""
import pathlib
import re
import sys

# Imports and registration calls for everything a SOCKS5 client never touches.
DROP_IMPORTS = [
    "github.com/sagernet/sing-box/dns/transport/fakeip",
    "github.com/sagernet/sing-box/dns/transport/hosts",
    "github.com/sagernet/sing-box/dns/transport/mdns",
    "github.com/sagernet/sing-box/protocol/anytls",
    "github.com/sagernet/sing-box/protocol/bridge",
    "github.com/sagernet/sing-box/protocol/group",
    "github.com/sagernet/sing-box/protocol/naive",
    "github.com/sagernet/sing-box/protocol/redirect",
    "github.com/sagernet/sing-box/protocol/shadowsocks",
    "github.com/sagernet/sing-box/protocol/shadowtls",
    "github.com/sagernet/sing-box/protocol/snell",
    "github.com/sagernet/sing-box/protocol/ssh",
    "github.com/sagernet/sing-box/protocol/tor",
    "github.com/sagernet/sing-box/protocol/trojan",
    "github.com/sagernet/sing-box/protocol/vless",
    "github.com/sagernet/sing-box/protocol/vmess",
    "github.com/sagernet/sing-box/service/api",
    "github.com/sagernet/sing-box/service/origin_ca",
    "github.com/sagernet/sing-box/service/resolved",
    "github.com/sagernet/sing-box/service/ssmapi",
]

DROP_CALLS = [
    "api.RegisterService(registry)",
    "resolved.RegisterService(registry)",
    "ssmapi.RegisterService(registry)",
    "originca.RegisterCertificateProvider(registry)",
    "registerDERPService(registry)",
    "registerUSBIPServices(registry)",
    "redirect.RegisterRedirect(registry)",
    "redirect.RegisterTProxy(registry)",
    "shadowsocks.RegisterInbound(registry)",
    "snell.RegisterInbound(registry)",
    "vmess.RegisterInbound(registry)",
    "trojan.RegisterInbound(registry)",
    "naive.RegisterInbound(registry)",
    "shadowtls.RegisterInbound(registry)",
    "vless.RegisterInbound(registry)",
    "anytls.RegisterInbound(registry)",
    "bridge.RegisterOutbound(registry)",
    "group.RegisterSelector(registry)",
    "group.RegisterURLTest(registry)",
    "shadowsocks.RegisterOutbound(registry)",
    "snell.RegisterOutbound(registry)",
    "vmess.RegisterOutbound(registry)",
    "trojan.RegisterOutbound(registry)",
    "registerNaiveOutbound(registry)",
    "tor.RegisterOutbound(registry)",
    "ssh.RegisterOutbound(registry)",
    "shadowtls.RegisterOutbound(registry)",
    "vless.RegisterOutbound(registry)",
    "anytls.RegisterOutbound(registry)",
    "hosts.RegisterTransport(registry)",
    "mdns.RegisterTransport(registry)",
    "fakeip.RegisterTransport(registry)",
    "resolved.RegisterTransport(registry)",
]

# Must survive the trim, otherwise we would ship a broken core.
KEEP = [
    "tun.RegisterInbound(registry)",
    "socks.RegisterInbound(registry)",
    "http.RegisterInbound(registry)",
    "mixed.RegisterInbound(registry)",
    "direct.RegisterInbound(registry)",
    "direct.RegisterOutbound(registry)",
    "block.RegisterOutbound(registry)",
    "socks.RegisterOutbound(registry)",
    "http.RegisterOutbound(registry)",
    "local.RegisterTransport(registry)",
]


def main() -> int:
    path = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "include/registry.go")
    src = path.read_text(encoding="utf-8")

    dropped, missing = [], []
    for imp in DROP_IMPORTS:
        pattern = re.compile(r'\n\t(?:[A-Za-z_][A-Za-z0-9_]* )?"' + re.escape(imp) + r'"')
        src, hits = pattern.subn("", src)
        (dropped if hits else missing).append(imp)

    for call in DROP_CALLS:
        pattern = re.compile(r'\n\t' + re.escape(call))
        src, hits = pattern.subn("", src)
        (dropped if hits else missing).append(call)

    absent = [marker for marker in KEEP if marker not in src]
    if absent:
        print("ERROR: registry shape changed, keeping these is required:", file=sys.stderr)
        for marker in absent:
            print("  -", marker, file=sys.stderr)
        return 1

    path.write_text(src, encoding="utf-8", newline="\n")
    print(f"slim: dropped {len(dropped)} registrations/imports, kept {len(KEEP)}")
    if missing:
        print(f"note: {len(missing)} entries were already absent (upstream moved on):")
        for entry in missing[:8]:
            print("  -", entry)
    return 0


if __name__ == "__main__":
    sys.exit(main())
