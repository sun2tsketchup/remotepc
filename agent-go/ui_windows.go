//go:build windows

package main

// Cửa sổ phần mềm RemotePC (Win32 + GDI thuần, không cần trình duyệt hay console).

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/sys/windows"
)

//go:embed web/icon-192.png
var iconPNG []byte

var (
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pFillRect                 = user32.NewProc("FillRect")
	pDrawTextW                = user32.NewProc("DrawTextW")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pSetCursor                = user32.NewProc("SetCursor")
	pSetTimer                 = user32.NewProc("SetTimer")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pAdjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	pGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	pGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	pGlobalLock               = kernel32.NewProc("GlobalLock")
	pGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
	pCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt                   = gdi32.NewProc("BitBlt")
	pDeleteDC                 = gdi32.NewProc("DeleteDC")
	pSelectObject             = gdi32.NewProc("SelectObject")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	pCreatePen                = gdi32.NewProc("CreatePen")
	pRoundRect                = gdi32.NewProc("RoundRect")
	pCreateFontW              = gdi32.NewProc("CreateFontW")
	pSetTextColor             = gdi32.NewProc("SetTextColor")
	pSetBkMode                = gdi32.NewProc("SetBkMode")
	pMoveToEx                 = gdi32.NewProc("MoveToEx")
	pLineTo                   = gdi32.NewProc("LineTo")
	pDwmSetWindowAttribute    = dwmapi.NewProc("DwmSetWindowAttribute")
)

type rect struct{ L, T, R, B int32 }
type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}
type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ X, Y int32 }
	private uint32
}
type paintStruct struct {
	hdc       uintptr
	erase     int32
	rcPaint   rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

const (
	wmDestroy, wmPaint, wmClose, wmEraseBkgnd = 0x0002, 0x000F, 0x0010, 0x0014
	wmSetCursor, wmMouseMove, wmLButtonUp     = 0x0020, 0x0200, 0x0202
	wmTimer, wmSetIcon, wmApp                 = 0x0113, 0x0080, 0x8000
	dtCenter, dtVCenter, dtSingle, dtWord     = 0x1, 0x4, 0x20, 0x10
	dtEndEllipsis, dtNoPrefix                 = 0x8000, 0x800
)

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

var (
	colBg     = rgb(0x0b, 0x12, 0x20)
	colPanel  = rgb(0x12, 0x1b, 0x2e)
	colLine   = rgb(0x22, 0x30, 0x4d)
	colText   = rgb(0xe6, 0xed, 0xf7)
	colMute   = rgb(0x8a, 0x9b, 0xb8)
	colAcc    = rgb(0x38, 0xbd, 0xf8)
	colAccTxt = rgb(0x04, 0x11, 0x1d)
	colOk     = rgb(0x4a, 0xde, 0x80)
	colBad    = rgb(0xf8, 0x71, 0x71)
	colHover  = rgb(0x1d, 0x2a, 0x45)
	colWhite  = rgb(0xff, 0xff, 0xff)
	colBlack  = rgb(0, 0, 0)
)

func wstr(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

// ------------------------------------------------------------------ state --
type button struct {
	id   string
	r    rect // toạ độ logic (chưa nhân DPI)
	text string
}

type ui struct {
	settings *Settings
	icon     uintptr
	app      *App
	hwnd     uintptr
	dpi      int
	mode     string // "lan" | "net"
	hover    string
	buttons  []button
	qrKey    string
	qrBits   [][]bool
	fonts    map[string]uintptr
}

var theUI *ui

const winW, winH = 440, 772

func (u *ui) s(v int32) int32 { return v * int32(u.dpi) / 96 }
func (u *ui) sr(r rect) rect  { return rect{u.s(r.L), u.s(r.T), u.s(r.R), u.s(r.B)} }

func (u *ui) font(name string, pt int, bold bool) uintptr {
	key := fmt.Sprintf("%s/%d/%v", name, pt, bold)
	if f, ok := u.fonts[key]; ok {
		return f
	}
	w := 400
	if bold {
		w = 700
	}
	h := -pt * u.dpi / 72
	f, _, _ := pCreateFontW.Call(uintptr(int32(h)), 0, 0, 0, uintptr(w), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wstr(name))))
	u.fonts[key] = f
	return f
}

