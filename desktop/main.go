package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/energye/systray"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func openURL(url string) {
	exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

// The core is deliberately NOT embedded: it is built by the "Build Core"
// workflow and fetched with desktop/scripts/fetch-core.sh, so the repo and the
// binary stay small.
//
//go:embed embed/app.ico embed/logo_store.png
var embeddedFiles embed.FS

const (
	appName          = "Socks Client Desktop"
	appVersion       = "1.2.0"
	modeTun          = "tun"
	modeProxy        = "proxy"
	defaultLocalPort = 2080
	lockFileName     = "socks_client_desktop.lock"
)

var logoPath string
var trayIconPath string

type App struct {
	mu        sync.Mutex
	mw        *walk.MainWindow
	process   *exec.Cmd
	connected bool
	settings  Settings
	runDir    string
	appDir    string

	hostEdit      *walk.LineEdit
	portEdit      *walk.LineEdit
	userEdit      *walk.LineEdit
	passEdit      *walk.LineEdit
	localPortEdit *walk.LineEdit
	tunRB         *walk.RadioButton
	proxyRB       *walk.RadioButton
	sysProxyCB    *walk.CheckBox
	connectBtn    *walk.PushButton
	disconnBtn    *walk.PushButton
	statusLabel   *walk.Label
	trayCB        *walk.CheckBox

	trayEnabled   bool
	trayStarted   bool
	windowVisible bool
}

type Settings struct {
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	User        string       `json:"user"`
	Pass        string       `json:"pass"`
	Tray        bool         `json:"tray"`
	Mode        string       `json:"mode"`
	LocalPort   int          `json:"local_port"`
	SystemProxy bool         `json:"system_proxy"`
	ProxyBackup *ProxyBackup `json:"proxy_backup,omitempty"`
}

// runtimeDir keeps the writable bits (settings, config, sing-box.exe, log) out
// of Program Files so mode "proxy" can run without Administrator rights.
func runtimeDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "SocksClientDesktop")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return os.TempDir()
	}
	return dir
}

func main() {
	app := &App{trayEnabled: true, windowVisible: true}
	app.runDir = runtimeDir()
	if exe, err := os.Executable(); err == nil {
		app.appDir = filepath.Dir(exe)
	}
	app.loadSettings()

	// Single instance check via lock file
	lockPath := filepath.Join(app.runDir, lockFileName)
	if !checkAndLock(lockPath) {
		showExisting()
		return
	}
	defer os.Remove(lockPath)

	// A leftover backup means the previous run died while the system proxy was
	// pointed at us. Only touch it once we own the lock - a live instance still
	// holds the proxy on purpose.
	app.restoreSystemProxy()

	// Windows named mutex - Inno Setup AppMutex detects this
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	pMutex := kernel32.NewProc("CreateMutexW")
	pMutex.Call(0, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("SocksClientDesktopMutex"))))

	// Extract logo (PNG for banner)
	logoData, _ := embeddedFiles.ReadFile("embed/logo_store.png")
	tmpLogo := filepath.Join(os.TempDir(), "socks_logo.png")
	os.WriteFile(tmpLogo, logoData, 0644)
	logoPath = tmpLogo

	// Extract tray icon (ICO)
	icoData, _ := embeddedFiles.ReadFile("embed/app.ico")
	tmpIco := filepath.Join(os.TempDir(), "socks_tray.ico")
	os.WriteFile(tmpIco, icoData, 0644)
	trayIconPath = tmpIco

	app.runUI()

	// Backup cleanup - if runUI returns (window closed without exitApp)
	app.killProcess()
	os.Remove(lockPath)
	os.Exit(0)
}

// --- Single Instance ----------------------------------

func checkAndLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0644)
		return true
	}
	pid, _ := strconv.Atoi(string(data))
	if pid > 0 {
		proc, err := os.FindProcess(pid)
		if err == nil && proc.Signal(syscall.Signal(0)) == nil {
			return false // alive = another instance
		}
	}
	os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0644)
	return true
}

