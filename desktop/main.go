package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net"
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

	"socks-client-desktop/internal/boxcfg"
)

// The core is deliberately NOT embedded: it is built by the "Build Core"
// workflow and fetched with desktop/scripts/fetch-core.sh, so the repo and the
// binary stay small.
//
//go:embed embed/app.ico embed/logo_store.png
var embeddedFiles embed.FS

const (
	// maxRestarts bounds the watchdog: a link that keeps flapping should end in
	// a clear status message, not an endless restart loop.
	maxRestarts = 4

	appName      = "Socks Client Desktop"
	appVersion   = "1.3.0"
	lockFileName = "socks_client_desktop.lock"
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

	watchStop chan struct{}
	restarts  int

	passwordNotice   string
	migratedPassword bool
}

type Settings struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	// Pass tidak pernah ditulis lagi: saveSettings menulis PassEnc (DPAPI).
	// Field ini hanya dibaca sekali untuk memigrasi settings.json lama.
	Pass    string `json:"pass,omitempty"`
	PassEnc string `json:"pass_enc,omitempty"`
	Tray    bool   `json:"tray"`
}

// runtimeDir keeps the writable bits (settings, config, sing-box.exe, log) out
// of Program Files: the install directory belongs to the installer, and a config
// written next to the exe would be lost on upgrade.
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
	a.settings = Settings{Port: 1080, Tray: true}

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

	if a.settings.PassEnc != "" {
		plain, err := unprotectPassword(a.settings.PassEnc)
		if err != nil {
			// Blob DPAPI terikat akun/mesin: file dari mesin lain tidak bisa dibuka.
			a.settings.Pass = ""
			a.settings.PassEnc = ""
			a.passwordNotice = err.Error()
		} else {
			a.settings.Pass = plain
		}
	} else if a.settings.Pass != "" {
		a.migratedPassword = true // plaintext lama: ditulis ulang terenkripsi di save berikutnya
	}

	a.normalizeSettings()
	a.trayEnabled = a.settings.Tray
}

func (a *App) normalizeSettings() {
	if a.settings.Port < 1 || a.settings.Port > 65535 {
		a.settings.Port = 1080
	}
}

func (a *App) saveSettings() {
	// S3: password keluar ke disk hanya dalam bentuk blob DPAPI.
	toWrite := a.settings
	toWrite.PassEnc = protectPassword(a.settings.Pass)
	toWrite.Pass = ""
	data, _ := json.MarshalIndent(toWrite, "", "  ")
	os.WriteFile(filepath.Join(a.runDir, "settings.json"), data, 0600)
}

// --- UI -----------------------------------------------

func (a *App) runUI() {
	var hostEdit, portEdit, userEdit, passEdit *walk.LineEdit
	var trayCB *walk.CheckBox
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
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 4, Right: 15, Bottom: 4}}, Children: []Widget{
				CheckBox{AssignTo: &trayCB, Text: "Minimize to tray when closed", Checked: a.settings.Tray, Font: Font{Family: "Segoe UI", PointSize: 9},
					OnCheckedChanged: func() { a.trayEnabled = trayCB.Checked() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 6, Right: 15, Bottom: 5}, Spacing: 6}, Children: []Widget{
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
				PushButton{Text: "Diagnosa", Font: Font{Family: "Segoe UI", PointSize: 9}, OnClicked: func() { a.showDiagnostics() }},
				PushButton{Text: "Info Developer", Font: Font{Family: "Segoe UI", PointSize: 9}, OnClicked: func() { a.showDeveloperInfo() }},
			}},
			Composite{Layout: VBox{Margins: Margins{Left: 15, Top: 6, Right: 15, Bottom: 10}}, Children: []Widget{
				Label{AssignTo: &statusLabel, Text: "Status: Disconnected", Font: Font{Family: "Segoe UI", PointSize: 9}, Alignment: AlignHCenterVCenter},
			}},
		},
	}.Create()

	a.hostEdit, a.portEdit, a.userEdit, a.passEdit = hostEdit, portEdit, userEdit, passEdit
	a.connectBtn, a.disconnBtn = connectBtn, disconnBtn
	a.statusLabel = statusLabel
	a.trayCB = trayCB

	// S3: tulis ulang settings lama (password plaintext) dalam bentuk terenkripsi,
	// dan beri tahu kalau blob DPAPI tidak bisa dibuka (file dari akun lain).
	if a.migratedPassword {
		a.migratedPassword = false
		a.saveSettings()
	}
	if a.passwordNotice != "" {
		a.statusLabel.SetText("Status: password perlu diketik ulang (" + a.passwordNotice + ")")
		a.passwordNotice = ""
	}

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

