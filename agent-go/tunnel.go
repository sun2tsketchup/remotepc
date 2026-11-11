package main

// Truy cập từ xa qua Internet bằng Cloudflare Quick Tunnel: không cần tài khoản, không cần mở cổng
// modem. RemotePC tự tải cloudflared (một lần) rồi tạo đường dẫn https://xxx.trycloudflare.com
// trỏ về server trên máy này. Đường dẫn đổi mỗi lần bật lại.

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

type Tunnel struct {
	mu       sync.Mutex
	state    string // off, downloading, starting, on, error
	url      string
	err      string
	cmd      *exec.Cmd
	OnChange func()
}

var tunnelURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

func (t *Tunnel) set(state, url, err string) {
	t.mu.Lock()
	t.state, t.url, t.err = state, url, err
	t.mu.Unlock()
	if t.OnChange != nil {
		t.OnChange()
	}
}

func (t *Tunnel) Snapshot() (state, url, err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == "" {
		return "off", "", ""
	}
	return t.state, t.url, t.err
}

func cloudflaredPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "RemotePC", name)
}

func cloudflaredURL() string {
	base := "https://github.com/cloudflare/cloudflared/releases/latest/download/"
	switch runtime.GOOS {
	case "windows":
		return base + "cloudflared-windows-amd64.exe"
	case "darwin":
		return "" // macOS: cài bằng `brew install cloudflared`
	}
	return base + "cloudflared-linux-" + runtime.GOARCH
}

func download(url, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0o755)
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("tải cloudflared lỗi HTTP %d", resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, dst)
}

// Start bật đường truyền (gọi nhiều lần không sao).
func (t *Tunnel) Start(port int) {
	t.mu.Lock()
	if t.state == "downloading" || t.state == "starting" || t.state == "on" {
		t.mu.Unlock()
		return
	}
	t.state = "starting"
	t.mu.Unlock()
	go t.run(port)
}

func (t *Tunnel) run(port int) {
	bin := cloudflaredPath()
	if _, err := os.Stat(bin); err != nil {
		if p, err := exec.LookPath("cloudflared"); err == nil {
			bin = p
		} else if u := cloudflaredURL(); u != "" {
			t.set("downloading", "", "")
			if err := download(u, bin); err != nil {
				t.set("error", "", tr("Không tải được cloudflared: ")+err.Error())
				return
			}
		} else {
			t.set("error", "", tr("Hãy cài cloudflared trước (brew install cloudflared)"))
			return
		}
	}
	t.set("starting", "", "")
	cmd := exec.Command(bin, "tunnel", "--no-autoupdate", "--url", fmt.Sprintf("http://127.0.0.1:%d", port))
	hideConsole(cmd)
	stderr, _ := cmd.StderrPipe()
	cmd.Stdout = cmd.Stderr
	if err := cmd.Start(); err != nil {
		t.set("error", "", tr("Không chạy được cloudflared: ")+err.Error())
		return
	}
	t.mu.Lock()
	t.cmd = cmd
	t.mu.Unlock()

	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if u := tunnelURLRe.FindString(sc.Text()); u != "" {
				select {
				case found <- u:
				default:
				}
			}
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case u := <-found:
		t.set("on", u, "")
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		t.set("error", "", tr("Hết thời gian chờ tạo đường truyền, kiểm tra Internet rồi thử lại"))
		return
	case err := <-exited:
		t.set("error", "", fmt.Sprintf(tr("cloudflared đã dừng (%v)"), err))
		return
	}
	err := <-exited
	if s, _, _ := t.Snapshot(); s != "off" {
		t.set("error", "", fmt.Sprintf(tr("Mất đường truyền Internet (%v), bấm để thử lại"), err))
	}
}

func (t *Tunnel) Stop() {
	t.mu.Lock()
	cmd := t.cmd
	t.cmd = nil
	t.mu.Unlock()
	t.set("off", "", "")
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
}
