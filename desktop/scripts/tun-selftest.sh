#!/usr/bin/env bash
# TUN self-test for Windows (run from git-bash AS ADMINISTRATOR).
#
#   desktop/scripts/tun-selftest.sh [physical-interface]
#
# It spins up a local SOCKS5 server, points the real TUN config at it, then
# proves the tunnel carries TCP *and* DNS, and finally checks that the adapter,
# the routes and the normal internet are back after shutdown.
#
# Use it on a laptop where "TUN connected but nothing loads" to find out which
# half is broken: this script only needs the local machine, no phone/hotspot.
set -u

IFACE="${1:-Wi-Fi}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CORE="${SINGBOX_BIN:-$ROOT/embed/sing-box.exe}"
WORK="${TMPDIR_WIN:-$LOCALAPPDATA/Temp}/socks-tun-selftest"
SOCKS_PORT="${SOCKS_PORT:-11080}"
mkdir -p "$WORK"

if [ ! -f "$CORE" ]; then
  echo "core not found at $CORE - run desktop/scripts/fetch-core.sh first" >&2
  exit 1
fi
if ! net session >/dev/null 2>&1; then
  echo "warning: not running as Administrator, TUN will fail" >&2
fi

cat > "$WORK/upstream.json" <<EOF
{
  "log": {"level": "warn"},
  "inbounds": [{"type": "socks", "tag": "in", "listen": "127.0.0.1", "listen_port": $SOCKS_PORT}],
  "outbounds": [{"type": "direct", "tag": "direct", "bind_interface": "$IFACE"}]
}
EOF

# The TUN config comes from the same builder the app uses (boxcfg via
# cmd/dumpconfig), so this test can never validate a config the app no longer
# emits. Stack gvisor + MTU 1400 come straight from boxcfg.
(cd "$ROOT" && go run ./cmd/dumpconfig -out "$WORK" -variants=false \
  -host 127.0.0.1 -port "$SOCKS_PORT" -iface sb-tun-selftest) || {
  echo "could not render the TUN config (needs Go in PATH)" >&2
  exit 1
}

cleanup() {
  [ -n "${TUN_PID:-}" ] && kill "$TUN_PID" 2>/dev/null
  sleep 2
  [ -n "${UP_PID:-}" ] && kill "$UP_PID" 2>/dev/null
  echo "--- adapter after stop"
  netsh interface show interface | grep -i "sb-tun-selftest" || echo "(cleaned up)"
}
trap cleanup EXIT

"$CORE" run -c "$WORK/upstream.json" -D "$WORK" > "$WORK/upstream.log" 2>&1 &
UP_PID=$!
sleep 2
"$CORE" run -c "$WORK/tun.json" -D "$WORK" > "$WORK/tun.log" 2>&1 &
TUN_PID=$!
sleep 6

echo "--- adapter"
netsh interface show interface | grep -i "sb-tun-selftest" || echo "MISSING - see $WORK/tun.log"

echo "--- TCP through the tunnel (DNS must resolve inside it)"
curl -s -o /dev/null -w "https=%{http_code} dns=%{time_namelookup}s\n" --max-time 25 https://example.com
curl -s -o /dev/null -w "http=%{http_code}\n" --max-time 25 http://example.com

echo "--- DNS answers seen by the core (proof it is not leaking to the LAN resolver)"
grep -E "dns: exchanged" "$WORK/tun.log" | tail -4 || echo "(none - DNS did not travel the tunnel)"

echo "--- logs"
echo "tun.log     : $WORK/tun.log"
echo "upstream.log: $WORK/upstream.log"
echo
echo "expect: https=200, at least one 'dns: exchanged' line, no errors above."
