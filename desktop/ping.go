package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTP ping: membuktikan jalur lengkap aplikasi -> TUN -> SOCKS -> internet.
// Trafik proses aplikasi sendiri tetap masuk TUN karena auto_route, jadi
// permintaan ini benar-benar melewati SOCKS server dan bukan internet langsung.
// Sudah diuji di mesin Windows dengan strict_route aktif: 204 dalam 0,28-0,39 s.
//
// Ini sengaja dibuat seringan mungkin dan sekaligus berguna dua hal:
//  1. keep-alive - koneksi ke server SOCKS tidak pernah benar-benar idle, jadi
//     NAT/hotspot yang memutus koneksi diam tidak mematikan jalur tunnel;
//  2. pengecek internet murah - hasil 204/latensi langsung terlihat di status.
//
// Caranya: HTTP (bukan HTTPS, jadi tidak ada TLS), 204 tanpa body, header
// seminimal mungkin, koneksi di-keep alive (permintaan berikutnya cukup ~2 paket),
// dan jeda 30 detik saat sehat. Kalau gagal, percobaan berikutnya 10 detik supaya
// pulihnya jaringan cepat kelihatan. Kira-kira 1-2 MB per hari kalau dibiarkan On.
const defaultPingInterval = 30 * time.Second

// Setelah gagal, cek lagi lebih cepat - ini yang membuatnya berguna sebagai
// pengecek internet: begitu jaringan pulih, status ikut pulih dalam detik.
const pingRetryInterval = 10 * time.Second

const pingTimeout = 4 * time.Second

// Dua target supaya satu endpoint yang diblokir jaringan tidak membuat ping
// selalu gagal. Keduanya mengembalikan 204 tanpa isi.
var pingTargets = []string{
	"http://connectivitycheck.gstatic.com/generate_204",
	"http://cp.cloudflare.com/generate_204",
}

// PingResult adalah hasil satu percobaan.
type PingResult struct {
	OK      bool
	Code    int
	Latency time.Duration
	Err     string
	At      time.Time
}

// summary dipakai UI: "ping 204 38ms" atau "ping gagal (timeout)".
func (p PingResult) summary() string {
	if !p.At.IsZero() && p.OK {
		return fmt.Sprintf("ping %d %dms", p.Code, p.Latency.Milliseconds())
	}
	if p.Err == "" {
		return "ping -"
	}
	return "ping gagal (" + p.Err + ")"
}

// pingOnce mencoba tiap target berurutan sampai ada yang menjawab 204.
func pingOnce(client *http.Client, targets []string) PingResult {
	var lastErr error
	for _, target := range targets {
		start := time.Now()
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			lastErr = err
			continue
		}
		// Header seminimal mungkin: User-Agent kosong (Go menghilangkannya) dan
		// tanpa Accept-Encoding. Dua-duanya memangkas byte per permintaan.
		req.Header.Set("User-Agent", "")
		req.Header.Set("Accept", "*/*")
		req.Header["Accept-Encoding"] = nil
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		// Buang body kecil ini; target 204 memang tidak punya isi.
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		latency := time.Since(start)
		if resp.StatusCode == http.StatusNoContent {
			return PingResult{OK: true, Code: resp.StatusCode, Latency: latency, At: time.Now()}
		}
		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = errors.New("tidak ada target")
	}
	return PingResult{Err: shortError(lastErr), At: time.Now()}
}

func shortError(err error) string {
	msg := err.Error()
	if len(msg) > 60 {
		msg = msg[:60]
	}
	return msg
}

// watchPing mengirim hasil ke onResult sampai ctx dibatalkan. Cek pertama
// langsung dilakukan supaya status tidak menunggu satu interval, dan setelah
// hasil gagal jedanya dipendekkan (retryInterval) supaya pemulihan jaringan
// cepat terlihat.
func watchPing(ctx context.Context, client *http.Client, targets []string, interval, retryInterval time.Duration, onResult func(PingResult)) {
	for {
		result := pingOnce(client, targets)
		onResult(result)

		wait := interval
		if !result.OK {
			wait = retryInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// newPingClient: satu client dipakai terus supaya koneksi di-keep alive dan
// biaya per ping tetap kecil (kira-kira dua paket kecil).
func newPingClient() *http.Client {
	return &http.Client{
		Timeout: pingTimeout,
		Transport: &http.Transport{
			// Keep-alive eksplisit: IdleConnTimeout harus jauh lebih besar dari
			// jeda ping supaya koneksi yang sudah dibangun benar-benar dipakai
			// ulang (permintaan berikutnya tanpa handshake TCP lagi).
			DisableKeepAlives:   false,
			DisableCompression:  true,
			MaxIdleConns:        2,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     10 * time.Minute,
		},
	}
}
