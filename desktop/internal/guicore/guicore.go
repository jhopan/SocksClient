// Package guicore memuat logika GUI yang dipakai bersama oleh shell GTK3 (Linux)
// dan AppKit (macOS): baca form, start/stop tunnel, simpan pengaturan, dan
// menyalakan HTTP ping. Bebas cgo, jadi bisa diuji tanpa GUI.
package guicore

import (
	"strconv"
	"strings"
	"sync"

	"socks-client-desktop/internal/engine"
	"socks-client-desktop/internal/settings"
)

// UI adalah jembatan ke widget: implementasinya tipis di tiap platform
// (setter GTK / AppKit + pembaca field).
type UI interface {
	FieldHost() string
	FieldPort() string
	FieldUser() string
	FieldPass() string
	PingChecked() bool

	SetStatus(string)
	SetLog(string)
	SetRunning(bool)
	ShowError(string)

	SetFields(host, port, user, pass string)
	SetPingChecked(bool)
}

// State menyimpan engine + pengaturan yang sedang dipakai.
type State struct {
	ui     UI
	engine *engine.Engine

	mu       sync.Mutex
	settings settings.Settings
}

// Attach mengikat UI dan engine; dipanggil sekali saat start.
func Attach(ui UI, eng *engine.Engine, s settings.Settings) *State {
	st := &State{ui: ui, engine: eng, settings: s}
	ui.SetFields(s.Host, strconv.Itoa(s.Port), s.User, Password(s))
	ui.SetPingChecked(s.Ping)
	ui.SetStatus("Disconnected")
	ui.SetLog("(belum ada keluaran core - tekan Connect)")
	return st
}

// Password mengambil password dari Keychain kalau ada (macOS), kalau tidak dari
// file pengaturan.
func Password(s settings.Settings) string {
	if pass, err := settings.LoadSecret(); err == nil && pass != "" {
		return pass
	}
	return s.Pass
}

// SaveSettings menulis pengaturan; password hanya ditulis ke file kalau tidak ada
// penyimpanan rahasia OS (lihat settings.KeychainAvailable).
func SaveSettings(s settings.Settings) {
	if settings.KeychainAvailable() {
		settings.StoreSecret(s.Pass)
		copy := s
		copy.Pass = "" // jangan simpan di file kalau ada Keychain
		settings.Save(copy)
		return
	}
	settings.Save(s)
}

// Toggle menghubungkan/memutuskan tunnel. Dipanggil dari callback tombol, jadi
// sudah di main thread GUI.
func (st *State) Toggle() {
	host := strings.TrimSpace(st.ui.FieldHost())
	if host == "" {
		st.ui.ShowError("Host/IP wajib diisi.")
		return
	}
	port := 1080
	if text := strings.TrimSpace(st.ui.FieldPort()); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > 65535 {
			st.ui.ShowError("Port tidak valid: " + text)
			return
		}
		port = n
	}

	if st.engine.Running() {
		st.engine.Stop()
		st.ui.SetRunning(false)
		st.ui.SetStatus(st.engine.Status())
		return
	}

	opts := engine.Options{
		Host: host, Port: port,
		User: strings.TrimSpace(st.ui.FieldUser()),
		Pass: st.ui.FieldPass(),
	}
	if err := st.engine.Start(opts); err != nil {
		st.ui.ShowError(err.Error())
		st.ui.SetStatus("Gagal: " + err.Error())
		return
	}
	st.engine.SetPing(st.ui.PingChecked())

	st.mu.Lock()
	st.settings.Host, st.settings.Port = host, port
	st.settings.User, st.settings.Pass = opts.User, opts.Pass
	st.settings.Ping = st.ui.PingChecked()
	snapshot := st.settings
	st.mu.Unlock()
	SaveSettings(snapshot)

	st.ui.SetRunning(true)
	st.ui.SetStatus(st.engine.Status())
}

// PingToggled menyalakan/mematikan HTTP ping dan menyimpan pilihannya.
func (st *State) PingToggled(on bool) {
	st.engine.SetPing(on)
	st.mu.Lock()
	st.settings.Ping = on
	snapshot := st.settings
	st.mu.Unlock()
	SaveSettings(snapshot)
}

// Tick dipanggil timer GUI (1 detik): menyegarkan status, tombol, dan log.
func (st *State) Tick() {
	st.ui.SetStatus(st.engine.Status())
	st.ui.SetRunning(st.engine.Running())
	st.ui.SetLog(st.logText())
}

func (st *State) logText() string {
	text := st.engine.LogTail(120)
	if strings.TrimSpace(text) == "" {
		return "(belum ada keluaran core - tekan Connect)"
	}
	return text
}

// StatusLine berguna untuk test: satu baris yang sama dengan label GUI.
func (st *State) StatusLine() string { return st.engine.Status() }
