package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"socks-client-desktop/internal/boxcfg"
)

const logLines = 400

// Options adalah parameter koneksi dari form GUI.
type Options struct {
	Host  string
	Port  int
	User  string
	Pass  string
	MTU   int
	Stack string
}

// Engine menjalankan dan mengawasi proses core sing-box, plus loop HTTP ping.
type Engine struct {
	core    string
	dataDir string

	mu        sync.Mutex
	cmd       *exec.Cmd
	running   bool
	status    string
	logBuffer []string
	opts      Options

	pingOn     bool
	pingCancel func()
	lastPing   string
}

// New membuat engine untuk binary core tertentu; dataDir menyimpan config.json
// (0600, memuat kredensial) dan tidak boleh di direktori bersama.
func New(core, dataDir string) *Engine {
	return &Engine{core: core, dataDir: dataDir, status: "Disconnected"}
}

// DataDir adalah direktori kerja engine (config.json + log core).
func (e *Engine) DataDir() string { return e.dataDir }

// Running melaporkan apakah proses core masih hidup.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Status mengembalikan satu baris untuk label status di GUI.
func (e *Engine) Status() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return e.status
	}
	text := fmt.Sprintf("Connected %s:%d", e.opts.Host, e.opts.Port)
	if e.lastPing != "" {
		text += "  -  " + e.lastPing
	}
	return text
}

// LogTail mengembalikan n baris terakhir keluaran core ("Diagnosa" sederhana).
func (e *Engine) LogTail(n int) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n <= 0 || n > len(e.logBuffer) {
		n = len(e.logBuffer)
	}
	return strings.Join(e.logBuffer[len(e.logBuffer)-n:], "\n")
}

func (e *Engine) appendLog(line string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logBuffer = append(e.logBuffer, line)
	if len(e.logBuffer) > logLines {
		e.logBuffer = e.logBuffer[len(e.logBuffer)-logLines:]
	}
}

func (e *Engine) setStatus(s string) {
	e.mu.Lock()
	e.status = s
	e.mu.Unlock()
}

// Start menyusun config lewat boxcfg (sumber yang sama dengan klien lain),
// menuliskannya 0600, lalu menjalankan core dan mengawasi keluarannya.
func (e *Engine) Start(o Options) error {
	if e.Running() {
		return nil
	}
	if o.Port < 1 || o.Port > 65535 {
		return fmt.Errorf("port tidak valid: %d", o.Port)
	}
	if o.Stack == "" {
		o.Stack = boxcfg.StackGVisor
	}
	if err := os.MkdirAll(e.dataDir, 0o700); err != nil {
		return err
	}

	tunOpts := boxcfg.TunOptions{Stack: o.Stack, MTU: o.MTU, LogLevel: "info"}
	if runtime.GOOS == "darwin" {
		// macOS hanya mengizinkan utunN: biarkan core memilih namanya.
		tunOpts.AutoInterfaceName = true
	} else {
		tunOpts.InterfaceName = boxcfg.DefaultInterfaceName(runtime.GOOS)
	}
	config := boxcfg.Tun(o.Host, o.Port, o.User, o.Pass, tunOpts)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	configPath := filepath.Join(e.dataDir, "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return err
	}

	cmd := exec.Command(e.core, "run", "-c", configPath, "-D", e.dataDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	spawnHidden(cmd) // Windows: jangan munculkan console sing-box
	if err := cmd.Start(); err != nil {
		// Jangan tinggalkan config berisi kredensial kalau core gagal dijalankan.
		os.Remove(configPath)
		return fmt.Errorf("gagal menjalankan core: %w", err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.running = true
	e.opts = o
	e.status = "Connected"
	e.logBuffer = nil
	e.mu.Unlock()

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
		for scanner.Scan() {
			e.appendLog(scanner.Text())
		}
	}()

	go func() {
		err := cmd.Wait()
		os.Remove(configPath)
		e.stopPing()
		e.mu.Lock()
		e.running = false
		e.cmd = nil
		e.lastPing = ""
		if err != nil {
			e.status = "Core berhenti - lihat log (Diagnosa)"
		} else {
			e.status = "Disconnected"
		}
		e.mu.Unlock()
	}()

	e.syncPing()
	return nil
}

// Stop menghentikan core (SIGTERM, lalu SIGKILL bila tidak mau berhenti).
func (e *Engine) Stop() {
	e.mu.Lock()
	cmd := e.cmd
	e.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	terminate(cmd.Process.Pid)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !e.Running() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	cmd.Process.Kill()
}