func (a *App) exitApp() {
	a.stopWatchdog()
	a.killProcess()
	systray.Quit()
	a.trayEnabled = false
	a.mw.Close()
	os.Exit(0)
}

// --- Connect / Disconnect -----------------------------

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

func (a *App) startTray() {
	if a.trayStarted {
		return
	}
	a.trayStarted = true

	systray.Run(func() {
		icoData, _ := os.ReadFile(trayIconPath)
		systray.SetIcon(icoData)
		systray.SetTitle(appName)
		systray.SetTooltip(appName + " v" + appVersion + "\nby JhopanStore\nMode: TUN (gvisor)")

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
	if net.ParseIP(host) == nil {
		// A hostname would make sing-box bootstrap-resolve it through the OS
		// resolver, which in TUN mode goes back into our own tunnel - or worse,
		// leaks the query before the tunnel is up. IP only, as agreed.
		walk.MsgBox(a.mw, "Host harus IP",
			"Masukkan alamat IP server SOCKS, contoh 10.12.132.225.\n\n"+
				"Hostname tidak didukung karena resolusi bootstrap-nya terjadi di luar tunnel.",
			walk.MsgBoxIconWarning)
		return
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		walk.MsgBox(a.mw, "Error", "Invalid port", walk.MsgBoxIconWarning)
		return
	}

	if a.connected {
		return
	}

	// TUN is the only mode here, and it needs Administrator.
	if !isAdmin() {
		answer := walk.MsgBox(a.mw, "TUN butuh Administrator",
			"TUN memerlukan hak Administrator.\n\n"+
				"Yes = jalankan ulang sebagai Administrator\n"+
				"No  = batal",
			walk.MsgBoxYesNo|walk.MsgBoxIconQuestion)
		if answer == walk.DlgCmdYes {
			a.relaunchElevated()
		}
		return
	}

	a.settings = Settings{
		Host: host, Port: portNum, User: user, Pass: pass,
		Tray: a.trayCB.Checked(),
	}
	a.saveSettings()
	a.statusLabel.SetText("Status: Connecting...")
	a.connectBtn.SetEnabled(false)

	go func() {
		errMsg := a.startCore(host, portNum, user, pass)
		a.mw.Synchronize(func() {
			if errMsg != "" {
				a.statusLabel.SetText("Status: " + errMsg)
				a.connectBtn.SetEnabled(true)
				return
			}
			a.connected = true
			a.mu.Lock()
			a.restarts = 0
			a.mu.Unlock()
			a.statusLabel.SetText(a.connectedStatus(host, portNum))
			a.connectBtn.SetEnabled(false)
			a.disconnBtn.SetEnabled(true)
			a.startWatchdog(host)
		})
	}()
}

func (a *App) connectedStatus(host string, port int) string {
	return fmt.Sprintf("Status: Connected (TUN, gvisor) %s:%d", host, port)
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

func (a *App) startCore(host string, port int, user, pass string) string {
	binPath, err := findSingBox(a)
	if err != nil {
		return "Core tidak ditemukan: " + err.Error()
	}

	config := boxcfg.Tun(host, port, user, pass, boxcfg.TunOptions{Stack: boxcfg.StackGVisor})

	configPath := filepath.Join(a.runDir, "config.json")
	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "Write config failed: " + err.Error()
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
			// Died right after start: wintun missing, driver blocked by AV,
			// route conflict. handleTunFailure names the cause and the cures.
			if time.Since(started) < 5*time.Second {
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
	a.statusLabel.SetText("Status: TUN gagal start")

	detail := tailFile(logPath, 400)
	if detail != "" {
		detail = "\n\nLog sing-box:\n" + detail
	}
	walk.MsgBox(a.mw, "TUN gagal start",
		"sing-box keluar tepat setelah start.\n\nYang bisa dicek:\n"+
			"  1. jalankan sebagai Administrator\n"+
			"  2. tambahkan folder app + %LOCALAPPDATA%\\SocksClientDesktop ke exclusion antivirus\n"+
			"  3. tekan Diagnosa untuk melihat log lengkap"+detail,
		walk.MsgBoxIconWarning)
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

// watchNetwork restarts the core when the machine changes networks.
//
// The hotspot can drop and come back on a different interface while sing-box
// keeps running against a dead route - the classic "still says Connected but
// nothing loads". GetBestInterfaceEx answers without sending a single packet, so
// the check works even under strict_route (where a probe from this process would
// be blocked by the firewall).
//
// probe and onChange are parameters so the restart path is testable without a
// GUI or a real network change.
func watchNetwork(host string, interval time.Duration, probe func(net.IP) (uint32, error), stop <-chan struct{}, onChange func(string)) {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		// Hostname or IPv6: no interface index to compare, the watchdog stays out
		// of the way. Hostname hosts are rejected before connecting anyway.
		return
	}
	startIndex, err := probe(ip)
	if err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		index, err := probe(ip)
		if err != nil {
			continue
		}
		if index != startIndex {
			onChange(fmt.Sprintf("interface %d -> %d", startIndex, index))
			return
		}
	}
}

const watchdogInterval = 8 * time.Second

func (a *App) startWatchdog(host string) {
	a.stopWatchdog()
	stop := make(chan struct{})
	a.mu.Lock()
	a.watchStop = stop
	a.mu.Unlock()

	go watchNetwork(host, watchdogInterval, bestInterfaceIndex, stop, func(reason string) {
		a.mw.Synchronize(func() {
			a.statusLabel.SetText("Status: jaringan berubah (" + reason + "), menyambung ulang...")
		})
		a.restartCore()
	})
}

func (a *App) stopWatchdog() {
	a.mu.Lock()
	stop := a.watchStop
	a.watchStop = nil
	a.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// restartCore re-establishes the tunnel after the network moved. Guarded: a
// flapping link must not turn into an endless restart loop.
func (a *App) restartCore() {
	a.mu.Lock()
	if a.restarts >= maxRestarts {
		a.mu.Unlock()
		a.mw.Synchronize(func() {
			a.statusLabel.SetText("Status: jaringan terus berubah - tekan Disconnect lalu Connect lagi")
		})
		return
	}
	a.restarts++
	a.mu.Unlock()

	a.killProcess()
	time.Sleep(1200 * time.Millisecond)
	a.mu.Lock()
	host, port, user, pass := a.settings.Host, a.settings.Port, a.settings.User, a.settings.Pass
	a.mu.Unlock()

	if errMsg := a.startCore(host, port, user, pass); errMsg != "" {
		a.mw.Synchronize(func() {
			a.statusLabel.SetText("Status: " + errMsg)
			a.connectBtn.SetEnabled(true)
		})
		return
	}
	a.mw.Synchronize(func() {
		a.connected = true
		a.statusLabel.SetText(a.connectedStatus(host, port))
	})
}

func (a *App) doDisconnect() {
	a.stopWatchdog()
	a.killProcess()
	a.statusLabel.SetText("Status: Disconnected")
	a.connectBtn.SetEnabled(true)
	a.disconnBtn.SetEnabled(false)
}

func (a *App) killProcess() {

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
				"\n\nJalankan aplikasi manual: klik kanan > Run as administrator.",
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
			"3. Isi Host, Port, User, Pass (sesuai server SOCKS)\n"+
			"4. Klik Connect Socks VPN - app langsung jalan sebagai Administrator kalau belum\n"+
			"5. Semua trafik TCP + UDP + DNS masuk ke tunnel (stack gvisor, MTU 1400)\n\n"+
			"Kalau gagal start:\n"+
			"  - pastikan dijalankan sebagai Administrator\n"+
			"  - tambahkan folder app + %LOCALAPPDATA%\\SocksClientDesktop ke exclusion antivirus\n"+
			"  - tekan Diagnosa untuk melihat penyebabnya\n\n"+
			"Kalau nyambung tapi internet tidak jalan, cek apakah server SOCKS benar-benar\n"+
			"meneruskan trafik (Host/Port/auth), lalu lihat ekor log di Diagnosa.",
		walk.MsgBoxIconInformation)
}

func (a *App) showDiagnostics() {
	admin := isAdmin()

	var b strings.Builder
	fmt.Fprintf(&b, "Mode: TUN (stack gvisor, MTU 1400)\n")
	fmt.Fprintf(&b, "Administrator: %s\n", map[bool]string{true: "ya", false: "tidak"}[admin])
	if a.statusLabel != nil {
		fmt.Fprintf(&b, "%s\n", a.statusLabel.Text())
	}

	if core, err := findSingBox(a); err != nil {
		fmt.Fprintf(&b, "Core: TIDAK DITEMUKAN (%v)\n", err)
	} else {
		fmt.Fprintf(&b, "Core: %s\n", coreVersion(a))
		fmt.Fprintf(&b, "  path: %s\n", core)
	}

	logTail := tailFile(filepath.Join(a.runDir, "sing-box.log"), 600)
	if logTail != "" {
		b.WriteString("\nLog sing-box (ekor):\n" + logTail + "\n")
	}

	b.WriteString("\nSaran:\n" + diagnoseAdvice(admin, logTail))
	walk.MsgBox(a.mw, "Diagnosa", b.String(), walk.MsgBoxIconInformation)
}

// coreVersion reports what the shipped core actually is, so dialogs never show
// a version string that drifts from the binary.
func coreVersion(a *App) string {
	core, err := findSingBox(a)
	if err != nil {
		return "tidak ditemukan"
	}
	cmd := exec.Command(core, "version")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "tidak terbaca"
	}
	return strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
}

func diagnoseAdvice(admin bool, logTail string) string {
	low := strings.ToLower(logTail)
	switch {
	case !admin:
		return "TUN butuh Administrator. Connect akan menawarkan jalan sebagai admin."
	case !strings.Contains(low, "dns: exchanged"):
		return "TUN aktif tapi belum ada DNS yang lewat tunnel. Cek jaringan ke server SOCKS,\n" +
			"lalu lihat ekor log di atas."
	case strings.Contains(low, "wintun") || strings.Contains(low, "access is denied") || strings.Contains(low, "adapter"):
		return "Adapter wintun tidak bisa dibuat - biasanya antivirus/EDR memblokir driver bawaan core.\n" +
			"Tambahkan exclusion untuk folder aplikasi dan %LOCALAPPDATA%\\SocksClientDesktop, restart, coba lagi."
	case strings.Contains(low, "connection refused") || strings.Contains(low, "i/o timeout"):
		return "Server SOCKS tidak menjawab. Cek Host/Port, dan pastikan HP server jalan + PC terhubung hotspotnya."
	default:
		return "Tidak ada masalah terdeteksi."
	}
}

func (a *App) showDeveloperInfo() {
	info := "Socks Client v" + appVersion + "\n\n" +
		"Developer: JhopanStore\n" +
		"Platform: Windows Desktop\n" +
		"Core: " + coreVersion(a) + "\n\n" +
		"Hubungi developer atau dukung pengembangan aplikasi:\n\n" +
		"Telegram: @jhopan_05\n" +
		"Website: jhopanstore.my.id\n" +
		"Trakteer: trakteer.id/jhopan"

	walk.MsgBox(a.mw, "Info Developer", info, walk.MsgBoxIconInformation)
}