func showExisting() {
	user32 := syscall.NewLazyDLL("user32.dll")
	pEnum := user32.NewProc("EnumWindows")
	pPID := user32.NewProc("GetWindowThreadProcessId")
	pShow := user32.NewProc("ShowWindow")
	pFore := user32.NewProc("SetForegroundWindow")

	lockPath := filepath.Join(runtimeDir(), lockFileName)
	data, _ := os.ReadFile(lockPath)
	pid, _ := strconv.Atoi(string(data))
	if pid <= 0 {
		return
	}

	cb := syscall.NewCallback(func(hwnd, lParam uintptr) uintptr {
		var wpid uint32
		pPID.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if int(wpid) == pid {
			pShow.Call(hwnd, 9) // SW_RESTORE
			pFore.Call(hwnd)
		}
		return 1
	})
	pEnum.Call(cb, 0)
}

// --- Settings -----------------------------------------

func (a *App) loadSettings() {
	a.settings = Settings{Port: 1080, Tray: true, Mode: modeTun, LocalPort: defaultLocalPort, SystemProxy: true}

	paths := []string{filepath.Join(a.runDir, "settings.json")}
	if a.appDir != "" {
		// pre-1.2.0 installs kept settings.json next to the exe (Program Files)
		paths = append(paths, filepath.Join(a.appDir, "settings.json"))
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		json.Unmarshal(data, &a.settings)
		break
	}

	a.normalizeSettings()
	a.trayEnabled = a.settings.Tray
}

func (a *App) normalizeSettings() {
	if a.settings.Mode != modeProxy {
		a.settings.Mode = modeTun
	}
	if a.settings.Port < 1 || a.settings.Port > 65535 {
		a.settings.Port = 1080
	}
	if a.settings.LocalPort < 1024 || a.settings.LocalPort > 65535 {
		a.settings.LocalPort = defaultLocalPort
	}
}

func (a *App) saveSettings() {
	data, _ := json.MarshalIndent(a.settings, "", "  ")
	os.WriteFile(filepath.Join(a.runDir, "settings.json"), data, 0644)
}

// --- UI -----------------------------------------------

