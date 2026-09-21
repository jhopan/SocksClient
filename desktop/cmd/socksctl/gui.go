package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"socks-client-desktop/internal/boxcfg"
	"socks-client-desktop/internal/ping"
)

// UI berbasis browser: satu halaman di 127.0.0.1, tanpa dependensi baru dan tanpa
// toolkit GUI native (Windows memakai lxn/walk, yang tidak ada di Linux/macOS).
// Halaman ini memanggil core sing-box yang sama seperti `socksctl up`.

const guiLogLines = 200

type guiState struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	core     string
	dataDir  string
	running  bool
	host     string
	port     int
	pingOn   bool
	lastPing ping.Result
	pingStop context.CancelFunc
	logTail  []string
}

func (g *guiState) appendLog(line string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.logTail = append(g.logTail, line)
	if len(g.logTail) > guiLogLines {
		g.logTail = g.logTail[len(g.logTail)-guiLogLines:]
	}
}

func (g *guiState) logWriter() io.Writer {
	return writerFunc(func(p []byte) (int, error) {
		for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
			if line != "" {
				g.appendLog(line)
			}
		}
		os.Stdout.Write(p)
		return len(p), nil
	})
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func runGUI(args []string) {
	o := parseOptions("gui", args)
	if !isRoot() {
		fatal("TUN butuh root - jalankan: sudo socksctl gui -host %s -port %d", o.host, o.port)
	}
	core, err := findCore(o.core)
	if err != nil {
		fatal("%v", err)
	}

	g := &guiState{core: core, dataDir: o.data, host: o.host, port: o.port}
	_ = os.MkdirAll(o.data, 0o700)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(guiPage))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		state := map[string]interface{}{
			"running":     g.running,
			"host":        g.host,
			"port":        g.port,
			"ping":        g.pingOn,
			"ping_result": g.lastPing.Summary(),
			"log":         strings.Join(g.logTail, "\n"),
		}
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		host, port, user, pass, pingOn := formValues(r)
		if err := g.start(host, port, user, pass, pingOn); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		g.stop()
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		g.setPing(r.FormValue("ping") == "true")
		w.Write([]byte("ok"))
	})

	url := "http://" + o.listen + "/"
	fmt.Printf("socksctl %s - UI di %s\n", appVersion, url)
	fmt.Printf("core   : %s\n", core)
	fmt.Println("hentikan dengan Ctrl+C (tunnel ikut berhenti)")
	openBrowser(url)
	if err := http.ListenAndServe(o.listen, mux); err != nil {
		fatal("UI gagal jalan: %v", err)
	}
}

func formValues(r *http.Request) (string, int, string, string, bool) {
	r.ParseForm()
	host := trim(r.FormValue("host"))
	var port int
	fmt.Sscanf(r.FormValue("port"), "%d", &port)
	if port == 0 {
		port = 1080
	}
	return host, port, trim(r.FormValue("user")), r.FormValue("pass"), r.FormValue("ping") == "true"
}

func (g *guiState) start(host string, port int, user, pass string, pingOn bool) error {
	g.stop()
	if net.ParseIP(host) == nil {
		return fmt.Errorf("host harus berupa IP literal")
	}
	o := options{
		host: host, port: port, user: user, pass: pass,
		mtu: boxcfgMTU(), stack: "gvisor",
		iface: boxcfg.DefaultInterfaceName(runtime.GOOS),
		data:  g.dataDir,
	}
	path, err := writeConfig(o, o.configJSON())
	if err != nil {
		return fmt.Errorf("gagal menulis config: %w", err)
	}
	cmd := exec.Command(g.core, "run", "-c", path, "-D", g.dataDir)
	cmd.Stdout = g.logWriter()
	cmd.Stderr = g.logWriter()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("gagal menjalankan core: %w", err)
	}

	g.mu.Lock()
	g.cmd = cmd
	g.running = true
	g.host = host
	g.port = port
	g.mu.Unlock()

	go func() {
		cmd.Wait()
		os.Remove(path)
		g.mu.Lock()
		g.running = false
		g.cmd = nil
		g.mu.Unlock()
	}()

	g.setPing(pingOn)
	return nil
}

func (g *guiState) stop() {
	g.setPing(false)
	g.mu.Lock()
	cmd := g.cmd
	g.running = false
	g.cmd = nil
	g.lastPing = ping.Result{}
	g.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(os.Interrupt)
		time.Sleep(300 * time.Millisecond)
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
}

