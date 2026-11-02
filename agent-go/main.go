// RemotePC agent (bản Go): một file .exe duy nhất, không cần cài Python.
// Dùng chung giao thức và web app với agent/server.py.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
)

//go:embed web
var webFS embed.FS

// ------------------------------------------------------------------ auth --
type Auth struct {
	mu     sync.Mutex
	path   string
	tokens []string
	pin    string
	fails  map[string]*failInfo

	totalFails  int
	OnPinChange func()
}
type failInfo struct {
	n     int
	until time.Time
}

func newAuth(reset bool) *Auth {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".remotepc")
	os.MkdirAll(dir, 0o700)
	a := &Auth{path: filepath.Join(dir, "tokens.json"), fails: map[string]*failInfo{}}
	if !reset {
		if b, err := os.ReadFile(a.path); err == nil {
			json.Unmarshal(b, &a.tokens)
		}
	}
	a.save()
	// PIN được lưu lại để bật lại máy vẫn dùng mã cũ (mã QR cũng giữ nguyên nếu IP không đổi).
	if b, err := os.ReadFile(a.pinPath()); err == nil && !reset && len(strings.TrimSpace(string(b))) == 6 {
		a.pin = strings.TrimSpace(string(b))
	} else {
		a.NewPIN()
	}
	return a
}
func (a *Auth) save() {
	if a.tokens == nil {
		a.tokens = []string{}
	}
	b, _ := json.Marshal(a.tokens)
	os.WriteFile(a.path, b, 0o600)
}
func (a *Auth) PIN() string     { a.mu.Lock(); defer a.mu.Unlock(); return a.pin }
func (a *Auth) pinPath() string { return filepath.Join(filepath.Dir(a.path), "pin.txt") }
func (a *Auth) setRandomPIN() { // gọi khi đang giữ khoá
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	a.pin = fmt.Sprintf("%06d", n.Int64())
	os.WriteFile(a.pinPath(), []byte(a.pin), 0o600)
}
func (a *Auth) NewPIN() {
	a.mu.Lock()
	a.setRandomPIN()
	a.mu.Unlock()
}
func (a *Auth) ClearTokens() { a.mu.Lock(); a.tokens = nil; a.save(); a.mu.Unlock() }
func (a *Auth) Count() int   { a.mu.Lock(); defer a.mu.Unlock(); return len(a.tokens) }

func (a *Auth) ok(tok string) bool {
	if tok == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, t := range a.tokens {
		if subtle.ConstantTimeCompare([]byte(t), []byte(tok)) == 1 {
			return true
		}
	}
	return false
}
func (a *Auth) pair(ip, pin string) (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.fails[ip]
	if f != nil && time.Now().Before(f.until) {
		return "", fmt.Sprintf("Sai quá nhiều lần, thử lại sau %d giây", int(time.Until(f.until).Seconds())+1)
	}
	if subtle.ConstantTimeCompare([]byte(pin), []byte(a.pin)) != 1 {
		if f == nil {
			f = &failInfo{}
			a.fails[ip] = f
		}
		f.n++
		a.totalFails++
		if a.totalFails >= 20 { // bị dò PIN từ nhiều nơi: đổi PIN mới
			a.totalFails = 0
			a.setRandomPIN()
			if a.OnPinChange != nil {
				go a.OnPinChange()
			}
		}
		if f.n >= 5 {
			f.until = time.Now().Add(60 * time.Second)
			f.n = 0
		}
		return "", "Mã PIN không đúng"
	}
	delete(a.fails, ip)
	b := make([]byte, 32)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	a.tokens = append(a.tokens, tok)
	a.save()
	return tok, ""
}

// --------------------------------------------------------------- capture --
type Monitor struct{ Left, Top, Width, Height int }

func monitors() []Monitor {
	n := screenshot.NumActiveDisplays()
	out := make([]Monitor, 0, n)
	for i := 0; i < n; i++ {
		r := screenshot.GetDisplayBounds(i)
		out = append(out, Monitor{r.Min.X, r.Min.Y, r.Dx(), r.Dy()})
	}
	return out
}

func grabJPEG(m Monitor, maxW, q int) ([]byte, error) {
	img, err := screenshot.CaptureRect(image.Rect(m.Left, m.Top, m.Left+m.Width, m.Top+m.Height))
	if err != nil {
		return nil, err
	}
	var src image.Image = img
	if img.Bounds().Dx() > maxW {
		h := img.Bounds().Dy() * maxW / img.Bounds().Dx()
		dst := image.NewRGBA(image.Rect(0, 0, maxW, h))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		src = dst
	}
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, src, &jpeg.Options{Quality: q})
	return buf.Bytes(), err
}