func (a *App) runUI() {
	var hostEdit, portEdit, userEdit, passEdit, localPortEdit *walk.LineEdit
	var tunRB, proxyRB *walk.RadioButton
	var sysProxyCB, trayCB *walk.CheckBox
	var connectBtn, disconnBtn *walk.PushButton
	var statusLabel *walk.Label
	var logoView *walk.ImageView

	a.mw = new(walk.MainWindow)

	MainWindow{
		AssignTo: &a.mw,
		Title:    appName + " v" + appVersion,
		MinSize:  Size{Width: 400, Height: 620},
		Size:     Size{Width: 400, Height: 620},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "E&xit", OnTriggered: func() { a.exitApp() }},
			}},
		},
		Children: []Widget{
			ImageView{AssignTo: &logoView, Mode: ImageViewModeZoom, MaxSize: Size{Width: 300, Height: 110}, Margin: 10},
			Composite{Layout: VBox{Margins: Margins{Left: 10, Right: 10}}, Children: []Widget{
				Label{Text: appName, Font: Font{Family: "Segoe UI", PointSize: 14, Bold: true}, Alignment: AlignHCenterVCenter},
				Label{Text: "by JhopanStore", Font: Font{Family: "Segoe UI", PointSize: 10, Bold: true, Italic: true}, Alignment: AlignHCenterVCenter},
				Label{Text: "v" + appVersion + " - Powered by sing-box", Font: Font{Family: "Segoe UI", PointSize: 8}, Alignment: AlignHCenterVCenter},
			}},
			Composite{Layout: Grid{Columns: 2, Margins: Margins{Left: 15, Top: 10, Right: 15, Bottom: 5}, Spacing: 6}, Children: []Widget{
				Label{Text: "Host:", Font: Font{Family: "Segoe UI", PointSize: 9}},
				LineEdit{AssignTo: &hostEdit, Text: a.settings.Host, Font: Font{Family: "Segoe UI", PointSize: 9}},
				Label{Text: "Port:", Font: Font{Family: "Segoe UI", PointSize: 9}},
				LineEdit{AssignTo: &portEdit, Text: strconv.Itoa(a.settings.Port), Font: Font{Family: "Segoe UI", PointSize: 9}},
				Label{Text: "User:", Font: Font{Family: "Segoe UI", PointSize: 9}},
				LineEdit{AssignTo: &userEdit, Text: a.settings.User, Font: Font{Family: "Segoe UI", PointSize: 9}},
				Label{Text: "Pass:", Font: Font{Family: "Segoe UI", PointSize: 9}},
				LineEdit{AssignTo: &passEdit, Text: a.settings.Pass, PasswordMode: true, Font: Font{Family: "Segoe UI", PointSize: 9}},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 6, Right: 15, Bottom: 4}, Spacing: 4}, Children: []Widget{
				Label{Text: "Mode koneksi:", Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true}},
				Composite{Layout: HBox{Spacing: 12}, Children: []Widget{
					RadioButton{AssignTo: &tunRB, Text: "TUN (butuh admin)", Font: Font{Family: "Segoe UI", PointSize: 9},
						OnClicked: func() { a.setMode(modeTun) }},
					RadioButton{AssignTo: &proxyRB, Text: "Proxy (tanpa admin)", Font: Font{Family: "Segoe UI", PointSize: 9},
						OnClicked: func() { a.setMode(modeProxy) }},
				}},
				Composite{Layout: Grid{Columns: 2, Spacing: 6}, Children: []Widget{
					Label{Text: "Proxy port:", Font: Font{Family: "Segoe UI", PointSize: 9}},
					LineEdit{AssignTo: &localPortEdit, Text: strconv.Itoa(a.settings.LocalPort), Font: Font{Family: "Segoe UI", PointSize: 9}},
				}},
				CheckBox{AssignTo: &sysProxyCB, Text: "Set proxy sistem Windows (mode Proxy)",
					Checked: a.settings.SystemProxy, Font: Font{Family: "Segoe UI", PointSize: 9},
					OnCheckedChanged: func() { a.settings.SystemProxy = sysProxyCB.Checked() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 4, Right: 15, Bottom: 4}}, Children: []Widget{
				CheckBox{AssignTo: &trayCB, Text: "Minimize to tray when closed", Checked: a.settings.Tray, Font: Font{Family: "Segoe UI", PointSize: 9},
					OnCheckedChanged: func() { a.trayEnabled = trayCB.Checked() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 6, Right: 15, Bottom: 5}, Spacing: 6}, Children: []Widget{
				PushButton{AssignTo: &connectBtn, Text: "Connect Socks VPN", Font: Font{Family: "Segoe UI", PointSize: 10, Bold: true},
					OnClicked: func() {
						a.hostEdit, a.portEdit, a.userEdit, a.passEdit = hostEdit, portEdit, userEdit, passEdit
						a.localPortEdit = localPortEdit
						a.connectBtn, a.disconnBtn = connectBtn, disconnBtn
						a.statusLabel = statusLabel
						a.trayCB = trayCB
						a.doConnect()
					}},
				PushButton{AssignTo: &disconnBtn, Text: "Disconnect", Enabled: false, Font: Font{Family: "Segoe UI", PointSize: 10},
					OnClicked: func() {
						a.disconnBtn, a.connectBtn = disconnBtn, connectBtn
						a.statusLabel = statusLabel
						a.doDisconnect()
					}},
			}},
			Composite{Layout: HBox{Margins: Margins{Left: 15, Top: 5, Right: 15, Bottom: 5}, Spacing: 6}, Children: []Widget{
				PushButton{Text: "Cara Pakai", Font: Font{Family: "Segoe UI", PointSize: 9}, OnClicked: func() { a.showHowTo() }},
				PushButton{Text: "Info Developer", Font: Font{Family: "Segoe UI", PointSize: 9}, OnClicked: func() { a.showDeveloperInfo() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 6, Right: 15, Bottom: 10}}, Children: []Widget{
				Label{AssignTo: &statusLabel, Text: "Status: Disconnected", Font: Font{Family: "Segoe UI", PointSize: 9}, Alignment: AlignHCenterVCenter},
			}},
		},
	}.Create()

	a.hostEdit, a.portEdit, a.userEdit, a.passEdit = hostEdit, portEdit, userEdit, passEdit
	a.localPortEdit = localPortEdit
	a.tunRB, a.proxyRB = tunRB, proxyRB
	a.sysProxyCB = sysProxyCB
	a.connectBtn, a.disconnBtn = connectBtn, disconnBtn
	a.statusLabel = statusLabel
	a.trayCB = trayCB

	if a.settings.Mode == modeProxy {
		proxyRB.SetChecked(true)
	} else {
		tunRB.SetChecked(true)
	}
	a.applyModeToUI()

	// Set window icon (taskbar) from ICO
	if ico, err := walk.NewIconFromFile(trayIconPath); err == nil {
		a.mw.SetIcon(ico)
	}

	// Window close - minimize to tray (if enabled) or exit cleanly
	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if a.trayEnabled {
			*canceled = true
			win.ShowWindow(a.mw.Handle(), win.SW_HIDE)
			a.windowVisible = false
		} else {
			a.killProcess()
			systray.Quit()
		}
	})

	// Load logo banner
	if logoView != nil {
		if bmp, err := walk.NewBitmapFromFile(logoPath); err == nil {
			logoView.SetImage(bmp)
		}
	}

	// Start system tray in background goroutine
	go a.startTray()

	a.mw.Run()
}

