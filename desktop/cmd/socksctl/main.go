// Command socksctl adalah klien SOCKS5 untuk Linux dan macOS: jalur TUN yang
// sama dengan aplikasi Windows (config dari internal/boxcfg, core sing-box dari
// release `core`), tanpa GUI native.
//
//	sudo socksctl up  -host 10.0.0.1 -port 1080     # jalan di depan (foreground)
//	sudo socksctl gui -host 10.0.0.1 -port 1080     # UI di browser, 127.0.0.1
//	     socksctl config -host 10.0.0.1 -port 1080  # cetak JSON saja
//	     socksctl check  -host 10.0.0.1 -port 1080  # validasi via sing-box check
//
// Hanya stdlib: satu binary kecil, core dipanggil sebagai proses terpisah.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"socks-client-desktop/internal/boxcfg"
)

const appVersion = "1.5.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "up":
		runUp(os.Args[2:])
	case "gui":
		runGUI(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "check":
		runCheck(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("socksctl %s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "subcommand tidak dikenal: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`socksctl - klien SOCKS5 (TUN) untuk Linux dan macOS

  sudo socksctl up   -host IP -port PORT [-user U] [-pass P] [opsi]
  sudo socksctl gui  -host IP -port PORT [-listen 127.0.0.1:17800] [opsi]
       socksctl config -host IP -port PORT [opsi]     cetak config sing-box
       socksctl check  -host IP -port PORT [opsi]     validasi config ke core
       socksctl version

Opsi TUN:
  -mtu 1400        MTU interface (576-9000, default 1400)
  -stack gvisor    gvisor | mixed | system (default gvisor)
  -iface NAME      nama interface (default: sb-tun; macOS selalu otomatis)
  -core PATH       path binary sing-box (default: cari otomatis, atau $SINGBOX_BIN)
  -data DIR        direktori kerja: config core + log (default /tmp/socksctl)

Host wajib berupa IP: hostname akan di-resolve di luar tunnel.
Server SOCKS yang dipakai harus menerima TCP (dan UDP associate untuk UDP).
`)
}

type options struct {
	host, user, pass string
	port             int
	mtu              int
	stack            string
	iface            string
	core             string
	data             string
	listen           string
}

func parseOptions(name string, args []string) options {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	var o options
	fs.StringVar(&o.host, "host", "", "alamat IP server SOCKS5")
	fs.IntVar(&o.port, "port", 1080, "port server SOCKS5")
	fs.StringVar(&o.user, "user", os.Getenv("SOCKS_USER"), "username (opsional)")
	fs.StringVar(&o.pass, "pass", os.Getenv("SOCKS_PASS"), "password (opsional)")
	fs.IntVar(&o.mtu, "mtu", boxcfg.DefaultTunMTU, "MTU interface")
	fs.StringVar(&o.stack, "stack", boxcfg.StackGVisor, "gvisor | mixed | system")
	fs.StringVar(&o.iface, "iface", boxcfg.DefaultInterfaceName(runtime.GOOS), "nama interface TUN")
	fs.StringVar(&o.core, "core", os.Getenv("SINGBOX_BIN"), "path binary sing-box")
	fs.StringVar(&o.data, "data", filepath.Join(os.TempDir(), "socksctl"), "direktori kerja")
	fs.StringVar(&o.listen, "listen", "127.0.0.1:17800", "alamat UI (subcommand gui)")
	fs.Parse(args)

	if o.host == "" {
		fatal("wajib: -host (alamat IP server SOCKS5)")
	}
	if net.ParseIP(o.host) == nil {
		fatal("host harus berupa IP literal (hostname akan di-resolve di luar tunnel)")
	}
	if o.port < 1 || o.port > 65535 {
		fatal("port tidak valid: %d", o.port)
	}
	return o
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "socksctl: "+format+"\n", args...)
	os.Exit(1)
}

// boxConfig membangun config sing-box dari satu tempat: internal/boxcfg, sama
// persis dengan yang dipakai aplikasi Windows.
func (o options) boxConfig() map[string]interface{} {
	opts := boxcfg.TunOptions{Stack: o.stack, MTU: o.mtu, LogLevel: "info"}
	if runtime.GOOS == "darwin" || o.iface == "" {
		// macOS hanya mengizinkan utunN, jadi biarkan core memilih namanya.
		opts.AutoInterfaceName = true
	} else {
		opts.InterfaceName = o.iface
	}
	return boxcfg.Tun(o.host, o.port, o.user, o.pass, opts)
}

