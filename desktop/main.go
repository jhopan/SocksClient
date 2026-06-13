package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/energye/systray"
	webview "github.com/jchv/go-webview2"
)

//go:embed embed/sing-box.exe embed/logo_store.png embed/app.ico
var singboxBin embed.FS

var logoBase64 string
var trayIconData []byte

func init() {
	data, _ := singboxBin.ReadFile("embed/logo_store.png")
	logoBase64 = base64.StdEncoding.EncodeToString(data)
	trayIconData, _ = singboxBin.ReadFile("embed/app.ico")
}

const (
	appName     = "Socks Client Desktop"
	appVersion  = "1.0.0"
	configFile  = "socks_client_config.json"
	settingFile = "settings.json"
)

type App struct {
	mu            sync.Mutex
	wv            webview.WebView
	process       *exec.Cmd
	connected     bool
	settings      Settings
	appDir        string
	trayRunning   bool
	windowVisible bool
	showChan      chan struct{}
}

type Settings struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Pass        string `json:"pass"`
	Traffic     bool   `json:"traffic"`
	TrayEnabled bool   `json:"tray"`
}

func main() {
	app := &App{windowVisible: true}
	app.loadSettings()

	// Start systray with external loop (non-blocking)
	systrayStart, systrayEnd := systray.RunWithExternalLoop(app.onTrayReady, app.onTrayExit)
	systrayStart()

	// Main loop: recreate webview when shown from tray
	for {
		app.runWebview()
		app.windowVisible = false

		// If tray not enabled, exit normally
		if !app.settings.TrayEnabled {
			break
		}

		// Wait for tray "Show" signal
		<-app.showChan
		app.windowVisible = true
	}

	if app.connected {
		app.killProcess()
	}
	systrayEnd()
}

func (a *App) runWebview() {
	w := webview.New(false)
	defer w.Destroy()

	a.wv = w
	w.SetTitle(appName + " v" + appVersion)
	w.SetSize(380, 700, webview.HintMin)

	// Init showChan for tray show signal
	if a.showChan == nil {
		a.showChan = make(chan struct{}, 1)
	}

	// Bind Go functions to JavaScript
	w.Bind("goConnect", func(args string) (string, error) {
		var params []string
		json.Unmarshal([]byte(args), &params)
		host, port, user, pass := "", "", "", ""
		if len(params) > 0 { host = params[0] }
		if len(params) > 1 { port = params[1] }
		if len(params) > 2 { user = params[2] }
		if len(params) > 3 { pass = params[3] }
		return a.connect(host, port, user, pass), nil
	})
	w.Bind("goDisconnect", func(args string) (string, error) {
		a.disconnect()
		return "", nil
	})
	w.Bind("goGetSettings", func(args string) (string, error) {
		data, _ := json.Marshal(a.settings)
		return string(data), nil
	})
	w.Bind("goSaveSettings", func(args string) (string, error) {
		var params []string
		json.Unmarshal([]byte(args), &params)
		if len(params) > 0 {
			json.Unmarshal([]byte(params[0]), &a.settings)
			a.saveSettings()
		}
		return "", nil
	})
	w.Bind("goGetTraffic", func(args string) (string, error) {
		rx, tx := readTunTraffic("socks-tun")
		return fmt.Sprintf(`{"download":%d,"upload":%d}`, rx, tx), nil
	})
	w.Bind("goToggleTraffic", func(args string) (string, error) {
		var params []bool
		json.Unmarshal([]byte(args), &params)
		if len(params) > 0 {
			a.settings.Traffic = params[0]
			a.saveSettings()
		}
		return "", nil
	})
	w.Bind("goSetTray", func(args string) (string, error) {
		var params []bool
		json.Unmarshal([]byte(args), &params)
		if len(params) > 0 {
			a.settings.TrayEnabled = params[0]
			a.saveSettings()
		}
		return "", nil
	})

	w.SetHtml(htmlContent())

	// Set window icon (taskbar + title bar)
	if runtime.GOOS == "windows" {
		setWindowIcon(w.Window())
	}

	w.Run()
}