func (u *ui) layoutButtons() {
	u.buttons = []button{
		{"lan", rect{60, 82, 218, 116}, "Cùng Wi-Fi"},
		{"net", rect{222, 82, 380, 116}, "Qua Internet"},
		{"auto", rect{24, 590, 416, 620}, "Tự chạy ngầm khi bật Windows"},
		{"nosleep", rect{24, 624, 416, 654}, "Giữ máy tính không ngủ để điện thoại vào bất cứ lúc nào"},
		{"pin", rect{24, 672, 152, 716}, "Đổi mã PIN"},
		{"unpair", rect{156, 672, 284, 716}, "Huỷ ghép nối"},
		{"quit", rect{288, 672, 416, 716}, "Thoát hẳn"},
	}
	if u.mode == "net" {
		st, url, _ := u.app.Tunnel.Snapshot()
		switch st {
		case "on":
			u.buttons = append(u.buttons, button{"copy", rect{100, 540, 220, 574}, "Sao chép link"}, button{"netoff", rect{224, 540, 340, 574}, "Tắt Internet"})
			_ = url
		case "error":
			u.buttons = append(u.buttons, button{"netretry", rect{150, 540, 290, 574}, "Thử lại"})
		}
	}
}

func (u *ui) hit(x, y int32) string {
	for _, b := range u.buttons {
		r := u.sr(b.r)
		if x >= r.L && x < r.R && y >= r.T && y < r.B {
			return b.id
		}
	}
	return ""
}

// ------------------------------------------------------------------ paint --
func fill(dc uintptr, r rect, c uintptr) {
	br, _, _ := pCreateSolidBrush.Call(c)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func (u *ui) round(dc uintptr, r rect, fillC, lineC uintptr, rad int32) {
	br, _, _ := pCreateSolidBrush.Call(fillC)
	pen, _, _ := pCreatePen.Call(0, uintptr(u.s(1)), lineC)
	ob, _, _ := pSelectObject.Call(dc, br)
	op, _, _ := pSelectObject.Call(dc, pen)
	pRoundRect.Call(dc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B), uintptr(u.s(rad)), uintptr(u.s(rad)))
	pSelectObject.Call(dc, ob)
	pSelectObject.Call(dc, op)
	pDeleteObject.Call(br)
	pDeleteObject.Call(pen)
}

func (u *ui) text(dc uintptr, s string, r rect, f uintptr, c uintptr, flags uintptr) {
	pSelectObject.Call(dc, f)
	pSetTextColor.Call(dc, c)
	p, _ := windows.UTF16FromString(s)
	sr := u.sr(r)
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(&sr)), flags|dtNoPrefix)
}

func (u *ui) qr(content string) [][]bool {
	if content != u.qrKey {
		u.qrKey = content
		u.qrBits = nil
		if q, err := qrcode.New(content, qrcode.Medium); err == nil {
			q.DisableBorder = true
			u.qrBits = q.Bitmap()
		}
	}
	return u.qrBits
}

func spacedPIN(p string) string {
	if len(p) == 6 {
		return p[:3] + " " + p[3:]
	}
	return p
}

