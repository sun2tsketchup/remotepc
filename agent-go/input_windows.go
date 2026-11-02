//go:build windows

package main

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const inputAvailable = true

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	pSendInput       = user32.NewProc("SendInput")
	pGetCursorPos    = user32.NewProc("GetCursorPos")
	pGetSystemMetric = user32.NewProc("GetSystemMetrics")
	pVkKeyScanW      = user32.NewProc("VkKeyScanW")
	pDpiCtx          = user32.NewProc("SetProcessDpiAwarenessContext")
	pDpiAware        = user32.NewProc("SetProcessDPIAware")
	pSetConsoleOutCP = kernel32.NewProc("SetConsoleOutputCP")
	pSetConsoleTitle = kernel32.NewProc("SetConsoleTitleW")
)

// INPUT trên Windows 64-bit: 40 byte.
type mouseInput struct {
	typ       uint32
	_         uint32
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	_         uint32
	extra     uintptr
}
type keyInput struct {
	typ      uint32
	_        uint32
	vk, scan uint16
	flags    uint32
	time     uint32
	_        uint32
	extra    uintptr
	_        [8]byte
}

const (
	mMove, mLeftDown, mLeftUp, mRightDown, mRightUp = 0x1, 0x2, 0x4, 0x8, 0x10
	mMidDown, mMidUp, mWheel, mHWheel               = 0x20, 0x40, 0x800, 0x1000
	mVirtualDesk, mAbsolute                         = 0x4000, 0x8000
	kExtended, kUp, kUnicode                        = 0x1, 0x2, 0x4
)

func initPlatform() {
	if pDpiCtx.Find() == nil {
		pDpiCtx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	} else if pDpiAware.Find() == nil {
		pDpiAware.Call()
	}
	pSetConsoleOutCP.Call(65001)
	t, _ := windows.UTF16PtrFromString("RemotePC")
	pSetConsoleTitle.Call(uintptr(unsafe.Pointer(t)))
}

func sendMouse(ins ...mouseInput) {
	for i := range ins {
		ins[i].typ = 0
	}
	pSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}
func sendKeys(ins []keyInput) {
	if len(ins) == 0 {
		return
	}
	for i := range ins {
		ins[i].typ = 1
	}
	pSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}

func metric(i int) int {
	r, _, _ := pGetSystemMetric.Call(uintptr(i))
	return int(int32(r))
}

func cursorPos() (int, int, bool) {
	var p struct{ X, Y int32 }
	r, _, _ := pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), r != 0
}

// ensureOn đưa con trỏ vào giữa màn hình m nếu nó đang ở màn hình khác.
func ensureOn(m Monitor) {
	if x, y, ok := cursorPos(); ok && (x < m.Left || x >= m.Left+m.Width || y < m.Top || y >= m.Top+m.Height) {
		moveTo(m.Left+m.Width/2, m.Top+m.Height/2)
	}
}

func moveTo(x, y int) {
	vx, vy, vw, vh := metric(76), metric(77), metric(78), metric(79)
	if vw < 2 || vh < 2 {
		return
	}
	nx := int32((x - vx) * 65535 / (vw - 1))
	ny := int32((y - vy) * 65535 / (vh - 1))
	sendMouse(mouseInput{dx: nx, dy: ny, flags: mMove | mAbsolute | mVirtualDesk})
}

func buttonFlags(b string) (uint32, uint32) {
	switch b {
	case "right":
		return mRightDown, mRightUp
	case "middle":
		return mMidDown, mMidUp
	}
	return mLeftDown, mLeftUp
}

var vkMap = map[string]uint16{
	"enter": 0x0D, "esc": 0x1B, "tab": 0x09, "backspace": 0x08, "delete": 0x2E, "space": 0x20,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27, "home": 0x24, "end": 0x23,
	"pageup": 0x21, "pagedown": 0x22, "ctrl": 0x11, "alt": 0x12, "shift": 0x10,
	"win": 0x5B, "cmd": 0x5B, "capslock": 0x14, "printscreen": 0x2C,
	"volup": 0xAF, "voldown": 0xAE, "mute": 0xAD, "playpause": 0xB3, "next": 0xB0, "prev": 0xB1,
}
var extendedVK = map[uint16]bool{0x2E: true, 0x26: true, 0x28: true, 0x25: true, 0x27: true,
	0x24: true, 0x23: true, 0x21: true, 0x22: true, 0x5B: true, 0x2C: true}

