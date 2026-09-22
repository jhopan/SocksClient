package guicore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"socks-client-desktop/internal/engine"
	"socks-client-desktop/internal/settings"
)

// fakeUI merekam apa yang diminta GUI lakukan, tanpa toolkit apa pun.
type fakeUI struct {
	host, port, user, pass string
	ping                   bool

	status  string
	log     string
	running bool
	err     string
}

func (f *fakeUI) FieldHost() string  { return f.host }
func (f *fakeUI) FieldPort() string  { return f.port }
func (f *fakeUI) FieldUser() string  { return f.user }
func (f *fakeUI) FieldPass() string  { return f.pass }
func (f *fakeUI) PingChecked() bool  { return f.ping }
func (f *fakeUI) SetStatus(s string) { f.status = s }
func (f *fakeUI) SetLog(s string)    { f.log = s }
func (f *fakeUI) SetRunning(r bool)  { f.running = r }
func (f *fakeUI) ShowError(s string) { f.err = s }
func (f *fakeUI) SetFields(host, port, user, pass string) {
	f.host, f.port, f.user, f.pass = host, port, user, pass
}
func (f *fakeUI) SetPingChecked(on bool) { f.ping = on }

func newState(t *testing.T, core string) (*State, *fakeUI) {
	t.Helper()
	ui := &fakeUI{}
	eng := engine.New(core, t.TempDir())
	st := Attach(ui, eng, settings.Settings{Port: 1080})
	return st, ui
}

func TestAttachFillsFields(t *testing.T) {
	ui := &fakeUI{}
	eng := engine.New("core", t.TempDir())
	Attach(ui, eng, settings.Settings{Host: "10.0.0.1", Port: 1080, User: "u", Pass: "p", Ping: true})
	if ui.host != "10.0.0.1" || ui.port != "1080" || ui.user != "u" || ui.pass != "p" {
		t.Fatalf("field tidak terisi: %+v", ui)
	}
	if !ui.ping {
		t.Fatal("pilihan ping tidak dipulihkan")
	}
	if ui.status != "Disconnected" {
		t.Fatalf("status awal = %q", ui.status)
	}
}

func TestToggleRejectsEmptyHost(t *testing.T) {
	st, ui := newState(t, "core-tidak-ada")
	st.Toggle()
	if ui.err == "" || !strings.Contains(ui.err, "Host/IP") {
		t.Fatalf("tidak ada peringatan host kosong: %q", ui.err)
	}
}

func TestToggleRejectsBadPort(t *testing.T) {
	st, ui := newState(t, "core-tidak-ada")
	ui.host = "1.2.3.4"
	ui.port = "abc"
	st.Toggle()
	if !strings.Contains(ui.err, "Port tidak valid") {
		t.Fatalf("peringatan port tidak muncul: %q", ui.err)
	}
}

func TestToggleReportsCoreFailure(t *testing.T) {
	st, ui := newState(t, filepath.Join(t.TempDir(), "core-hilang"))
	ui.host, ui.port = "1.2.3.4", "1080"
	st.Toggle()
	if !strings.Contains(ui.err, "gagal menjalankan core") {
		t.Fatalf("error core tidak ditampilkan: %q", ui.err)
	}
	if !strings.HasPrefix(ui.status, "Gagal:") {
		t.Fatalf("status setelah gagal = %q", ui.status)
	}
	if ui.running {
		t.Fatal("tombol tidak boleh berubah jadi Disconnect setelah gagal")
	}
	// Config berisi kredensial tidak boleh tertinggal dari percobaan yang gagal.
	if _, err := os.Stat(filepath.Join(st.engine.DataDir(), "config.json")); err == nil {
		t.Fatal("config.json tertinggal")
	}
}

func TestTickRefreshesStatusAndLog(t *testing.T) {
	st, ui := newState(t, "core")
	ui.status, ui.log = "", ""
	st.Tick()
	if ui.status != "Disconnected" {
		t.Fatalf("status = %q", ui.status)
	}
	if !strings.Contains(ui.log, "belum ada keluaran") {
		t.Fatalf("log placeholder tidak muncul: %q", ui.log)
	}
	if ui.running {
		t.Fatal("Running() harus false saat idle")
	}
}
