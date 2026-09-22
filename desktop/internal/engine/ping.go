package engine

import (
	"context"

	"socks-client-desktop/internal/ping"
)

// SetPing menyalakan/mematikan HTTP ping. Ping dijalankan di proses GUI, yang
// trafiknya ikut masuk TUN (auto_route), jadi hasilnya benar-benar menguji jalur
// GUI -> TUN -> SOCKS -> internet. Aman dipanggil dari callback GUI (main thread)
// karena hanya mengubah state + memulai goroutine.
func (e *Engine) SetPing(on bool) {
	e.mu.Lock()
	e.pingOn = on
	e.mu.Unlock()
	e.syncPing()
}

// PingEnabled melaporkan keadaan tombol ping.
func (e *Engine) PingEnabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pingOn
}

// syncPing memastikan loop ping berjalan hanya saat tunnel hidup dan tombolnya
// menyala.
func (e *Engine) syncPing() {
	e.mu.Lock()
	shouldRun := e.pingOn && e.running
	if !shouldRun {
		cancel := e.pingCancel
		e.pingCancel = nil
		e.lastPing = ""
		e.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	if e.pingCancel != nil {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.pingCancel = cancel
	e.mu.Unlock()

	client := ping.NewClient()
	go ping.Watch(ctx, client, ping.Targets, ping.DefaultInterval, ping.RetryInterval, func(r ping.Result) {
		e.mu.Lock()
		e.lastPing = r.Summary()
		e.mu.Unlock()
	})
}

func (e *Engine) stopPing() {
	e.mu.Lock()
	cancel := e.pingCancel
	e.pingCancel = nil
	e.lastPing = ""
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