func init() {
	for i := 1; i <= 12; i++ {
		vkMap[fmt.Sprintf("f%d", i)] = uint16(0x6F + i)
	}
}

func vkOf(name string) (uint16, error) {
	name = strings.ToLower(name)
	if v, ok := vkMap[name]; ok {
		return v, nil
	}
	r := []rune(name)
	if len(r) == 1 {
		c := r[0]
		if c >= 'a' && c <= 'z' {
			return uint16(c - 'a' + 'A'), nil
		}
		if c >= '0' && c <= '9' {
			return uint16(c), nil
		}
		if c < 0x10000 {
			res, _, _ := pVkKeyScanW.Call(uintptr(c))
			if int16(res) != -1 {
				return uint16(res & 0xFF), nil
			}
		}
	}
	return 0, fmt.Errorf("phím không hỗ trợ: %s", name)
}

func keyEv(vk uint16, up bool) keyInput {
	var f uint32
	if extendedVK[vk] {
		f |= kExtended
	}
	if up {
		f |= kUp
	}
	return keyInput{vk: vk, flags: f}
}

func typeText(s string) {
	var ins []keyInput
	for _, r := range s {
		if r == '\n' {
			ins = append(ins, keyEv(0x0D, false), keyEv(0x0D, true))
			continue
		}
		if r == '\r' {
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			ins = append(ins, keyInput{scan: u, flags: kUnicode}, keyInput{scan: u, flags: kUnicode | kUp})
		}
	}
	sendKeys(ins)
}

func handleInput(m msg, mon Monitor) error {
	switch m.str("t") {
	case "move": // giữ con trỏ trong màn hình đang xem
		if x, y, ok := cursorPos(); ok {
			nx := clamp(x+int(m.num("dx", 0)), mon.Left, mon.Left+mon.Width-1)
			ny := clamp(y+int(m.num("dy", 0)), mon.Top, mon.Top+mon.Height-1)
			moveTo(nx, ny)
		}
	case "moveto":
		moveTo(mon.Left+int(m.num("x", 0)*float64(mon.Width)), mon.Top+int(m.num("y", 0)*float64(mon.Height)))
	case "click":
		d, u := buttonFlags(m.str("b"))
		n := clamp(int(m.num("n", 1)), 1, 3)
		for i := 0; i < n; i++ {
			sendMouse(mouseInput{flags: d}, mouseInput{flags: u})
		}
	case "down":
		d, _ := buttonFlags(m.str("b"))
		sendMouse(mouseInput{flags: d})
	case "up":
		_, u := buttonFlags(m.str("b"))
		sendMouse(mouseInput{flags: u})
	case "scroll":
		if dy := int32(m.num("dy", 0)); dy != 0 {
			sendMouse(mouseInput{flags: mWheel, mouseData: uint32(dy * 120)})
		}
		if dx := int32(m.num("dx", 0)); dx != 0 {
			sendMouse(mouseInput{flags: mHWheel, mouseData: uint32(dx * 120)})
		}
	case "wheel": // đơn vị 1/120 nấc, cho phép lăn mịn
		if dy := int32(m.num("dy", 0)); dy != 0 {
			sendMouse(mouseInput{flags: mWheel, mouseData: uint32(dy)})
		}
		if dx := int32(m.num("dx", 0)); dx != 0 {
			sendMouse(mouseInput{flags: mHWheel, mouseData: uint32(dx)})
		}
	case "kdown", "kup":
		vk, err := vkOf(m.str("k"))
		if err != nil {
			return err
		}
		sendKeys([]keyInput{keyEv(vk, m.str("t") == "kup")})
	case "text":
		s := m.str("s")
		if r := []rune(s); len(r) > 2000 {
			s = string(r[:2000])
		}
		typeText(s)
	case "key":
		vk, err := vkOf(m.str("k"))
		if err != nil {
			return err
		}
		sendKeys([]keyInput{keyEv(vk, false), keyEv(vk, true)})
	case "combo":
		arr, _ := m["k"].([]any)
		if len(arr) > 5 {
			arr = arr[:5]
		}
		var vks []uint16
		for _, k := range arr {
			s, _ := k.(string)
			vk, err := vkOf(s)
			if err != nil {
				return err
			}
			vks = append(vks, vk)
		}
		var ins []keyInput
		for _, v := range vks {
			ins = append(ins, keyEv(v, false))
		}
		for i := len(vks) - 1; i >= 0; i-- {
			ins = append(ins, keyEv(vks[i], true))
		}
		sendKeys(ins)
	}
	return nil
}
