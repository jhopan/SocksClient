package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// Windows helpers for the non-TUN mode.
//
// Mode "proxy" needs neither a TUN interface nor Administrator rights: sing-box
// listens on a local mixed (SOCKS5 + HTTP) inbound and we point WinINet at it.
// That is the fallback for machines where wintun or the route table fights back.

const inetKeyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// ProxyBackup holds the WinINet proxy values we overwrite so that disconnect
// (or the next launch after a crash) can put them back untouched.
type ProxyBackup struct {
	ProxyEnable   uint32 `json:"proxy_enable"`
	ProxyServer   string `json:"proxy_server"`
	ProxyOverride string `json:"proxy_override"`
	AutoConfigURL string `json:"auto_config_url"`
	Valid         bool   `json:"valid"`
}

func openInetKey() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, inetKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
}

func readSystemProxy() (ProxyBackup, error) {
	k, err := openInetKey()
	if err != nil {
		return ProxyBackup{}, err
	}
	defer k.Close()

	var b ProxyBackup
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		b.ProxyEnable = uint32(v)
	}
	b.ProxyServer, _, _ = k.GetStringValue("ProxyServer")
	b.ProxyOverride, _, _ = k.GetStringValue("ProxyOverride")
	b.AutoConfigURL, _, _ = k.GetStringValue("AutoConfigURL")
	b.Valid = true
	return b, nil
}

func writeSystemProxy(b ProxyBackup) error {
	k, err := openInetKey()
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetDWordValue("ProxyEnable", b.ProxyEnable); err != nil {
		return err
	}
	set := func(name, value string) error {
		if value == "" {
			k.DeleteValue(name)
			return nil
		}
		return k.SetStringValue(name, value)
	}
	if err := set("ProxyServer", b.ProxyServer); err != nil {
		return err
	}
	if err := set("ProxyOverride", b.ProxyOverride); err != nil {
		return err
	}
	if err := set("AutoConfigURL", b.AutoConfigURL); err != nil {
		return err
	}
	notifyProxyChanged()
	return nil
}

// applySystemProxy points WinINet (Edge/Chrome/WinHTTP apps) at our local mixed
// inbound and returns the previous values so the caller can restore them.
func applySystemProxy(addr string) (ProxyBackup, error) {
	prev, err := readSystemProxy()
	if err != nil {
		return ProxyBackup{}, err
	}

	next := prev
	next.ProxyEnable = 1
	next.ProxyServer = addr
	next.AutoConfigURL = "" // a PAC script outranks ProxyServer, drop it while connected
	if next.ProxyOverride == "" {
		next.ProxyOverride = "<local>"
	}
	if err := writeSystemProxy(next); err != nil {
		return ProxyBackup{}, err
	}
	return prev, nil
}

// notifyProxyChanged tells running WinINet apps to re-read the settings.
func notifyProxyChanged() {
	p := syscall.NewLazyDLL("wininet.dll").NewProc("InternetSetOptionW")
	p.Call(0, 39, 0, 0) // INTERNET_OPTION_SETTINGS_CHANGED
	p.Call(0, 37, 0, 0) // INTERNET_OPTION_REFRESH
}

func isAdmin() bool {
	ok, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	return ok != 0
}

// ---------------------------------------------------------------- env vars

const envKeyPath = `Environment`

// EnvBackup keeps the user-level proxy environment variables we overwrite.
// Go, Python, Node, curl and git read these instead of the WinINet settings.
type EnvBackup struct {
	HTTPProxy  string `json:"http_proxy"`
	HTTPSProxy string `json:"https_proxy"`
	ALLProxy   string `json:"all_proxy"`
	NOProxy    string `json:"no_proxy"`
	Valid      bool   `json:"valid"`
}

func openEnvKey() (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, envKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
}

