package main

import (
	"fmt"
	"os"
	"path/filepath"
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