func (a *App) onTrayReady() {
	systray.SetIcon(trayIconData)
	systray.SetTitle(appName)
	systray.SetTooltip(appName + " v" + appVersion)

	// Menu items
	mShow := systray.AddMenuItem("🪟 Show Window", "Show the main window")
	systray.AddSeparator()
	mConnect := systray.AddMenuItem("🔗 Connect", "Connect to SOCKS5 VPN")
	mDisconnect := systray.AddMenuItem("⛔ Disconnect", "Disconnect VPN")
	systray.AddSeparator()
	mExit := systray.AddMenuItem("❌ Exit", "Exit application")

	// Double-click tray icon = show window
	systray.SetOnDClick(func(menu systray.IMenu) {
		a.showWindow()
	})

	// Menu click handlers
	mShow.Click(func() { a.showWindow() })
	mConnect.Click(func() {
		if !a.connected {
			a.connect(a.settings.Host, strconv.Itoa(a.settings.Port), a.settings.User, a.settings.Pass)
		}
	})
	mDisconnect.Click(func() {
		if a.connected {
			a.disconnect()
		}
	})
	mExit.Click(func() {
		a.settings.TrayEnabled = false
		a.showWindow()
		systray.Quit()
	})
}

func (a *App) onTrayExit() {}

func (a *App) showWindow() {
	if a.showChan != nil {
		select {
		case a.showChan <- struct{}{}:
		default:
		}
	}
}

// ─── Actions ───────────────────────────────────────────────

func (a *App) connect(host, portStr, user, pass string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return "already connected"
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return "Host/IP wajib diisi!"
	}

	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return "Port tidak valid! (1-65535)"
	}

	a.settings = Settings{Host: host, Port: port, User: strings.TrimSpace(user), Pass: pass, Traffic: a.settings.Traffic}
	a.saveSettings()

	binPath, err := a.extractSingbox()
	if err != nil {
		return "sing-box error: " + err.Error()
	}

	config := generateConfig(host, port, strings.TrimSpace(user), pass)
	configPath := filepath.Join(a.getAppDir(), configFile)
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	cmd := exec.Command(binPath, "run", "-c", configPath, "-D", filepath.Dir(configPath))
	if runtime.GOOS == "windows" {
		attr := windowsSysProcAttr()
		cmd.SysProcAttr = &attr
	}

	if err := cmd.Start(); err != nil {
		return "Failed to start: " + err.Error() + "\n\nRun as Administrator!"
	}

	time.Sleep(800 * time.Millisecond)
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return "sing-box exited. Run as Administrator!"
	}

	a.process = cmd
	a.connected = true
	go a.monitorTraffic()

	return ""
}

func (a *App) disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.killProcess()
	a.connected = false
}

func (a *App) killProcess() {
	if a.process != nil && a.process.Process != nil {
		a.process.Process.Kill()
		a.process.Wait()
	}
	a.process = nil
	a.connected = false
}

