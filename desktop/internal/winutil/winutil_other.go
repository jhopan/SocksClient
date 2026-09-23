//go:build !windows

package winutil

// Di Linux/macOS TUN butuh root, bukan UAC: pemanggil memeriksa sendiri
// (os.Geteuid() == 0) dan mencetak baris sudo yang perlu dijalankan.
func IsAdmin() bool { return true }

// RelaunchElevated tidak berlaku di luar Windows.
func RelaunchElevated() error { return nil }