func (a *App) currentMode() string {
	if a.proxyRB != nil && a.proxyRB.Checked() {
		return modeProxy
	}
	return modeTun
}

func (a *App) applyModeToUI() {
	isProxy := a.currentMode() == modeProxy
	a.settings.Mode = a.currentMode()
	if a.localPortEdit != nil {
		a.localPortEdit.SetEnabled(isProxy)
	}
	if a.sysProxyCB != nil {
		a.sysProxyCB.SetEnabled(isProxy)
	}
}

func (a *App) setMode(mode string) {
	if a.tunRB != nil && a.proxyRB != nil {
		if mode == modeProxy {
			a.proxyRB.SetChecked(true)
		} else {
			a.tunRB.SetChecked(true)
		}
	}
	a.applyModeToUI()
	a.saveSettings()
}

// --- System Tray --------------------------------------

func (a *App) startTray() {
	if a.trayStarted {
		return
	}
	a.trayStarted = true

	systray.Run(func() {
		icoData, _ := os.ReadFile(trayIconPath)
		systray.SetIcon(icoData)
		systray.SetTitle(appName)
		systray.SetTooltip(appName + " v" + appVersion + "\nby JhopanStore\nMode: " + a.settings.Mode)

		mShow := systray.AddMenuItem("Show Window", "")
		systray.AddSeparator()
		mConnect := systray.AddMenuItem("Connect", "")
		mDisconnect := systray.AddMenuItem("Disconnect", "")
		systray.AddSeparator()
		mExit := systray.AddMenuItem("Exit", "")

		mShow.Click(func() { a.showFromTray() })
		systray.SetOnDClick(func(menu systray.IMenu) { a.showFromTray() })
		mConnect.Click(func() { a.mw.Synchronize(func() { a.doConnect() }) })
		mDisconnect.Click(func() { a.mw.Synchronize(func() { a.doDisconnect() }) })
		mExit.Click(func() { a.exitApp() })
	}, func() {})
}