func (a *App) monitorTraffic() {
	for {
		a.mu.Lock()
		c := a.connected
		a.mu.Unlock()
		if !c {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// ─── sing-box extraction ───────────────────────────────────

func (a *App) extractSingbox() (string, error) {
	appDir := a.getAppDir()
	binName := "sing-box.exe"
	if runtime.GOOS != "windows" {
		binName = "sing-box"
	}
	binPath := filepath.Join(appDir, binName)

	if info, err := os.Stat(binPath); err == nil && info.Size() > 1000 {
		return binPath, nil
	}

	data, err := singboxBin.ReadFile("embed/sing-box.exe")
	if err != nil {
		return "", err
	}
	return binPath, os.WriteFile(binPath, data, 0755)
}

func (a *App) getAppDir() string {
	if a.appDir != "" {
		return a.appDir
	}
	dir, _ := os.UserCacheDir()
	a.appDir = filepath.Join(dir, "socks-client-desktop")
	os.MkdirAll(a.appDir, 0755)
	return a.appDir
}

// ─── Settings ──────────────────────────────────────────────

func (a *App) loadSettings() {
	a.settings = Settings{Host: "192.168.1.10", Port: 1080, Traffic: true}
	dir, _ := os.UserConfigDir()
	path := filepath.Join(dir, "socks-client-desktop", settingFile)
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &a.settings)
	}
}

func (a *App) saveSettings() {
	dir, _ := os.UserConfigDir()
	appDir := filepath.Join(dir, "socks-client-desktop")
	os.MkdirAll(appDir, 0755)
	data, _ := json.MarshalIndent(a.settings, "", "  ")
	os.WriteFile(filepath.Join(appDir, settingFile), data, 0644)
}

// ─── Config ────────────────────────────────────────────────

func generateConfig(host string, port int, user, pass string) map[string]interface{} {
	outbound := map[string]interface{}{
		"type": "socks", "tag": "socks-out",
		"server": host, "server_port": port, "version": "5",
	}
	if user != "" {
		outbound["username"] = user
		if pass != "" {
			outbound["password"] = pass
		}
	}
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "warn"},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{"tag": "remote", "address": "tcp://8.8.8.8", "detour": "socks-out"},
				{"tag": "local", "address": "1.1.1.1", "detour": "direct"},
			},
			"rules":    []map[string]interface{}{{"outbound": "any", "server": "local"}},
			"strategy": "prefer_ipv4",
		},
		"inbounds": []map[string]interface{}{{
			"type": "tun", "tag": "tun-in", "interface_name": "socks-tun",
			"address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			"mtu": 1400, "auto_route": true, "strict_route": false,
			"stack": "system", "sniff": true,
		}},
		"outbounds": []interface{}{
			outbound,
			map[string]interface{}{"type": "direct", "tag": "direct"},
			map[string]interface{}{"type": "block", "tag": "block"},
		},
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"rules":                 []map[string]interface{}{{"outbound": "socks-out"}},
		},
	}
}

// ─── Traffic ───────────────────────────────────────────────

func readTunTraffic(tunName string) (int64, int64) {
	switch runtime.GOOS {
	case "linux":
		rx, _ := readIntFile(fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", tunName))
		tx, _ := readIntFile(fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", tunName))
		return rx, tx
	case "windows":
		out, err := exec.Command("powershell", "-Command",
			fmt.Sprintf("Get-NetAdapterStatistics -Name '*%s*','*Wintun*' -EA SilentlyContinue | select -First 1 | %% { $_.ReceivedBytes.ToString()+' '+$_.SentBytes.ToString() }", tunName),
		).Output()
		if err != nil {
			return 0, 0
		}
		parts := strings.Fields(string(out))
		if len(parts) >= 2 {
			rx, _ := strconv.ParseInt(parts[0], 10, 64)
			tx, _ := strconv.ParseInt(parts[1], 10, 64)
			return rx, tx
		}
	case "darwin":
		out, err := exec.Command("netstat", "-ibn").Output()
		if err != nil {
			return 0, 0
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, tunName) || strings.Contains(strings.ToLower(line), "utun") {
				parts := strings.Fields(line)
				if len(parts) >= 10 {
					rx, _ := strconv.ParseInt(parts[6], 10, 64)
					tx, _ := strconv.ParseInt(parts[9], 10, 64)
					return rx, tx
				}
			}
		}
	}
	return 0, 0
}

func readIntFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// ─── Windows helper ────────────────────────────────────────

func windowsSysProcAttr() syscall.SysProcAttr {
	return syscall.SysProcAttr{HideWindow: true}
}

// ─── Set window icon via Win32 API ──────────────────────────

func setWindowIcon(hwnd unsafe.Pointer) {
	if hwnd == nil {
		return
	}
	// Extract ICO to temp file
	icoPath := filepath.Join(os.TempDir(), "socks-client-icon.ico")
	os.WriteFile(icoPath, trayIconData, 0644)

	user32 := syscall.NewLazyDLL("user32.dll")
	sendMessage := user32.NewProc("SendMessageW")
	loadImage := user32.NewProc("LoadImageW")

	// WM_SETICON = 0x0080
	// ICON_SMALL = 0, ICON_BIG = 1
	// LR_LOADFROMFILE = 0x0010, IMAGE_ICON = 1
	const (
		WM_SETICON       = 0x0080
		ICON_SMALL       = 0
		ICON_BIG         = 1
		IMAGE_ICON       = 1
		LR_LOADFROMFILE  = 0x0010
	)

	icoPathW, _ := syscall.UTF16PtrFromString(icoPath)

	// Load big icon (32x32)
	bigIcon, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(icoPathW)), IMAGE_ICON, 32, 32, LR_LOADFROMFILE)
	if bigIcon != 0 {
		sendMessage.Call(uintptr(hwnd), WM_SETICON, ICON_BIG, bigIcon)
	}

	// Load small icon (16x16)
	smallIcon, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(icoPathW)), IMAGE_ICON, 16, 16, LR_LOADFROMFILE)
	if smallIcon != 0 {
		sendMessage.Call(uintptr(hwnd), WM_SETICON, ICON_SMALL, smallIcon)
	}
}

