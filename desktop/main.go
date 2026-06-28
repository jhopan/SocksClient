package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"github.com/energye/systray"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func openURL(url string) {
	exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

//go:embed embed/sing-box.exe embed/app.ico embed/logo_store.png
var embeddedFiles embed.FS

const (
	appName    = "Socks Client Desktop"
	appVersion = "1.1.0"
)

var logoPath string
var trayIconPath string

type App struct {
	mu        sync.Mutex
	mw        *walk.MainWindow
	process   *exec.Cmd
	connected bool
	settings  Settings
	appDir    string

	hostEdit    *walk.LineEdit
	portEdit    *walk.LineEdit
	userEdit    *walk.LineEdit
	passEdit    *walk.LineEdit
	connectBtn  *walk.PushButton
	disconnBtn  *walk.PushButton
	statusLabel *walk.Label
	trayCB      *walk.CheckBox

	trayEnabled   bool
	trayStarted   bool
	windowVisible bool
}

type Settings struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	Tray bool   `json:"tray"`
}

func main() {
	app := &App{trayEnabled: true, windowVisible: true}
	app.loadSettings()

	app.appDir, _ = os.Executable()
	app.appDir = filepath.Dir(app.appDir)

	// Single instance check via lock file
	lockPath := filepath.Join(os.TempDir(), "socks_client_desktop.lock")
	if !checkAndLock(lockPath) {
		showExisting()
		return
	}
	defer os.Remove(lockPath)

	// Windows named mutex — Inno Setup AppMutex detects this
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

	// Backup cleanup — if runUI returns (window closed without exitApp)
	app.killProcess()
	os.Remove(filepath.Join(os.TempDir(), "socks_client_desktop.lock"))
	os.Exit(0)
}

// ─── Single Instance ──────────────────────────────────

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

	lockPath := filepath.Join(os.TempDir(), "socks_client_desktop.lock")
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

// ─── Settings ─────────────────────────────────────────

func (a *App) loadSettings() {
	a.settings = Settings{Port: 1080, Tray: true}
	exePath, _ := os.Executable()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exePath), "settings.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &a.settings)
	a.trayEnabled = a.settings.Tray
}

func (a *App) saveSettings() {
	exePath, _ := os.Executable()
	data, _ := json.MarshalIndent(a.settings, "", "  ")
	os.WriteFile(filepath.Join(filepath.Dir(exePath), "settings.json"), data, 0644)
}

// ─── UI ───────────────────────────────────────────────

func (a *App) runUI() {
	var hostEdit, portEdit, userEdit, passEdit *walk.LineEdit
	var connectBtn, disconnBtn *walk.PushButton
	var statusLabel *walk.Label
	var trayCB *walk.CheckBox
	var logoView *walk.ImageView

	a.mw = new(walk.MainWindow)

	MainWindow{
		AssignTo: &a.mw,
		Title:    appName + " v" + appVersion,
		MinSize:  Size{Width: 380, Height: 520},
		Size:     Size{Width: 380, Height: 520},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "E&xit", OnTriggered: func() { a.exitApp() }},
			}},
		},
		Children: []Widget{
			ImageView{AssignTo: &logoView, Mode: ImageViewModeZoom, MaxSize: Size{Width: 300, Height: 120}, Margin: 10},
			Composite{Layout: VBox{Margins: Margins{Left: 10, Right: 10}}, Children: []Widget{
				Label{Text: "🧦 " + appName, Font: Font{Family: "Segoe UI", PointSize: 14, Bold: true}, Alignment: AlignHCenterVCenter},
				Label{Text: "by JhopanStore", Font: Font{Family: "Segoe UI", PointSize: 10, Bold: true, Italic: true}, Alignment: AlignHCenterVCenter},
				Label{Text: "v" + appVersion + " — Powered by sing-box", Font: Font{Family: "Segoe UI", PointSize: 8}, Alignment: AlignHCenterVCenter},
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
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 5, Right: 15, Bottom: 5}}, Children: []Widget{
				CheckBox{AssignTo: &trayCB, Text: "Minimize to tray when closed", Checked: a.settings.Tray, Font: Font{Family: "Segoe UI", PointSize: 9},
					OnCheckedChanged: func() { a.trayEnabled = trayCB.Checked() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 5, Right: 15, Bottom: 5}, Spacing: 6}, Children: []Widget{
				PushButton{AssignTo: &connectBtn, Text: "Connect Socks VPN", Font: Font{Family: "Segoe UI", PointSize: 10, Bold: true},
					OnClicked: func() {
						a.hostEdit, a.portEdit, a.userEdit, a.passEdit = hostEdit, portEdit, userEdit, passEdit
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
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 10, Right: 15, Bottom: 10}}, Children: []Widget{
				Label{AssignTo: &statusLabel, Text: "Status: Disconnected", Font: Font{Family: "Segoe UI", PointSize: 9}, Alignment: AlignHCenterVCenter},
			}},
		},
	}.Create()

	a.hostEdit, a.portEdit, a.userEdit, a.passEdit = hostEdit, portEdit, userEdit, passEdit
	a.connectBtn, a.disconnBtn = connectBtn, disconnBtn
	a.statusLabel = statusLabel
	a.trayCB = trayCB

	// Set window icon (taskbar) from ICO
	if ico, err := walk.NewIconFromFile(trayIconPath); err == nil {
		a.mw.SetIcon(ico)
	}

	// Window close — minimize to tray (if enabled) or exit cleanly
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