func (a *App) showFromTray() {
	if a.mw == nil {
		return
	}
	a.mw.Synchronize(func() {
		win.ShowWindow(a.mw.Handle(), win.SW_RESTORE)
		win.SetForegroundWindow(a.mw.Handle())
		a.windowVisible = true
	})
}

func (a *App) exitApp() {
	a.killProcess()
	systray.Quit()
	a.trayEnabled = false
	a.mw.Close()
	os.Exit(0)
}

// --- Connect / Disconnect -----------------------------

func (a *App) doConnect() {
	host := strings.TrimSpace(a.hostEdit.Text())
	port := strings.TrimSpace(a.portEdit.Text())
	user := strings.TrimSpace(a.userEdit.Text())
	pass := a.passEdit.Text()

	if host == "" {
		walk.MsgBox(a.mw, "Error", "Host cannot be empty", walk.MsgBoxIconWarning)
		return
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		walk.MsgBox(a.mw, "Error", "Invalid port", walk.MsgBoxIconWarning)
		return
	}

	mode := a.currentMode()
	localPort := a.settings.LocalPort
	if mode == modeProxy {
		localPort, err = strconv.Atoi(strings.TrimSpace(a.localPortEdit.Text()))
		if err != nil || localPort < 1024 || localPort > 65535 {
			walk.MsgBox(a.mw, "Error", "Proxy port tidak valid (1024-65535)", walk.MsgBoxIconWarning)
			return
		}
	}

	if a.connected {
		return
	}

	// TUN needs Administrator; offer the two sane escapes instead of failing later.
	if mode == modeTun && !isAdmin() {
		answer := walk.MsgBox(a.mw, "Mode TUN butuh Administrator",
			"Mode TUN memerlukan hak Administrator.\n\n"+
				"Yes  = jalankan ulang sebagai Administrator\n"+
				"No   = lanjut pakai mode Proxy (tanpa admin)",
			walk.MsgBoxYesNo|walk.MsgBoxIconQuestion)
		if answer == walk.DlgCmdYes {
			a.relaunchElevated()
			return
		}
		mode = modeProxy
		a.setMode(modeProxy)
		a.statusLabel.SetText("Status: mode Proxy (tanpa admin)")
	}

	a.settings = Settings{
		Host: host, Port: portNum, User: user, Pass: pass,
		Tray: a.trayCB.Checked(), Mode: mode,
		LocalPort: localPort, SystemProxy: a.sysProxyCB.Checked(),
		ProxyBackup: a.settings.ProxyBackup,
	}
	a.saveSettings()
	a.statusLabel.SetText("Status: Connecting...")
	a.connectBtn.SetEnabled(false)

	go func() {
		errMsg := a.startCore(mode, host, portNum, user, pass, localPort)
		a.mw.Synchronize(func() {
			if errMsg != "" {
				a.statusLabel.SetText("Status: " + errMsg)
				a.connectBtn.SetEnabled(true)
				return
			}
			a.connected = true
			a.statusLabel.SetText(a.connectedStatus(mode, host, portNum, localPort))
			a.connectBtn.SetEnabled(false)
			a.disconnBtn.SetEnabled(true)
		})
	}()
}

func (a *App) connectedStatus(mode, host string, port, localPort int) string {
	if mode == modeProxy {
		text := fmt.Sprintf("Status: Connected (Proxy) %s:%d via 127.0.0.1:%d", host, port, localPort)
		if a.settings.SystemProxy {
			text += " - proxy sistem aktif"
		}
		return text
	}
	return fmt.Sprintf("Status: Connected (TUN) %s:%d", host, port)
}

func socksOutbound(host string, port int, user, pass string) map[string]interface{} {
	ob := map[string]interface{}{
		"type": "socks", "tag": "socks-out",
		"server": host, "server_port": port, "version": "5",
	}
	if user != "" {
		ob["username"] = user
		ob["password"] = pass
	}
	return ob
}