// ─── HTML UI ───────────────────────────────────────────────

func htmlContent() string {
	logoImg := "data:image/png;base64," + logoBase64
	return strings.Replace(htmlTemplate, "{{LOGO_IMG}}", logoImg, 1)
}

var htmlTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
*{margin:0;padding:0;box-sizing:border-box}
html,body{height:100%;overflow-x:hidden}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;background:#0e0e12;color:#e8e8ec;user-select:none;-webkit-font-smoothing:antialiased}
.container{width:100%;padding:14px 18px 16px}
.logo-area{text-align:center;margin-bottom:8px}
.logo-area img{max-width:200px;height:auto;border-radius:8px;filter:drop-shadow(0 2px 12px rgba(28,184,98,0.25))}
.title{font-size:19px;font-weight:700;text-align:center;letter-spacing:-0.3px;color:#ffffff}
.subtitle{font-size:11px;color:#999;text-align:center;margin-bottom:14px}
.field{background:#1a1a22;border-radius:10px;padding:10px 14px;margin-bottom:7px;border:1px solid #2a2a34;transition:border-color .2s}
.field:focus-within{border-color:#1cb862}
.field label{font-size:10px;color:#999;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;display:block;margin-bottom:3px}
.field input{width:100%;background:transparent;border:none;color:#ffffff;font-size:14px;outline:none;font-family:inherit;font-weight:500}
.field input::placeholder{color:#555}
.traffic-box{background:#1a1a22;border-radius:10px;padding:10px 13px;margin:8px 0;border:1px solid #2a2a34}
.traffic-toggle{display:flex;align-items:center;gap:10px;margin-bottom:6px}
.traffic-toggle span{font-size:12px}
.switch{position:relative;width:38px;height:20px;flex-shrink:0}
.switch input{opacity:0;width:0;height:0}
.slider{position:absolute;cursor:pointer;top:0;left:0;right:0;bottom:0;background:#3a3a44;border-radius:20px;transition:.3s}
.slider:before{content:"";position:absolute;height:14px;width:14px;left:3px;bottom:3px;background:white;border-radius:50%;transition:.3s}
input:checked+.slider{background:#1cb862}
input:checked+.slider:before{transform:translateX(18px)}
.traffic-stats{display:flex;justify-content:space-between}
.stat{font-family:'Consolas','Courier New',monospace;font-size:13px}
.stat.dl{color:#4caf50}.stat.ul{color:#42a5f5}
.btn{width:100%;padding:11px;border:none;border-radius:10px;font-size:14px;font-weight:600;cursor:pointer;margin-bottom:6px;font-family:inherit;transition:all .15s}
.btn:hover{opacity:0.88}.btn:active{opacity:0.7}
.btn:disabled{opacity:0.35;cursor:not-allowed}
.btn-connect{background:linear-gradient(135deg,#1cb862,#15a055);color:white}
.btn-disconnect{background:linear-gradient(135deg,#dc3c3c,#c03030);color:white}
.btn-guide{background:#466e82;color:white}
.btn-social{background:#2a2a36;color:#aaa;font-size:12px}
.status{text-align:center;font-size:13px;font-weight:600;margin:10px 0 4px;padding:8px;border-radius:8px;background:#1a1a22;border:1px solid #2a2a34}
.status.connected{color:#1cb862;border-color:#1cb86233}
.status.error{color:#dc3c3c;border-color:#dc3c3c33}
.footer{text-align:center;font-size:9px;color:#666;margin-top:6px}
.modal-overlay{display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.8);z-index:100;justify-content:center;align-items:center}
.modal-overlay.active{display:flex}
.modal{background:#252529;border-radius:14px;padding:18px;max-width:320px;width:90%;max-height:80vh;overflow-y:auto;border:1px solid #444}
.modal h2{font-size:15px;margin-bottom:10px;color:#f0f0f2}
.modal p{font-size:12px;line-height:1.7;color:#bbb;margin-bottom:5px}
.modal .close-btn{width:100%;padding:9px;background:#466e82;color:white;border:none;border-radius:8px;font-size:13px;cursor:pointer;margin-top:10px;font-family:inherit;font-weight:600}
.modal .close-btn:hover{background:#5a8296}
</style></head><body>
<div class="container">
  <div class="logo-area"><img src="{{LOGO_IMG}}" alt="Logo"></div>
  <div class="title">Socks Client Desktop</div>
  <div class="subtitle">Client VPN untuk SOCKS5 server hotspot</div>
  <div class="field"><label>HOST / IP</label><input id="host" type="text" placeholder="192.168.1.10"></div>
  <div class="field"><label>PORT</label><input id="port" type="number" placeholder="1080"></div>
  <div class="field"><label>USERNAME (opsional)</label><input id="user" type="text" placeholder=""></div>
  <div class="field"><label>PASSWORD (opsional)</label><input id="pass" type="password" placeholder=""></div>
  <div class="traffic-box">
    <div class="traffic-toggle">
      <label class="switch"><input type="checkbox" id="trafficToggle" checked><span class="slider"></span></label>
      <span>Traffic Counter</span>
    </div>
    <div class="traffic-stats">
      <span class="stat dl" id="dlStat">↓ 0 B</span>
      <span class="stat ul" id="ulStat">↑ 0 B</span>
    </div>
  </div>
  <div class="traffic-box" style="margin-top:4px">
    <div class="traffic-toggle">
      <label class="switch"><input type="checkbox" id="trayToggle"><span class="slider"></span></label>
      <span>🗂️ Minimize to Tray saat ditutup</span>
    </div>
  </div>
  <button class="btn btn-connect" id="connectBtn" onclick="doConnect()">🔗 Connect Socks VPN</button>
  <button class="btn btn-disconnect" id="disconnectBtn" onclick="doDisconnect()" disabled>⛔ Disconnect</button>
  <button class="btn btn-guide" onclick="showModal('guideModal')">📖 Cara Pakai</button>
  <button class="btn btn-social" onclick="showModal('socialModal')">🌐 JhopanStore — Social Media</button>
  <div class="status" id="statusBar">Status: Disconnected</div>
  <div class="footer">v1.0.0 — Powered by sing-box</div>
</div>
<div class="modal-overlay" id="guideModal" onclick="if(event.target===this)closeModal('guideModal')">
  <div class="modal">
    <h2>📖 Panduan Socks Client</h2>
    <p><b>1)</b> Pastikan HP server menjalankan <b>VPN Hospot</b> app.</p>
    <p><b>2)</b> Hubungkan PC ke hotspot/USB tether server.</p>
    <p><b>3)</b> Isi Host/IP server, Port (default 1080).</p>
    <p><b>4)</b> Klik <b>Connect Socks VPN</b>.</p>
    <p><b>5)</b> Izinkan admin/root saat diminta.</p>
    <p><b>6)</b> Semua traffic PC lewat SOCKS5! ✓</p>
    <p style="margin-top:8px"><b>⚠️ Admin/Root Required:</b></p>
    <p>• Windows: Klik kanan → Run as Administrator</p>
    <p>• macOS/Linux: sudo ./socks-client</p>
    <button class="close-btn" onclick="closeModal('guideModal')">Tutup</button>
  </div>
</div>
<div class="modal-overlay" id="socialModal" onclick="if(event.target===this)closeModal('socialModal')">
  <div class="modal">
    <h2>🌐 JhopanStore</h2>
    <p>📱 <b>Telegram:</b> @JhopanStore</p>
    <p>📸 <b>Instagram:</b> @jhopanstore</p>
    <p>🎬 <b>YouTube:</b> JhopanStore</p>
    <p style="margin-top:8px"><b>GitHub:</b></p>
    <p>• github.com/jhopan/VpnHospotByJhopanStore</p>
    <p>• github.com/jhopan/SocksClientByJhopanStore</p>
    <button class="close-btn" onclick="closeModal('socialModal')">Tutup</button>
  </div>
</div>
<script>
let connected=false,trafficEnabled=true;
async function init(){try{const s=JSON.parse(await goGetSettings());document.getElementById('host').value=s.host||'';document.getElementById('port').value=s.port||1080;document.getElementById('user').value=s.user||'';document.getElementById('pass').value=s.pass||'';document.getElementById('trafficToggle').checked=s.traffic!==false;trafficEnabled=s.traffic!==false;document.getElementById('trayToggle').checked=s.tray===true}catch(e){}pollTraffic()}
async function doConnect(){const h=document.getElementById('host').value,p=document.getElementById('port').value,u=document.getElementById('user').value,ps=document.getElementById('pass').value,tr=document.getElementById('trayToggle').checked;await goSaveSettings(JSON.stringify({host:h,port:parseInt(p),user:u,pass:ps,traffic:trafficEnabled,tray:tr}));document.getElementById('connectBtn').disabled=true;setStatus('Connecting...','');const e=await goConnect(h,String(p),u,ps);if(e){setStatus('Error: '+e,'error');document.getElementById('connectBtn').disabled=false}else{connected=true;setStatus('Connected ✓ '+h+':'+p,'connected');document.getElementById('connectBtn').disabled=true;document.getElementById('disconnectBtn').disabled=false}}
async function doDisconnect(){await goDisconnect();connected=false;setStatus('Disconnected','');document.getElementById('connectBtn').disabled=false;document.getElementById('disconnectBtn').disabled=true;document.getElementById('dlStat').textContent='↓ 0 B';document.getElementById('ulStat').textContent='↑ 0 B'}
function setStatus(t,c){const e=document.getElementById('statusBar');e.textContent='Status: '+t;e.className='status '+c}
async function pollTraffic(){while(true){await new Promise(r=>setTimeout(r,2000));if(!connected||!trafficEnabled)continue;try{const t=JSON.parse(await goGetTraffic());document.getElementById('dlStat').textContent='↓ '+hb(t.download);document.getElementById('ulStat').textContent='↑ '+hb(t.upload)}catch(e){}}}
function hb(b){if(!b)return'0 B';const u=['B','KB','MB','GB','TB'];let i=0;while(b>=1024&&i<u.length-1){b/=1024;i++}return(i===0?b:b.toFixed(2))+' '+u[i]}
document.getElementById('trafficToggle').addEventListener('change',function(){trafficEnabled=this.checked;goToggleTraffic(this.checked);if(!this.checked){document.getElementById('dlStat').textContent='↓ —';document.getElementById('ulStat').textContent='↑ —'}});
document.getElementById('trayToggle').addEventListener('change',function(){goSetTray(this.checked)});
function showModal(id){document.getElementById(id).classList.add('active')}
function closeModal(id){document.getElementById(id).classList.remove('active')}
init();
</script></body></html>`