// ----------------------------------------------------------------- server --
var upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 1 << 16,
	CheckOrigin: func(r *http.Request) bool { return true }}

type msg map[string]any

func (m msg) str(k string) string { s, _ := m[k].(string); return s }
func (m msg) num(k string, def float64) float64 {
	if v, ok := m[k].(float64); ok {
		return v
	}
	return def
}

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

func wsHandler(auth *Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.ok(r.URL.Query().Get("token")) {
			http.Error(w, "token không hợp lệ", http.StatusUnauthorized)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		activeConns.Add(1)
		defer activeConns.Add(-1)
		held := map[string]bool{} // phím/nút đang giữ, thả hết khi điện thoại ngắt kết nối
		defer func() {
			if !inputAvailable {
				return
			}
			for k, on := range held {
				if on && strings.HasPrefix(k, "b:") {
					handleInput(msg{"t": "up", "b": k[2:]}, Monitor{})
				} else if on {
					handleInput(msg{"t": "kup", "k": k[2:]}, Monitor{})
				}
			}
		}()
		c.SetReadLimit(1 << 20)

		var mu sync.Mutex // gorilla: chỉ một goroutine được ghi
		send := func(mt int, b []byte) error {
			mu.Lock()
			defer mu.Unlock()
			c.SetWriteDeadline(time.Now().Add(10 * time.Second))
			return c.WriteMessage(mt, b)
		}
		sendJSON := func(v any) error { b, _ := json.Marshal(v); return send(websocket.TextMessage, b) }

		mons := monitors()
		if len(mons) == 0 {
			sendJSON(msg{"t": "err", "msg": "không tìm thấy màn hình"})
			return
		}
		var st struct {
			sync.Mutex
			mon, maxW, q, fps int
			stream            bool
		}
		st.mon, st.maxW, st.q, st.fps, st.stream = 1, 1280, 60, 15, true
		cur := func() Monitor {
			st.Lock()
			defer st.Unlock()
			if st.mon == 0 && len(mons) > 1 { // tất cả màn hình ghép lại
				u := image.Rect(mons[0].Left, mons[0].Top, mons[0].Left+mons[0].Width, mons[0].Top+mons[0].Height)
				for _, m := range mons[1:] {
					u = u.Union(image.Rect(m.Left, m.Top, m.Left+m.Width, m.Top+m.Height))
				}
				return Monitor{u.Min.X, u.Min.Y, u.Dx(), u.Dy()}
			}
			if st.mon-1 < len(mons) && st.mon >= 1 {
				return mons[st.mon-1]
			}
			return mons[0]
		}
		host, _ := os.Hostname()
		list := []msg{}
		for i, m := range mons {
			list = append(list, msg{"i": i + 1, "w": m.Width, "h": m.Height})
		}
		osName := map[string]string{"windows": "Windows", "darwin": "Darwin", "linux": "Linux"}[runtime.GOOS]
		sendJSON(msg{"t": "hello", "host": host, "os": osName, "input": inputAvailable, "monitors": list, "version": Version})

		ack := make(chan struct{}, 1)
		ack <- struct{}{}
		poke := func() {
			select {
			case ack <- struct{}{}:
			default:
			}
		}
		done := make(chan struct{})
		defer close(done)

		go func() { // luồng gửi hình
			var last []byte
			for {
				select {
				case <-done:
					return
				case <-ack:
				}
				t0 := time.Now()
				st.Lock()
				stream, maxW, q, fps, mon := st.stream, st.maxW, st.q, st.fps, st.mon
				st.Unlock()
				gotAck := true
				if stream {
					m := cur()
					meta := msg{"t": "frame", "mon": mon, "w": m.Width, "h": m.Height}
					if x, y, ok := cursorPos(); ok {
						meta["cx"] = float64(x-m.Left) / float64(m.Width)
						meta["cy"] = float64(y-m.Top) / float64(m.Height)
					}
					jpg, err := grabJPEG(m, maxW, q)
					if err == nil && !bytes.Equal(jpg, last) {
						if sendJSON(meta) != nil || send(websocket.BinaryMessage, jpg) != nil {
							return
						}
						last = jpg
						gotAck = false // chờ điện thoại xác nhận
					} else {
						meta["same"] = true
						if sendJSON(meta) != nil {
							return
						}
					}
				}
				time.Sleep(time.Second/time.Duration(fps) - time.Since(t0))
				if gotAck {
					poke()
				}
			}
		}()

		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			var m msg
			if json.Unmarshal(data, &m) != nil {
				continue
			}
			switch m.str("t") {
			case "ack":
				poke()
			case "cfg":
				st.Lock()
				if v, ok := m["mon"].(float64); ok {
					st.mon = clamp(int(v), 0, 16)
				}
				if v, ok := m["max_w"].(float64); ok {
					st.maxW = clamp(int(v), 320, 3840)
				}
				if v, ok := m["q"].(float64); ok {
					st.q = clamp(int(v), 20, 95)
				}
				if v, ok := m["fps"].(float64); ok {
					st.fps = clamp(int(v), 1, 30)
				}
				if v, ok := m["stream"].(bool); ok {
					st.stream = v
				}
				st.Unlock()
				if _, ok := m["mon"]; ok && inputAvailable {
					ensureOn(cur())
				}
				poke()
			default:
				switch t := m.str("t"); t {
				case "down", "up":
					held["b:"+m.str("b")] = t == "down"
				case "kdown", "kup":
					held["k:"+m.str("k")] = t == "kdown"
				}
				if inputAvailable {
					if err := handleInput(m, cur()); err != nil {
						sendJSON(msg{"t": "err", "msg": err.Error()})
					}
				}
			}
		}
	}
}

