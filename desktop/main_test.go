package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Verify the config for both modes survives `sing-box check` unchanged.
func TestConfigsAcceptedBySingBox(t *testing.T) {
	bin := testSingBox(t)

	cases := map[string]map[string]interface{}{
		"tun":   buildTunConfig("10.12.132.225", 1080, "user", "pass"),
		"proxy": buildProxyConfig("10.12.132.225", 1080, "user", "pass", defaultLocalPort),
	}

	for name, cfg := range cases {
		path := filepath.Join(t.TempDir(), name+".json")
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}

		out, err := exec.Command(bin, "check", "-c", path).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: sing-box check failed: %v\n%s", name, err, out)
		}
	}
}

// The wininet values we touch must round-trip: apply -> restore == original.
func TestSystemProxyRoundTrip(t *testing.T) {
	before, err := readSystemProxy()
	if err != nil {
		t.Fatalf("readSystemProxy: %v", err)
	}

	const addr = "127.0.0.1:20808"
	backup, err := applySystemProxy(addr)
	if err != nil {
		t.Fatalf("applySystemProxy: %v", err)
	}
	defer writeSystemProxy(before)

	during, err := readSystemProxy()
	if err != nil {
		t.Fatalf("readSystemProxy (during): %v", err)
	}
	if during.ProxyEnable != 1 || during.ProxyServer != addr {
		t.Fatalf("system proxy not applied: enable=%d server=%q", during.ProxyEnable, during.ProxyServer)
	}
	if backup.Valid != before.Valid || backup.ProxyServer != before.ProxyServer || backup.ProxyEnable != before.ProxyEnable {
		t.Fatalf("backup mismatch: got %+v want %+v", backup, before)
	}

	if err := writeSystemProxy(backup); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, err := readSystemProxy()
	if err != nil {
		t.Fatalf("readSystemProxy (after): %v", err)
	}
	if after != before {
		t.Fatalf("restore mismatch:\n got %+v\nwant %+v", after, before)
	}
}

// End-to-end proof of the non-TUN path: local client -> mixed inbound (mode
// proxy) -> socks outbound -> fake SOCKS5 server (second sing-box) -> dst.
func TestProxyModeCarriesTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping traffic test in -short mode")
	}
	bin := testSingBox(t)
	dir := t.TempDir()

	// destination
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "socks-ok")
	}))
	defer dst.Close()

	socksPort := freePort(t)
	mixedPort := freePort(t)

	// fake upstream SOCKS5 server: sing-box socks inbound + direct outbound
	socksCfg := map[string]interface{}{
		"log":       map[string]interface{}{"level": "warn"},
		"inbounds":  []map[string]interface{}{{"type": "socks", "tag": "in", "listen": "127.0.0.1", "listen_port": socksPort}},
		"outbounds": []map[string]interface{}{{"type": "direct", "tag": "direct"}},
	}
	socksCmd := startSingBox(t, bin, dir, "socks-upstream.json", socksCfg)
	defer killCmd(socksCmd)
	waitPort(t, socksPort)

	proxyCmd := startSingBox(t, bin, dir, "proxy-mode.json",
		buildProxyConfig("127.0.0.1", socksPort, "", "", mixedPort))
	defer killCmd(proxyCmd)
	waitPort(t, mixedPort)

	proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(mixedPort))
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get(dst.URL)
	if err != nil {
		t.Fatalf("request through proxy mode failed: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 16)
	n, _ := resp.Body.Read(buf)
	if got := string(buf[:n]); got != "socks-ok" {
		t.Fatalf("unexpected body %q", got)
	}
}

// testSingBox returns the core to test against: SINGBOX_BIN, or the core
// downloaded by desktop/scripts/fetch-core.sh. CI always fetches it.
func testSingBox(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("SINGBOX_BIN"); bin != "" {
		return bin
	}
	bin := filepath.Join("embed", "sing-box.exe")
	if st, err := os.Stat(bin); err != nil || st.Size() < 1024 {
		t.Skipf("core not found at %s - run desktop/scripts/fetch-core.sh or set SINGBOX_BIN", bin)
	}
	return bin
}

func startSingBox(t *testing.T, bin, dir, name string, cfg map[string]interface{}) *exec.Cmd {
	t.Helper()
	path := filepath.Join(dir, name)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	cmd := exec.Command(bin, "run", "-c", path, "-D", dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sing-box %s: %v", name, err)
	}
	return cmd
}

func killCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	exec.Command("taskkill.exe", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	cmd.Wait()
}

func waitPort(t *testing.T, port int) {
	t.Helper()
	addr := "127.0.0.1:" + strconv.Itoa(port)
	for i := 0; i < 100; i++ {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d never opened", port)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