func buildTunConfig(host string, port int, user, pass string) map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"inbounds": []map[string]interface{}{{
			"type": "tun", "interface_name": "sb-tun",
			"address": []string{"172.19.0.1/30"}, "mtu": 9000,
			"auto_route": true, "strict_route": false, "stack": "system",
		}},
		"outbounds": []map[string]interface{}{socksOutbound(host, port, user, pass)},
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			// sniff used to live in the tun inbound; sing-box 1.13 moved it to
			// a route action. The core we ship is built >= 1.13.
			"rules": []map[string]interface{}{{"action": "sniff"}},
			"final": "socks-out",
		},
	}
}

// buildProxyConfig is the non-TUN mode: one local mixed inbound (SOCKS5 + HTTP),
// no interface, no routes, no admin rights.
func buildProxyConfig(host string, port int, user, pass string, localPort int) map[string]interface{} {
	return map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"inbounds": []map[string]interface{}{{
			"type": "mixed", "tag": "mixed-in",
			"listen": "127.0.0.1", "listen_port": localPort,
		}},
		"outbounds": []map[string]interface{}{socksOutbound(host, port, user, pass)},
	}
}

// findSingBox locates the core executable: next to the app (installer), in the
// repo checkout (desktop/embed, put there by scripts/fetch-core.sh) or in the
// runtime dir. SINGBOX_BIN overrides everything (CI and dev).
func findSingBox(a *App) (string, error) {
	if override := os.Getenv("SINGBOX_BIN"); override != "" {
		return override, nil
	}
	for _, candidate := range []string{
		filepath.Join(a.appDir, "sing-box.exe"),
		filepath.Join(a.appDir, "embed", "sing-box.exe"),
		filepath.Join(a.runDir, "sing-box.exe"),
	} {
		if st, err := os.Stat(candidate); err == nil && st.Size() > 1024 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("sing-box.exe tidak ditemukan - instal ulang atau jalankan desktop/scripts/fetch-core.sh")
}

func (a *App) startCore(mode, host string, port int, user, pass string, localPort int) string {
	binPath, err := findSingBox(a)
	if err != nil {
		return "Core tidak ditemukan: " + err.Error()
	}

	var config map[string]interface{}
	proxyAddr := ""
	if mode == modeProxy {
		config = buildProxyConfig(host, port, user, pass, localPort)
		proxyAddr = "127.0.0.1:" + strconv.Itoa(localPort)
	} else {
		config = buildTunConfig(host, port, user, pass)
	}

	configPath := filepath.Join(a.runDir, "config.json")
	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "Write config failed: " + err.Error()
	}

	if mode == modeProxy && a.settings.SystemProxy {
		prev, err := applySystemProxy(proxyAddr)
		if err != nil {
			return "Set system proxy failed: " + err.Error()
		}
		a.settings.ProxyBackup = &prev
		a.saveSettings()
	}

	logPath := filepath.Join(a.runDir, "sing-box.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return "Open log failed: " + err.Error()
	}

	cmd := exec.Command(binPath, "run", "-c", configPath, "-D", a.runDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		a.restoreSystemProxy()
		return "Start failed: " + err.Error()
	}

	a.mu.Lock()
	a.process = cmd
	a.mu.Unlock()

	started := time.Now()
	go func() {
		cmd.Wait()
		logFile.Close()
		a.mw.Synchronize(func() {
			if !a.connected {
				return
			}
			// Died right after start while in TUN mode: wintun missing, driver
			// blocked, route conflict. Offer the proxy mode instead of a dead end.
			if mode == modeTun && time.Since(started) < 5*time.Second {
				a.handleTunFailure(logPath)
				return
			}
			a.statusLabel.SetText("Status: Disconnected (sing-box process exited)")
			a.doDisconnect()
		})
	}()
	return ""
}

