package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartRejectsInvalidPort(t *testing.T) {
	e := New("tidak-ada", t.TempDir())
	if err := e.Start(Options{Host: "1.2.3.4", Port: 0}); err == nil {
		t.Fatal("port 0 seharusnya ditolak")
	}
	if e.Running() {
		t.Fatal("tidak boleh berstatus jalan setelah gagal")
	}
}

// Core yang tidak ada harus menghasilkan error, TANPA meninggalkan config.json
// (isinya kredensial).
func TestStartMissingCoreLeavesNoConfig(t *testing.T) {
	dir := t.TempDir()
	e := New(filepath.Join(dir, "core-tidak-ada"), dir)
	err := e.Start(Options{Host: "1.2.3.4", Port: 1080})
	if err == nil {
		t.Fatal("core yang tidak ada seharusnya error")
	}
	if !strings.Contains(err.Error(), "gagal menjalankan core") {
		t.Fatalf("pesan error tidak jelas: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "config.json")); statErr == nil {
		t.Fatal("config.json tertinggal setelah gagal start")
	}
}

func TestStatusAndLogTail(t *testing.T) {
	e := New("core", t.TempDir())
	if got := e.Status(); got != "Disconnected" {
		t.Fatalf("status awal = %q", got)
	}
	for _, line := range []string{"baris-1", "baris-2", "baris-3"} {
		e.appendLog(line)
	}
	if got := e.LogTail(2); got != "baris-2\nbaris-3" {
		t.Fatalf("LogTail(2) = %q", got)
	}
	if got := e.LogTail(0); !strings.HasPrefix(got, "baris-1") {
		t.Fatalf("LogTail(0) = %q", got)
	}
}

// Ping hanya berjalan saat tunnel hidup; mematikannya harus membersihkan hasil.
func TestSetPingOnlyWhenRunning(t *testing.T) {
	e := New("core", t.TempDir())
	e.SetPing(true)
	if !e.PingEnabled() {
		t.Fatal("tombol ping tidak tercatat")
	}
	e.mu.Lock()
	active := e.pingCancel
	e.mu.Unlock()
	if active != nil {
		t.Fatal("loop ping tidak boleh jalan kalau tunnel belum hidup")
	}
	e.SetPing(false)
	if e.PingEnabled() {
		t.Fatal("ping tidak ikut mati")
	}
}
