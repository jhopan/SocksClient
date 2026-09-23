//go:build (linux || darwin || windows) && cgo

// Socks Client - GUI Gio, satu kode untuk Windows, Linux dan macOS.
//
// Alur: splash (kredit jhopanstore + cara pakai) -> form -> Connect:
// cek host/port, cek koneksi TCP, cek autentikasi SOCKS5 (internal/engine),
// baru core sing-box dijalankan, lalu HTTP ping (internal/ping) tiap 30 detik.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"socks-client-desktop/internal/boxcfg"
	"socks-client-desktop/internal/engine"
	"socks-client-desktop/internal/settings"
	"socks-client-desktop/internal/winutil"
)

const (
	appVersion = "1.7.0"
	splashLama = 2500 * time.Millisecond
)

var (
	mu          sync.Mutex
	pesanStatus = "Belum tersambung"
	warnaStatus = abu
	sedangCek   bool
	en          *engine.Engine
)

func setPesan(s string, c color.NRGBA) {
	mu.Lock()
	pesanStatus, warnaStatus = s, c
	mu.Unlock()
}

func main() {
	coreFlag := flag.String("core", "", "path ke sing-box (opsional)")
	autoFlag := flag.Bool("auto", false, "langsung Connect setelah splash (untuk uji)")
	flag.Parse()

	// TUN butuh hak admin. Windows: minta lewat UAC lalu proses ini selesai.
	// Linux/macOS: root diperiksa dan pengguna diberi tahu (paket .deb memakai pkexec).
	if runtime.GOOS == "windows" && !winutil.IsAdmin() {
		if err := winutil.RelaunchElevated(); err == nil {
			return
		}
	}
	if !instanceTunggal() {
		os.Exit(0) // sudah ada instance yang berjalan
	}

	st := settings.Load()
	core, err := engine.FindCore(*coreFlag)
	if err != nil {
		core = ""
	}
	data, _ := settings.Dir()
	en = engine.New(core, data)

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		setPesan("TUN butuh root: jalankan lewat menu aplikasi (pkexec) atau sudo", merah)
	}
	if core == "" {
		setPesan("Core sing-box tidak ditemukan di samping aplikasi", merah)
	} else if _, err := os.Stat(core); err != nil {
		setPesan("Core sing-box tidak bisa dibaca: "+err.Error(), merah)
	}

	terbuka(st, *autoFlag)
}

// terbuka menyalakan jendela Gio.
func terbuka(st settings.Settings, auto bool) {
	edPort.SetText(fmt.Sprintf("%d", st.Port))
	if st.Host != "" {
		edHost.SetText(st.Host)
	}
	edUser.SetText(st.User)
	edPass.SetText(st.Pass)
	saklarPing.Value = st.Ping

	go func() {
		win = new(app.Window)
		win.Option(app.Title("Socks Client"), app.Size(unit.Dp(400), unit.Dp(430)))

		th := material.NewTheme()
		th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
		th.Palette = palet

		// Status ping dihitung engine; segarkan jendela tiap detik saat hidup.
		go func() {
			for range time.Tick(time.Second) {
				if en.Running() {
					win.Invalidate()
				}
			}
		}()

		selesai := time.Now().Add(splashLama)
		time.AfterFunc(splashLama, func() {
			win.Invalidate()
			if auto {
				time.Sleep(200 * time.Millisecond)
				sambung()
			}
		})

		for {
			e := win.Event()
			switch e := e.(type) {
			case app.DestroyEvent:
				en.Stop()
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				paint.Fill(gtx.Ops, th.Palette.Bg)
				if time.Now().Before(selesai) && !btnMulai.Clicked(gtx) {
					splash(gtx, th)
				} else {
					form(gtx, th)
				}
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

// aksiTombol: Disconnect bila tunnel hidup, kalau tidak cek lalu sambung.
func aksiTombol() {
	mu.Lock()
	cek := sedangCek
	mu.Unlock()
	if cek {
		return
	}
	if en.Running() {
		putus()
		return
	}
	sambung()
}

// sambung: preflight -> jalankan core -> nyalakan ping.
func sambung() {
	host, portStr := edHost.Text(), strings.TrimSpace(edPort.Text())
	user, pass := edUser.Text(), edPass.Text()

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		setPesan("Port tidak valid", merah)
		segarkan()
		return
	}
	if en == nil {
		return
	}

	mu.Lock()
	sedangCek = true
	mu.Unlock()
	simpanSetelan()
	defer func() {
		mu.Lock()
		sedangCek = false
		mu.Unlock()
		segarkan()
	}()

	tahap := func(s string) {
		setPesan(s, abu)
		segarkan()
	}
	if err := engine.Preflight(host, port, user, pass, tahap); err != nil {
		setPesan(err.Error(), merah)
		return
	}

	tahap("Menjalankan core (TUN " + boxcfg.DefaultInterfaceName(runtime.GOOS) + ")...")
	if err := en.Start(engine.Options{Host: host, Port: port, User: user, Pass: pass}); err != nil {
		setPesan(err.Error(), merah)
		return
	}
	en.SetPing(saklarPing.Value)
	setPesan("Tersambung "+host+":"+portStr, hijau)

	// Kalau core mati sendiri (jaringan berubah, dsb), tampilkan apa adanya.
	go func() {
		for range time.Tick(2 * time.Second) {
			if !en.Running() {
				setPesan(en.Status(), merah)
				segarkan()
				return
			}
		}
	}()
}

func putus() {
	en.Stop()
	en.SetPing(false)
	setPesan("Belum tersambung", abu)
	segarkan()
}

func terapkanPing() {
	en.SetPing(saklarPing.Value && en.Running())
}

func segarkan() {
	if win != nil {
		win.Invalidate()
	}
}

func simpanSetelan() {
	port := 1080
	if _, err := fmt.Sscanf(strings.TrimSpace(edPort.Text()), "%d", &port); err != nil {
		port = 1080
	}
	_ = settings.Save(settings.Settings{
		Host: edHost.Text(),
		Port: port,
		User: edUser.Text(),
		Pass: edPass.Text(),
		Ping: saklarPing.Value,
	})
}
