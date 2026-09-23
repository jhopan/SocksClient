package engine

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Timeout untuk tiap tahap cek pra-sambung.
const cekTimeout = 8 * time.Second

// Preflight memeriksa server sebelum core dijalankan:
//  1. host harus IP dan port masuk akal,
//  2. TCP ke host:port,
//  3. handshake SOCKS5 (pilih metode + login bila dipakai).
//
// step dipanggil di tiap tahap supaya GUI bisa menampilkan progres; error yang
// dikembalikan sudah berupa kalimat siap tampil.
func Preflight(host string, port int, user, pass string, step func(string)) error {
	if step == nil {
		step = func(string) {}
	}

	step("Cek host dan port...")
	if net.ParseIP(host) == nil {
		return errors.New("Host harus IP, contoh 10.0.0.1")
	}
	if port < 1 || port > 65535 {
		return errors.New("Port tidak valid")
	}

	alamat := net.JoinHostPort(host, strconv.Itoa(port))
	step("Cek koneksi ke " + alamat + "...")
	c, err := net.DialTimeout("tcp", alamat, cekTimeout)
	if err != nil {
		return fmt.Errorf("Server tidak bisa dihubungi (%s)", shortError(err))
	}
	defer c.Close()

	step("Cek autentikasi SOCKS5...")
	_ = c.SetDeadline(time.Now().Add(cekTimeout))

	greet := []byte{5, 1, 0}
	if user != "" {
		greet = []byte{5, 2, 0, 2}
	}
	if _, err := c.Write(greet); err != nil {
		return errors.New("Gagal bicara dengan server")
	}
	balas := make([]byte, 2)
	if _, err := io.ReadFull(c, balas); err != nil {
		return errors.New("Tidak ada balasan SOCKS5")
	}
	if balas[0] != 5 {
		return errors.New("Bukan server SOCKS5")
	}
	switch balas[1] {
	case 0: // tanpa autentikasi
	case 2:
		if user == "" {
			return errors.New("Server butuh login, isi Username dan Password")
		}
		auth := []byte{1, byte(len(user))}
		auth = append(auth, user...)
		auth = append(auth, byte(len(pass)))
		auth = append(auth, pass...)
		if _, err := c.Write(auth); err != nil {
			return errors.New("Gagal mengirim login")
		}
		hasil := make([]byte, 2)
		if _, err := io.ReadFull(c, hasil); err != nil {
			return errors.New("Tidak ada balasan login")
		}
		if hasil[1] != 0 {
			return errors.New("Login ditolak (username/password salah)")
		}
	default:
		return errors.New("Metode autentikasi server tidak didukung")
	}
	return nil
}

// shortError mengambil inti pesan error (tanpa prefix panjang) untuk status bar.
func shortError(err error) string {
	s := err.Error()
	if i := lastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func lastIndex(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