func (u *ui) paint(dc uintptr) {
	var cr rect
	pGetClientRect.Call(u.hwnd, uintptr(unsafe.Pointer(&cr)))
	fill(dc, cr, colBg)
	pSetBkMode.Call(dc, 1)
	u.layoutButtons()

	// tiêu đề + trạng thái
	u.text(dc, "RemotePC", rect{24, 18, 300, 60}, u.font("Segoe UI", 20, true), colText, dtSingle|dtVCenter)
	u.text(dc, "v"+Version, rect{166, 30, 260, 56}, u.font("Segoe UI", 9, false), colMute, dtSingle|dtVCenter)
	n := activeConns.Load()
	status, sc := "Đang chạy", colOk
	if n > 0 {
		status = fmt.Sprintf("%d thiết bị đang điều khiển", n)
	}
	u.text(dc, "● "+status, rect{200, 18, 416, 60}, u.font("Segoe UI", 10, false), sc, dtSingle|dtVCenter|0x2 /*DT_RIGHT*/)

	// chọn chế độ
	for _, b := range u.buttons[:2] {
		r := u.sr(b.r)
		on := b.id == u.mode
		f, t := colPanel, colText
		if on {
			f, t = colAcc, colAccTxt
		} else if u.hover == b.id {
			f = colHover
		}
		u.round(dc, r, f, map[bool]uintptr{true: colAcc, false: colLine}[on], 10)
		u.text(dc, b.text, b.r, u.font("Segoe UI", 10, on), t, dtCenter|dtVCenter|dtSingle)
	}

	// mã QR + dòng địa chỉ
	qrBox := rect{90, 132, 350, 392}
	var content, addr, note string
	if u.mode == "lan" {
		content = u.app.QRURL()
		addr = u.app.URLs[0]
		note = "Điện thoại dùng chung Wi-Fi với máy tính này"
	} else {
		st, url, err := u.app.Tunnel.Snapshot()
		switch st {
		case "on":
			content = url + "/?pin=" + u.app.Auth.PIN()
			addr = url
			note = "Dùng được ở bất cứ đâu có Internet (4G, Wi-Fi khác)"
		case "downloading":
			note = "Đang tải thành phần kết nối (chỉ lần đầu)…"
		case "error":
			note = err
		default:
			note = "Đang tạo đường truyền qua Internet…"
		}
	}
	box := u.sr(qrBox)
	if bits := u.qr(content); content != "" && bits != nil {
		u.round(dc, box, colWhite, colWhite, 16)
		pad := u.s(14)
		size := box.R - box.L - 2*pad
		m := int32(len(bits))
		cell := size / m
		off := (size - cell*m) / 2
		br, _, _ := pCreateSolidBrush.Call(colBlack)
		for y, row := range bits {
			for x, on := range row {
				if on {
					r := rect{box.L + pad + off + int32(x)*cell, box.T + pad + off + int32(y)*cell, 0, 0}
					r.R, r.B = r.L+cell, r.T+cell
					pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
				}
			}
		}
		pDeleteObject.Call(br)
	} else {
		u.round(dc, box, colPanel, colLine, 16)
		u.text(dc, note, rect{qrBox.L + 20, qrBox.T + 100, qrBox.R - 20, qrBox.B - 20}, u.font("Segoe UI", 10, false), colMute, dtCenter|dtWord)
		note = ""
	}
	u.text(dc, "Quét mã QR bằng camera điện thoại để kết nối", rect{24, 400, 416, 424}, u.font("Segoe UI", 10, false), colMute, dtCenter|dtSingle|dtVCenter)

	// PIN
	u.text(dc, "MÃ PIN", rect{24, 432, 416, 452}, u.font("Segoe UI", 9, true), colMute, dtCenter|dtSingle|dtVCenter)
	u.text(dc, spacedPIN(u.app.Auth.PIN()), rect{24, 452, 416, 500}, u.font("Segoe UI", 28, true), colAcc, dtCenter|dtSingle|dtVCenter)

	if addr != "" {
		u.text(dc, addr, rect{24, 502, 416, 524}, u.font("Consolas", 10, false), colText, dtCenter|dtSingle|dtVCenter|dtEndEllipsis)
	}
	if note != "" {
		u.text(dc, note, rect{24, 520, 416, 540}, u.font("Segoe UI", 9, false), colMute, dtCenter|dtSingle|dtVCenter|dtEndEllipsis)
	}

	// ô đánh dấu
	for _, b := range u.buttons {
		if b.id != "auto" && b.id != "nosleep" {
			continue
		}
		on := (b.id == "auto" && autostartEnabled()) || (b.id == "nosleep" && u.settings.NoSleep)
		box := rect{b.r.L, b.r.T + 5, b.r.L + 20, b.r.T + 25}
		bc := colPanel
		if on {
			bc = colAcc
		} else if u.hover == b.id {
			bc = colHover
		}
		u.round(dc, u.sr(box), bc, map[bool]uintptr{true: colAcc, false: colLine}[on], 6)
		if on { // vẽ dấu tích bằng nét bút (không phụ thuộc phông chữ)
			pen, _, _ := pCreatePen.Call(0, uintptr(u.s(2)), colAccTxt)
			op, _, _ := pSelectObject.Call(dc, pen)
			pMoveToEx.Call(dc, uintptr(u.s(box.L+5)), uintptr(u.s(box.T+10)), 0)
			pLineTo.Call(dc, uintptr(u.s(box.L+9)), uintptr(u.s(box.T+14)))
			pLineTo.Call(dc, uintptr(u.s(box.L+15)), uintptr(u.s(box.T+6)))
			pSelectObject.Call(dc, op)
			pDeleteObject.Call(pen)
		}
		u.text(dc, b.text, rect{b.r.L + 30, b.r.T, b.r.R, b.r.B}, u.font("Segoe UI", 10, false), colText, dtVCenter|dtSingle|dtEndEllipsis)
	}

	// nút
	for _, b := range u.buttons[2:] {
		if b.id == "auto" || b.id == "nosleep" {
			continue
		}
		r := u.sr(b.r)
		f := colPanel
		if u.hover == b.id {
			f = colHover
		}
		t := colText
		if b.id == "unpair" || b.id == "netoff" {
			t = colBad
		}
		u.round(dc, r, f, colLine, 10)
		u.text(dc, b.text, b.r, u.font("Segoe UI", 10, false), t, dtCenter|dtVCenter|dtSingle)
	}
	u.text(dc, fmt.Sprintf("%d điện thoại đã ghép nối  ·  Đóng cửa sổ: vẫn chạy ngầm ở khay đồng hồ", u.app.Auth.Count()),
		rect{24, 726, 416, 756}, u.font("Segoe UI", 8, false), colMute, dtCenter|dtSingle|dtVCenter)
}

