//go:build windows

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Password harus kembali utuh setelah Save/Load, tapi tidak boleh ada sebagai
// teks di settings.json (di Windows lewat DPAPI, di macOS lewat Keychain).
func TestSaveLoadKeepsPasswordOutOfJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)

	in := Settings{Host: "127.0.0.1", Port: 1080, User: "jhopan", Pass: "rahasia123", Ping: true}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "SocksClientDesktop", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json tidak ada: %v", err)
	}
	if strings.Contains(string(raw), "rahasia123") {
		t.Fatalf("password bocor ke settings.json: %s", raw)
	}
	out := Load()
	if out.Pass != in.Pass {
		t.Fatalf("password tidak kembali: %q (harap %q)", out.Pass, in.Pass)
	}
	if out.Host != in.Host || out.Port != in.Port || out.User != in.User || !out.Ping {
		t.Fatalf("setelan lain berubah: %+v", out)
	}
}

// Blob DPAPI harus bolak-balik, dan blob yang dirusak harus dilaporkan gagal.
func TestDPAPIRoundTrip(t *testing.T) {
	enc := protect("apa saja")
	if enc == "" {
		t.Fatal("protect mengembalikan kosong")
	}
	plain, err := unprotect(enc)
	if err != nil || plain != "apa saja" {
		t.Fatalf("unprotect: %q %v", plain, err)
	}
	if _, err := unprotect("bukan-base64!!"); err == nil {
		t.Fatal("blob rusak seharusnya error")
	}
}

// Password kosong menghapus blob, bukan meninggalkan file kosong.
func TestEmptyPasswordRemovesBlob(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	if err := Save(Settings{Host: "1.1.1.1", Port: 1080, Pass: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	blob := filepath.Join(dir, "SocksClientDesktop", "pass.enc")
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("blob tidak dibuat: %v", err)
	}
	if err := Save(Settings{Host: "1.1.1.1", Port: 1080}); err != nil {
		t.Fatalf("Save kedua: %v", err)
	}
	if _, err := os.Stat(blob); !os.IsNotExist(err) {
		t.Fatalf("blob masih ada setelah password dikosongkan")
	}
	if got := Load().Pass; got != "" {
		t.Fatalf("masih ada password: %q", got)
	}
}

// Kalau file utama kehilangan alamat server, alamat harus diambil dari salinan
// cadangan (settings.json.bak). Cadangan berisi isi SEBELUM penyimpanan
// terakhir, jadi yang kembali adalah alamat versi sebelumnya - intinya alamat
// tidak hilang begitu saja.
func TestLoadFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	if err := Save(Settings{Host: "10.79.67.123", Port: 1080, User: "u", Ping: true}); err != nil {
		t.Fatalf("Save pertama: %v", err)
	}
	// Save kedua menulis cadangan berisi isi lama, lalu menimpa file utama.
	if err := Save(Settings{Host: "192.168.1.9", Port: 1081}); err != nil {
		t.Fatalf("Save kedua: %v", err)
	}
	bak := filepath.Join(dir, "SocksClientDesktop", "settings.json.bak")
	if _, err := os.Stat(bak); err != nil {
		t.Fatalf("cadangan tidak dibuat: %v", err)
	}
	// Sekarang rusak file utamanya seperti kejadian nyata: host dikosongkan.
	main := filepath.Join(dir, "SocksClientDesktop", "settings.json")
	if err := os.WriteFile(main, []byte(`{"host":"","port":1080}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load().Host; got != "10.79.67.123" {
		t.Fatalf("host tidak dipulihkan dari cadangan (harap versi sebelumnya): %q", got)
	}
}
