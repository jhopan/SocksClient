package main

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// Which interface would Windows use to reach this address? Lazily asked, only
// when the watchdog needs it.
var pGetBestInterfaceEx = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetBestInterfaceEx")

// sockaddrIn mirrors SOCKADDR_IN for GetBestInterfaceEx (Go's SockaddrInet4 has
// no family field in memory, so build the struct by hand).
type sockaddrIn struct {
	Family uint16
	Port   uint16
	Addr   [4]byte
	Zero   [8]byte
}

// bestInterfaceIndex returns the interface index the OS picks for ip. A change
// of index means the machine moved networks (Wi-Fi handed over to Ethernet,
// hotspot reconnected, VPN adapter came up) - which is exactly when a running
// sing-box still points its outbound at the old interface.
func bestInterfaceIndex(ip net.IP) (uint32, error) {
	v4 := ip.To4()
	if v4 == nil {
		return 0, fmt.Errorf("hanya IPv4 yang didukung probe ini")
	}
	var sa sockaddrIn
	sa.Family = syscall.AF_INET
	copy(sa.Addr[:], v4)

	var index uint32
	ret, _, _ := pGetBestInterfaceEx.Call(
		uintptr(unsafe.Pointer(&sa)),
		uintptr(unsafe.Pointer(&index)),
	)
	if ret != 0 {
		return 0, fmt.Errorf("GetBestInterfaceEx gagal (kode %d)", ret)
	}
	return index, nil
}