// ------------------------------------------------------------------ events --
func msgBox(hwnd uintptr, text string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(wstr(text))), uintptr(unsafe.Pointer(wstr("RemotePC"))), flags)
	return int(r)
}

func setClipboard(hwnd uintptr, s string) {
	p, _ := windows.UTF16FromString(s)
	if r, _, _ := pOpenClipboard.Call(hwnd); r == 0 {
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(p)*2))
	if h == 0 {
		return
	}
	ptr, _, _ := pGlobalLock.Call(h)
	kernel32.NewProc("RtlMoveMemory").Call(ptr, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)*2))
	pGlobalUnlock.Call(h)
	pSetClipboardData.Call(13, h) // CF_UNICODETEXT
}

func (u *ui) click(id string) {
	switch id {
	case "lan":
		u.mode = "lan"
	case "net":
		u.mode = "net"
		u.app.Tunnel.Start(u.app.Port)
		u.settings.Internet = true
		u.settings.Save()
	case "netretry":
		u.app.Tunnel.Stop()
		u.app.Tunnel.Start(u.app.Port)
	case "netoff":
		u.app.Tunnel.Stop()
		u.mode = "lan"
		u.settings.Internet = false
		u.settings.Save()
	case "auto":
		if err := setAutostart(!autostartEnabled()); err != nil {
			msgBox(u.hwnd, "Không bật được tự chạy: "+err.Error(), 0x10)
		}
	case "nosleep":
		u.settings.NoSleep = !u.settings.NoSleep
		u.settings.Save()
		applyNoSleep(u.settings.NoSleep)
	case "copy":
		if _, url, _ := u.app.Tunnel.Snapshot(); url != "" {
			setClipboard(u.hwnd, url)
			msgBox(u.hwnd, "Đã sao chép link:\n"+url+"\n\nGửi link này sang điện thoại rồi nhập mã PIN.", 0x40)
		}
	case "pin":
		u.app.Auth.NewPIN()
	case "unpair":
		if msgBox(u.hwnd, "Huỷ ghép nối tất cả điện thoại?\nLần sau điện thoại phải nhập lại mã PIN.", 0x4|0x30) == 6 {
			u.app.Auth.ClearTokens()
			u.app.Auth.NewPIN()
		}
	case "quit":
		if msgBox(u.hwnd, "Thoát hẳn RemotePC?\nĐiện thoại sẽ không vào được máy tính cho tới khi mở lại.", 0x4|0x20) == 6 {
			pPostQuitMessage.Call(0)
		}
	}
	u.invalidate()
}