func readEnvProxy() (EnvBackup, error) {
	k, err := openEnvKey()
	if err != nil {
		return EnvBackup{}, err
	}
	defer k.Close()

	var b EnvBackup
	b.HTTPProxy, _, _ = k.GetStringValue("HTTP_PROXY")
	b.HTTPSProxy, _, _ = k.GetStringValue("HTTPS_PROXY")
	b.ALLProxy, _, _ = k.GetStringValue("ALL_PROXY")
	b.NOProxy, _, _ = k.GetStringValue("NO_PROXY")
	b.Valid = true
	return b, nil
}

func writeEnvProxy(b EnvBackup) error {
	k, err := openEnvKey()
	if err != nil {
		return err
	}
	defer k.Close()

	setPair := func(name, value string) error {
		if value == "" {
			k.DeleteValue(name)
			return nil
		}
		return k.SetStringValue(name, value)
	}
	names := []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"}
	values := []string{b.HTTPProxy, b.HTTPSProxy, b.ALLProxy, b.NOProxy}
	for i, name := range names {
		if err := setPair(name, values[i]); err != nil {
			return err
		}
	}
	notifyEnvChanged()
	return nil
}

// applyEnvProxy points CLI tools (Go, Python, Node, curl, git) at the local
// mixed inbound. Returns the previous values for restore.
func applyEnvProxy(addr string) (EnvBackup, error) {
	prev, err := readEnvProxy()
	if err != nil {
		return EnvBackup{}, err
	}
	next := EnvBackup{
		HTTPProxy:  "http://" + addr,
		HTTPSProxy: "http://" + addr,
		ALLProxy:   "socks5://" + addr,
		NOProxy:    "localhost,127.0.0.1,::1",
		Valid:      true,
	}
	if err := writeEnvProxy(next); err != nil {
		return EnvBackup{}, err
	}
	return prev, nil
}

// notifyEnvChanged tells newly started processes to pick the variables up.
func notifyEnvChanged() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	user32 := syscall.NewLazyDLL("user32.dll")
	p := user32.NewProc("SendMessageTimeoutW")
	p.Call(0xffff, 0x001A, 0, uintptr(unsafe.Pointer(env)), 0x2, 5000, 0) // HWND_BROADCAST, WM_SETTINGCHANGE, SMTO_ABORTIFHUNG
}

// ---------------------------------------------------------------- WinHTTP

// WinHTTPBackup is the previous `netsh winhttp show proxy` state.
type WinHTTPBackup struct {
	Direct bool   `json:"direct"`
	Proxy  string `json:"proxy"`
	Bypass string `json:"bypass"`
	Valid  bool   `json:"valid"`
}

func runNetsh(args ...string) (string, error) {
	cmd := exec.Command("netsh.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readWinHTTPProxy() WinHTTPBackup {
	out, err := runNetsh("winhttp", "show", "proxy")
	if err != nil {
		return WinHTTPBackup{}
	}
	b := WinHTTPBackup{Valid: true}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "direct access") || strings.Contains(lower, "no proxy server") {
		b.Direct = true
		return b
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Proxy Server(s)"):
			b.Proxy = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(trimmed, ":", 2)[1], " "))
		case strings.HasPrefix(trimmed, "Bypass List"):
			b.Bypass = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(trimmed, ":", 2)[1], " "))
		}
	}
	if b.Proxy == "" {
		b.Valid = false
	}
	return b
}

func applyWinHTTPProxy(addr string) (WinHTTPBackup, error) {
	prev := readWinHTTPProxy()
	if _, err := runNetsh("winhttp", "set", "proxy", addr, "<local>"); err != nil {
		return prev, err
	}
	return prev, nil
}

func restoreWinHTTPProxy(b WinHTTPBackup) {
	if !b.Valid {
		return
	}
	if b.Direct {
		runNetsh("winhttp", "reset", "proxy")
		return
	}
	args := []string{"winhttp", "set", "proxy", b.Proxy}
	if b.Bypass != "" {
		args = append(args, b.Bypass)
	}
	runNetsh(args...)
}

// relaunchAsAdmin starts a second, elevated copy of the app (UAC prompt) and
// lets the caller exit. Needed only for mode "tun" without elevation.
func relaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString("--elevated")
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))

	ret, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(dir)),
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW gagal (kode %d)", ret)
	}
	return nil
}