// setPing menyalakan/mematikan loop HTTP ping. Ping berjalan di proses UI
// (root), lewat tunnel yang sama karena auto_route menangkap trafiknya.
func (g *guiState) setPing(on bool) {
	g.mu.Lock()
	if g.pingStop != nil {
		g.pingStop()
		g.pingStop = nil
	}
	g.pingOn = on
	if !on || !g.running {
		g.lastPing = ping.Result{}
		g.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.pingStop = cancel
	client := ping.NewClient()
	g.mu.Unlock()

	go ping.Watch(ctx, client, ping.Targets, ping.DefaultInterval, ping.RetryInterval, func(r ping.Result) {
		g.mu.Lock()
		g.lastPing = r
		g.mu.Unlock()
	})
}

func boxcfgMTU() int { return defaultMTU }

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		}
	}
	if cmd != nil {
		cmd.Start()
	}
}

const guiPage = `<!doctype html>
<html lang="id"><head><meta charset="utf-8"><title>Socks Client</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
 body{background:#0a0a0c;color:#f5f5f7;font:14px/1.5 system-ui,Segoe UI,Roboto,sans-serif;margin:0;padding:28px}
 h1{font-size:20px;margin:0 0 4px} .sub{color:#888;font-size:12px;margin-bottom:20px}
 .card{background:#18181e;border-radius:12px;padding:18px;max-width:520px;margin-bottom:14px}
 label{display:block;font-size:12px;color:#9aa0aa;margin:10px 0 4px}
 input[type=text],input[type=password]{width:100%;box-sizing:border-box;background:#101014;border:1px solid #2a2a32;border-radius:8px;color:#f5f5f7;padding:10px;font-size:14px}
 .btn{display:inline-block;border:0;border-radius:8px;padding:11px 18px;font-weight:600;font-size:14px;cursor:pointer;margin-top:14px;margin-right:8px}
 .go{background:#1cb862;color:#fff}.stop{background:#dc3c3c;color:#fff}.ghost{background:#2a2a32;color:#f5f5f7}
 .row{display:flex;align-items:center;gap:8px;margin-top:12px}
 #status{font-weight:600} #ping{color:#9aa0aa;font-size:13px;margin-top:4px}
 pre{background:#101014;border:1px solid #2a2a32;border-radius:8px;padding:10px;height:220px;overflow:auto;font-size:11px;white-space:pre-wrap}
 .ok{color:#1cb862}.bad{color:#dc3c3c}
</style></head><body>
<h1>Socks Client</h1>
<div class="sub">Tunnel SOCKS5 - semua trafik lewat server kamu (Linux/macOS)</div>
<div class="card">
  <label>Server IP</label><input type="text" id="host" placeholder="10.0.0.1">
  <label>Port</label><input type="text" id="port" placeholder="1080" value="1080">
  <label>Username (opsional)</label><input type="text" id="user">
  <label>Password (opsional)</label><input type="password" id="pass">
  <div class="row"><input type="checkbox" id="pingOn"><label for="pingOn" style="margin:0">HTTP ping (204, keep-alive)</label></div>
  <button class="btn go" onclick="start()">Connect</button>
  <button class="btn stop" onclick="stop()">Disconnect</button>
  <div class="row"><span id="status">Status: memuat...</span></div>
  <div id="ping"></div>
</div>
<div class="card"><label>Log core</label><pre id="log"></pre></div>
<script>
 let running=false;
 async function post(path, body){
   const r=await fetch(path,{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:body||''});
   if(!r.ok){ alert(await r.text()); }
 }
 function start(){
   const host=document.getElementById('host').value.trim();
   if(!host){ alert('Server IP wajib diisi'); return; }
   post('/api/start', new URLSearchParams({host:host,port:document.getElementById('port').value,
     user:document.getElementById('user').value, pass:document.getElementById('pass').value,
     ping:document.getElementById('pingOn').checked}));
 }
 function stop(){ post('/api/stop'); }
 async function tick(){
   try{
     const s=await (await fetch('/api/status')).json();
     running=s.running;
     document.getElementById('status').innerHTML='Status: <span class="'+(s.running?'ok':'bad')+'">'+(s.running?'Connected':'Disconnected')+'</span>'+(s.running?' '+s.host+':'+s.port:'');
     document.getElementById('ping').textContent = s.ping ? ('Ping: '+s.ping_result) : 'Ping: off';
     const log=document.getElementById('log');
     const atBottom = log.scrollTop+log.clientHeight >= log.scrollHeight-20;
     log.textContent=s.log.split('\n').slice(-120).join('\n');
     if(atBottom) log.scrollTop=log.scrollHeight;
     if(s.running){ document.getElementById('host').value=s.host||document.getElementById('host').value; document.getElementById('port').value=s.port; }
   }catch(e){ document.getElementById('status').textContent='Status: UI terputus'; }
 }
 document.getElementById('pingOn').addEventListener('change',function(){ post('/api/ping','ping='+this.checked); });
 setInterval(tick,2000); tick();
</script></body></html>`