func (u *ui) invalidate() { pInvalidateRect.Call(u.hwnd, 0, 0) }

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	u := theUI
	if wmTaskbarCreated != 0 && msg == wmTaskbarCreated { // Explorer khởi động lại
		trayAdd(hwnd, u.icon)
		return 0
	}
	switch msg {
	case wmTray:
		switch lParam & 0xFFFF {
		case 0x202, 0x203: // bấm trái / bấm đúp
			showMain(hwnd)
		case 0x205: // bấm phải
			switch trayMenu(hwnd) {
			case 1:
				showMain(hwnd)
			case 2:
				pPostQuitMessage.Call(0)
			}
		}
		return 0
	case wmShowMain:
		showMain(hwnd)
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var cr rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
		mem, _, _ := pCreateCompatibleDC.Call(hdc)
		bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(cr.R), uintptr(cr.B))
		old, _, _ := pSelectObject.Call(mem, bmp)
		u.paint(mem)
		pBitBlt.Call(hdc, 0, 0, uintptr(cr.R), uintptr(cr.B), mem, 0, 0, 0x00CC0020)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mem)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBkgnd:
		return 1
	case wmMouseMove:
		h := u.hit(int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16)))
		if h != u.hover {
			u.hover = h
			u.invalidate()
		}
		return 0
	case wmSetCursor:
		if u.hover != "" && lParam&0xFFFF == 1 { // HTCLIENT
			c, _, _ := pLoadCursorW.Call(0, 32649) // IDC_HAND
			pSetCursor.Call(c)
			return 1
		}
	case wmLButtonUp:
		if id := u.hit(int32(int16(lParam&0xFFFF)), int32(int16(lParam>>16))); id != "" {
			u.click(id)
		}
		return 0
	case wmTimer, wmApp:
		u.invalidate()
		return 0
	case wmClose: // đóng cửa sổ = chạy ngầm
		hideMain(hwnd)
		if !u.settings.Hinted {
			u.settings.Hinted = true
			u.settings.Save()
			trayBalloon(hwnd, u.icon, "RemotePC vẫn đang chạy ngầm", "Điện thoại vẫn vào được. Bấm biểu tượng RemotePC ở khay đồng hồ để mở lại, bấm chuột phải để thoát hẳn.")
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func fatalUI(s string) { msgBox(0, s, 0x10) }

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func runUI(app *App) {
	runtime.LockOSThread()
	u := &ui{app: app, mode: "lan", fonts: map[string]uintptr{}, dpi: 96, settings: loadSettings()}
	theUI = u
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d > 0 {
			u.dpi = int(d)
		}
	}
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, 32512)
	var icon uintptr
	if len(iconPNG) > 0 {
		icon, _, _ = pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&iconPNG[0])), uintptr(len(iconPNG)), 1, 0x00030000, 0, 0, 0)
	}
	wc := wndClassEx{wndProc: windows.NewCallback(wndProc), instance: inst, cursor: cursor, icon: icon, iconSm: icon,
		className: wstr("RemotePCWindow")}
	wc.size = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	style := uintptr(0x00C00000 | 0x00080000 | 0x00020000) // CAPTION | SYSMENU | MINIMIZEBOX
	r := rect{0, 0, u.s(winW), u.s(winH)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	w, h := r.R-r.L, r.B-r.T
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	x, y := (int32(sw)-w)/2, (int32(sh)-h)/2
	hwnd, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(wstr("RemotePCWindow"))), uintptr(unsafe.Pointer(wstr("RemotePC v"+Version))),
		style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, inst, 0)
	if hwnd == 0 {
		fatalUI("Không tạo được cửa sổ RemotePC")
		os.Exit(1)
	}
	u.hwnd = hwnd
	u.icon = icon
	if icon != 0 {
		pSendMessageW.Call(hwnd, wmSetIcon, 0, icon)
		pSendMessageW.Call(hwnd, wmSetIcon, 1, icon)
	}
	if pDwmSetWindowAttribute.Find() == nil { // thanh tiêu đề tối
		dark := int32(1)
		pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	}
	refresh := func() { pInvalidateRect.Call(hwnd, 0, 0) }
	app.Tunnel.OnChange = refresh
	app.Auth.OnPinChange = refresh
	pSetTimer.Call(hwnd, 1, 1000, 0)
	refreshAutostartPath()
	applyNoSleep(u.settings.NoSleep)
	trayAdd(hwnd, icon)
	if u.settings.Internet { // lần trước bật Internet thì bật lại luôn
		u.mode = "net"
		app.Tunnel.Start(app.Port)
	}
	if !app.Background {
		pShowWindow.Call(hwnd, 5)
		pUpdateWindow.Call(hwnd)
	}

	var m winMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	trayRemove(hwnd, icon)
	applyNoSleep(false)
	app.Tunnel.Stop()
	os.Exit(0)
}