// ─── System Tray ──────────────────────────────────────

func (a *App) startTray() {
	if a.trayStarted {
		return
	}
	a.trayStarted = true

	systray.Run(func() {
		icoData, _ := os.ReadFile(trayIconPath)
		systray.SetIcon(icoData)
		systray.SetTitle(appName)
		systray.SetTooltip(appName + " v" + appVersion + "\nby JhopanStore")

		mShow := systray.AddMenuItem("🪟 Show Window", "")
		systray.AddSeparator()
		mConnect := systray.AddMenuItem("🔗 Connect", "")
		mDisconnect := systray.AddMenuItem("⛔ Disconnect", "")
		systray.AddSeparator()
		mExit := systray.AddMenuItem("❌ Exit", "")

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

// ─── Connect / Disconnect ─────────────────────────────

func (a *App) doConnect() {
	host := a.hostEdit.Text()
	port := a.portEdit.Text()
	user := a.userEdit.Text()
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

	a.settings = Settings{Host: host, Port: portNum, User: user, Pass: pass, Tray: a.trayCB.Checked()}
	a.saveSettings()
	a.statusLabel.SetText("Status: Connecting...")
	a.connectBtn.SetEnabled(false)

	go func() {
		errMsg := a.connectVPN(host, port, user, pass)
		a.mw.Synchronize(func() {
			if errMsg != "" {
				a.statusLabel.SetText("Status: " + errMsg)
				a.connectBtn.SetEnabled(true)
			} else {
				a.connected = true
				a.statusLabel.SetText(fmt.Sprintf("Status: Connected ✓ %s:%s", host, port))
				a.connectBtn.SetEnabled(false)
				a.disconnBtn.SetEnabled(true)
			}
		})
	}()
}

func (a *App) connectVPN(host, port, user, pass string) string {
	binPath := filepath.Join(a.appDir, "sing-box.exe")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		data, err := embeddedFiles.ReadFile("embed/sing-box.exe")
		if err != nil {
			return "Extract sing-box failed"
		}
		os.WriteFile(binPath, data, 0755)
	}

	portNum, _ := strconv.Atoi(port)
	config := map[string]interface{}{
		"log": map[string]interface{}{"level": "info"},
		"inbounds": []map[string]interface{}{{
			"type": "tun", "interface_name": "sb-tun",
			"address": []string{"172.19.0.1/30"}, "mtu": 9000,
			"auto_route": true, "strict_route": false, "stack": "system", "sniff": true,
		}},
		"outbounds": []map[string]interface{}{{
			"type": "socks", "tag": "socks-out",
			"server": host, "server_port": portNum,
			"username": user, "password": pass, "version": "5",
		}},
	}

	configPath := filepath.Join(a.appDir, "config.json")
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	logPath := filepath.Join(a.appDir, "sing-box.log")
	logFile, _ := os.Create(logPath)

	cmd := exec.Command(binPath, "run", "-c", configPath, "-D", a.appDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return "Start failed: " + err.Error()
	}
	a.mu.Lock()
	a.process = cmd
	a.mu.Unlock()
	go func() {
		cmd.Wait()
		logFile.Close()
		a.mw.Synchronize(func() {
			if a.connected {
				a.statusLabel.SetText("Status: Disconnected (process died)")
				a.doDisconnect()
			}
		})
	}()
	return ""
}

func (a *App) doDisconnect() {
	a.killProcess()
	a.statusLabel.SetText("Status: Disconnected")
	a.connectBtn.SetEnabled(true)
	a.disconnBtn.SetEnabled(false)
}

func (a *App) killProcess() {
	a.mu.Lock()
	if a.process == nil || a.process.Process == nil {
		a.mu.Unlock()
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

// ─── Dialogs ──────────────────────────────────────────

func (a *App) showHowTo() {
	walk.MsgBox(a.mw, "Cara Pakai",
		"1. Pastikan HP server menjalankan VPN Hospot\n"+
			"2. Hubungkan PC ke hotspot server\n"+
			"3. Isi Host, Port, User, Pass\n"+
			"4. Klik Connect Socks VPN ✓", walk.MsgBoxIconInformation)
}

func (a *App) showDeveloperInfo() {
	// Custom dialog with 3 buttons using MsgBox + virtual key simulation
	// Walk's MsgBox only supports 3 buttons via YesNoCancel
	info := "Socks Client v" + appVersion + "\n\n" +
		"Developer: JhopanStore\n" +
		"Platform: Windows Desktop\n\n" +
		"Hubungi developer atau dukung pengembangan aplikasi:\n\n" +
		"Telegram: @jhopan_05\n" +
		"Website: jhopanstore.my.id\n" +
		"Trakteer: trakteer.id/jhopan"

	walk.MsgBox(a.mw, "Info Developer", info, walk.MsgBoxIconInformation)
}