func clientIP(r *http.Request) string {
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	// Qua đường truyền Internet, yêu cầu đến từ cloudflared trên chính máy này.
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		if cf := r.Header.Get("Cf-Connecting-Ip"); cf != "" {
			return cf
		}
	}
	return h
}

func lanIPs() []string {
	var ips []string
	if c, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		ips = append(ips, c.LocalAddr().(*net.UDPAddr).IP.String())
		c.Close()
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			s := n.IP.String()
			if len(ips) == 0 || ips[0] != s {
				ips = append(ips, s)
			}
		}
	}
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	return ips
}

// App là trạng thái chung cho giao diện (cửa sổ trên Windows, dòng lệnh trên hệ khác).
type App struct {
	Auth   *Auth
	URLs   []string
	Port   int
	Tunnel *Tunnel

	Background bool // chạy ngầm (khởi động cùng Windows): không hiện cửa sổ
}

func (a *App) QRURL() string { return fmt.Sprintf("%s/?pin=%s", a.URLs[0], a.Auth.PIN()) }

// Version: tăng mỗi lần phát hành, xem CHANGELOG.md.
var Version = "0.5.1"

var activeConns atomic.Int32

func main() {
	port := flag.Int("port", 8765, "cổng")
	reset := flag.Bool("reset", false, "xoá các điện thoại đã ghép nối")
	flag.Bool("no-browser", false, "(giữ để tương thích, không còn dùng)")
	background := flag.Bool("background", false, "chạy ngầm, không hiện cửa sổ")
	flag.Parse()
	ensureSingleInstance()

	initPlatform()
	auth := newAuth(*reset)
	ips := lanIPs()
	urls := make([]string, len(ips))
	for i, ip := range ips {
		urls[i] = fmt.Sprintf("http://%s:%d", ip, *port)
	}
	app := &App{Auth: auth, URLs: urls, Port: *port, Tunnel: &Tunnel{}, Background: *background}

	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(auth))
	mux.HandleFunc("/audio", audioHandler(auth))
	mux.HandleFunc("/api/pair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "", 405)
			return
		}
		var body struct {
			Pin string `json:"pin"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
		tok, e := auth.pair(clientIP(r), body.Pin)
		w.Header().Set("Content-Type", "application/json")
		if e != "" {
			w.WriteHeader(403)
			json.NewEncoder(w).Encode(msg{"error": e})
			return
		}
		log.Printf("[+] Đã ghép nối thiết bị mới từ %s", clientIP(r))
		json.NewEncoder(w).Encode(msg{"token": tok})
	})
	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		ok := auth.ok(r.URL.Query().Get("token"))
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(401)
		}
		json.NewEncoder(w).Encode(msg{"ok": ok})
	})
	mux.Handle("/", files)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		fatalUI(fmt.Sprintf("Không mở được cổng %d.\nCó thể RemotePC đang chạy rồi.\n\n%v", *port, err))
		os.Exit(1)
	}
	go func() { log.Println(http.Serve(ln, handler)) }()
	runUI(app)
}
