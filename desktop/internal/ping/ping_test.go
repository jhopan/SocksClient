package ping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func noContent(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPingOnceReportsLatency(t *testing.T) {
	srv := noContent(t)
	res := Once(NewClient(), []string{srv.URL})
	if !res.OK {
		t.Fatalf("ping gagal: %+v", res)
	}
	if res.Code != http.StatusNoContent {
		t.Fatalf("kode = %d", res.Code)
	}
	if res.Latency <= 0 {
		t.Fatal("latensi tidak terukur")
	}
	if got := res.Summary(); !strings.HasPrefix(got, "ping 204 ") {
		t.Fatalf("summary = %q", got)
	}
}

// Satu endpoint yang diblokir tidak boleh membuat ping selalu gagal.
func TestPingOnceFallsBackToSecondTarget(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	good := noContent(t)

	res := Once(NewClient(), []string{bad.URL, good.URL})
	if !res.OK || res.Code != http.StatusNoContent {
		t.Fatalf("fallback tidak dipakai: %+v", res)
	}
}

func TestPingOnceReportsFailure(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	res := Once(NewClient(), []string{bad.URL})
	if res.OK {
		t.Fatal("status 500 dilaporkan sebagai sukses")
	}
	if !strings.Contains(res.Summary(), "ping gagal") {
		t.Fatalf("summary = %q", res.Summary())
	}

	// Port tertutup: harus jadi hasil gagal yang rapi, bukan panic.
	dead := Once(NewClient(), []string{"http://127.0.0.1:1/generate_204"})
	if dead.OK || dead.Err == "" {
		t.Fatalf("target mati tidak dilaporkan: %+v", dead)
	}
}

// Tombol Off harus benar-benar menghentikan loop, bukan sekadar menyembunyikan.
func TestWatchPingStopsOnCancel(t *testing.T) {
	srv := noContent(t)
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, NewClient(), []string{srv.URL}, 10*time.Millisecond, 10*time.Millisecond, func(Result) {
			atomic.AddInt32(&calls, 1)
		})
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Watch tidak berhenti setelah ctx dibatalkan")
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("hanya %d hasil ping", calls)
	}
}

// Pengecek internet murah: setelah gagal, percobaan berikutnya harus datang lebih
// cepat (retryInterval) daripada interval normal.
func TestWatchPingRetriesSoonerAfterFailure(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, NewClient(), []string{bad.URL},
		time.Hour,           // interval sehat: sengaja tidak mungkin tiba
		20*time.Millisecond, // retry setelah gagal
		func(Result) { atomic.AddInt32(&calls, 1) })

	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("hanya %d percobaan - retry cepat tidak jalan", got)
	}
}

// Endpoint yang dipakai harus yang disepakati: www.gstatic.com/generate_204.
func TestPingTargetsUseGstatic(t *testing.T) {
	if len(Targets) == 0 || Targets[0] != "http://www.gstatic.com/generate_204" {
		t.Fatalf("target utama berubah: %v", Targets)
	}
	for _, target := range Targets {
		if !strings.HasPrefix(target, "http://") {
			t.Fatalf("target bukan HTTP: %s", target)
		}
	}
}