func (a *App) handleTunFailure(logPath string) {
	a.connected = false
	a.connectBtn.SetEnabled(true)
	a.disconnBtn.SetEnabled(false)
	a.killProcess()
	a.statusLabel.SetText("Status: TUN gagal start - coba mode Proxy")

	detail := tailFile(logPath, 400)
	if detail != "" {
		detail = "\n\nLog sing-box:\n" + detail
	}
	answer := walk.MsgBox(a.mw, "TUN gagal start",
		"sing-box keluar tepat setelah start di mode TUN."+detail+
			"\n\nPindah ke mode Proxy (tanpa admin) dan connect ulang?",
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning)
	if answer == walk.DlgCmdYes {
		a.setMode(modeProxy)
		a.doConnect()
	}
}

func tailFile(path string, max int) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > max {
		data = data[len(data)-max:]
	}
	return strings.TrimSpace(string(data))
}

func (a *App) doDisconnect() {
	a.killProcess()
	a.statusLabel.SetText("Status: Disconnected")
	a.connectBtn.SetEnabled(true)
	a.disconnBtn.SetEnabled(false)
}

// restoreSystemProxy puts the WinINet settings back exactly as we found them.
func (a *App) restoreSystemProxy() {
	if a.settings.ProxyBackup == nil || !a.settings.ProxyBackup.Valid {
		return
	}
	writeSystemProxy(*a.settings.ProxyBackup)
	a.settings.ProxyBackup = nil
	a.saveSettings()
}

func (a *App) killProcess() {
	a.restoreSystemProxy()

	a.mu.Lock()
	if a.process == nil || a.process.Process == nil {
		a.mu.Unlock()
		a.connected = false
		return
	}
	pid := a.process.Process.Pid
	a.process = nil
	a.mu.Unlock()

	// Kill entire process tree (taskkill cmd must be hidden too)
	taskKill := exec.Command("taskkill.exe", "/F", "/T", "/PID", strconv.Itoa(pid))
	taskKill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	taskKill.Run()
	a.connected = false
}

// relaunchElevated restarts the app with a UAC prompt for mode TUN. The lock
// file must go first or the new instance would see itself as a duplicate.
func (a *App) relaunchElevated() {
	a.killProcess()
	a.saveSettings()
	systray.Quit()
	os.Remove(filepath.Join(a.runDir, lockFileName))
	if err := relaunchAsAdmin(); err != nil {
		walk.MsgBox(a.mw, "Gagal",
			"Tidak bisa menjalankan ulang sebagai Administrator:\n"+err.Error()+
				"\n\nPakai mode Proxy (tanpa admin) sebagai gantinya.",
			walk.MsgBoxIconError)
		return
	}
	os.Exit(0)
}

// --- Dialogs ------------------------------------------

func (a *App) showHowTo() {
	walk.MsgBox(a.mw, "Cara Pakai",
		"1. Pastikan HP server menjalankan VPN Hospot\n"+
			"2. Hubungkan PC ke hotspot server\n"+
			"3. Isi Host, Port, User, Pass\n"+
			"4. Pilih mode koneksi:\n"+
			"   - TUN: semua aplikasi lewat tunnel, butuh Administrator\n"+
			"   - Proxy: tanpa admin, proxy sistem diarahkan ke 127.0.0.1 (default 2080)\n"+
			"5. Klik Connect Socks VPN\n\n"+
			"Kalau di laptop ini TUN gagal start, pilih mode Proxy - "+
			"aplikasi otomatis menawarkan pindah mode saat itu terjadi.",
		walk.MsgBoxIconInformation)
}

func (a *App) showDeveloperInfo() {
	info := "Socks Client v" + appVersion + "\n\n" +
		"Developer: JhopanStore\n" +
		"Platform: Windows Desktop\n" +
		"Core: sing-box v1.12.2\n\n" +
		"Hubungi developer atau dukung pengembangan aplikasi:\n\n" +
		"Telegram: @jhopan_05\n" +
		"Website: jhopanstore.my.id\n" +
		"Trakteer: trakteer.id/jhopan"

	walk.MsgBox(a.mw, "Info Developer", info, walk.MsgBoxIconInformation)
}