func (o options) configJSON() []byte {
	data, err := json.MarshalIndent(o.boxConfig(), "", "  ")
	if err != nil {
		fatal("gagal menyusun config: %v", err)
	}
	return append(data, '\n')
}

// findCore: flag/env dulu, lalu di sebelah binary, lalu lokasi paket Linux,
// terakhir dari PATH.
func findCore(hint string) (string, error) {
	if hint != "" {
		if st, err := os.Stat(hint); err == nil && st.Size() > 1024 {
			return hint, nil
		}
		return "", fmt.Errorf("core tidak ditemukan di %s", hint)
	}
	self, _ := os.Executable()
	candidates := []string{
		filepath.Join(filepath.Dir(self), "sing-box"),
		"/usr/lib/socksclient/sing-box",
		"/usr/local/lib/socksclient/sing-box",
		filepath.Join(filepath.Dir(self), "sing-box.exe"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.Size() > 1024 {
			return c, nil
		}
	}
	if found, err := exec.LookPath("sing-box"); err == nil {
		return found, nil
	}
	return "", errors.New("binary sing-box tidak ditemukan - pakai -core PATH atau set SINGBOX_BIN")
}

// writeConfig menulis config ke direktori kerja dengan mode 0600. File ini memuat
// kredensial SOCKS, jadi tidak boleh terbaca user lain dan dihapus saat keluar.
func writeConfig(o options, data []byte) (string, error) {
	if err := os.MkdirAll(o.data, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(o.data, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func runConfig(args []string) {
	o := parseOptions("config", args)
	os.Stdout.Write(o.configJSON())
}

func runCheck(args []string) {
	o := parseOptions("check", args)
	core, err := findCore(o.core)
	if err != nil {
		fatal("%v", err)
	}
	path, err := writeConfig(o, o.configJSON())
	if err != nil {
		fatal("gagal menulis config: %v", err)
	}
	defer os.Remove(path)
	out, err := exec.Command(core, "check", "-c", path).CombinedOutput()
	if len(out) > 0 {
		os.Stdout.Write(out)
	}
	if err != nil {
		fatal("core menolak config: %v", err)
	}
	fmt.Println("config diterima core - TUN siap dijalankan")
}

func runUp(args []string) {
	o := parseOptions("up", args)
	if !isRoot() {
		fatal("TUN butuh root - jalankan: sudo socksctl up -host %s -port %d", o.host, o.port)
	}
	core, err := findCore(o.core)
	if err != nil {
		fatal("%v", err)
	}
	path, err := writeConfig(o, o.configJSON())
	if err != nil {
		fatal("gagal menulis config: %v", err)
	}
	defer os.Remove(path)

	fmt.Printf("socksctl %s - tunnel ke %s:%d\n", appVersion, o.host, o.port)
	fmt.Printf("core   : %s\n", core)
	fmt.Printf("tun    : stack %s, mtu %d%s\n", o.stack, o.mtu, interfaceNote(o))
	fmt.Printf("config : %s (dihapus saat keluar)\n\n", path)

	code := runCore(core, path, o)
	os.Remove(path)
	os.Exit(code)
}

func interfaceNote(o options) string {
	if runtime.GOOS == "darwin" || o.iface == "" {
		return ", interface otomatis (utunN)"
	}
	return ", interface " + o.iface
}

// runCore menjalankan core di depan proses ini dan meneruskan sinyal berhenti.
// Keluarannya dibiarkan apa adanya supaya log sing-box terlihat seperti di app
// Windows (Diagnosa).
func runCore(core, configPath string, o options) int {
	cmd := exec.Command(core, "run", "-c", configPath, "-D", o.data)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		fatal("gagal menjalankan core: %v", err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case sig := <-signals:
		fmt.Printf("\nmenerima %s - menghentikan tunnel...\n", sig)
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			<-done
		}
		return 0
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "core berhenti: %v\n", err)
			return 1
		}
		return 0
	}
}

// dipakai gui.go
func trim(s string) string { return strings.TrimSpace(s) }
