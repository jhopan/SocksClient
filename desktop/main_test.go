package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"socks-client-desktop/internal/boxcfg"
)

// Verify the config the app actually runs survives `sing-box check` unchanged.
// The app only ever emits stack gvisor with the default MTU, so that is what is
// asserted here.
func TestConfigsAcceptedBySingBox(t *testing.T) {
	bin := testSingBox(t)

	opts := boxcfg.TunOptions{Stack: boxcfg.StackGVisor}
	cases := map[string]map[string]interface{}{
		"tun":  boxcfg.Tun("10.12.132.225", 1080, "user", "pass", opts),
		"host": boxcfg.Tun("server.example.com", 1080, "user", "pass", opts),
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
