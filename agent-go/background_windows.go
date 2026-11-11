//go:build windows

package main

// Chạy ngầm: biểu tượng ở khay hệ thống, tự chạy cùng Windows, giữ máy không ngủ, chỉ chạy 1 bản.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	shell32                  = windows.NewLazySystemDLL("shell32.dll")
	pShellNotifyIconW        = shell32.NewProc("Shell_NotifyIconW")
	pCreatePopupMenu         = user32.NewProc("CreatePopupMenu")
	pAppendMenuW             = user32.NewProc("AppendMenuW")
	pTrackPopupMenu          = user32.NewProc("TrackPopupMenu")
	pDestroyMenu             = user32.NewProc("DestroyMenu")
	pSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
	pFindWindowW             = user32.NewProc("FindWindowW")
	pPostMessageW            = user32.NewProc("PostMessageW")
	pRegisterWindowMessageW  = user32.NewProc("RegisterWindowMessageW")
	pIsWindowVisible         = user32.NewProc("IsWindowVisible")
	pCreateMutexW            = kernel32.NewProc("CreateMutexW")
	pSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	wmTaskbarCreated         uintptr
)

const (
	wmTray     = wmApp + 1 // tin nhắn từ biểu tượng khay
	wmShowMain = wmApp + 2 // bản thứ hai nhờ bản đang chạy hiện cửa sổ
	runKey     = `Software\Microsoft\Windows\CurrentVersion\Run`
	runName    = "BetagonRemotePC"
	oldRunName = "RemotePC" // tên cũ trước v0.6.0
)

// ---------------------------------------------------------------- settings --
type Settings struct {
	NoSleep  bool   `json:"no_sleep"`
	Internet bool   `json:"internet"`
	Hinted   bool   `json:"tray_hinted"`
	Lang     string `json:"lang"` // "vi" (mặc định) hoặc "en"
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".remotepc", "settings.json")
}

func loadSettings() *Settings {
	s := &Settings{NoSleep: true}
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, s)
	}
	return s
}

func (s *Settings) Save() {
	b, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(settingsPath(), b, 0o600)
}

// --------------------------------------------------------------- autostart --
func exePath() string {
	p, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func autostartCmd() string { return `"` + exePath() + `" --background` }

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runName)
	return err == nil && v != ""
}

func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if on {
		return k.SetStringValue(runName, autostartCmd())
	}
	err = k.DeleteValue(runName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

// Khi chạy bản .exe mới (tên file đổi theo phiên bản), cập nhật lại đường dẫn tự chạy.
func refreshAutostartPath() {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if v, _, err := k.GetStringValue(oldRunName); err == nil && v != "" { // chuyển tên cũ sang tên mới
		k.DeleteValue(oldRunName)
		k.SetStringValue(runName, autostartCmd())
	}
	if v, _, err := k.GetStringValue(runName); err == nil && v != "" && !strings.EqualFold(v, autostartCmd()) {
		k.SetStringValue(runName, autostartCmd())
	}
}

// ---------------------------------------------------------------- no sleep --
func applyNoSleep(on bool) {
	const esContinuous, esSystemRequired = 0x80000000, 0x00000001
	if on {
		pSetThreadExecutionState.Call(esContinuous | esSystemRequired)
	} else {
		pSetThreadExecutionState.Call(esContinuous)
	}
}

// ---------------------------------------------------------- single instance --
// ensureSingleInstance: nếu RemotePC đang chạy rồi thì hiện cửa sổ của nó và thoát bản này.
func ensureSingleInstance() {
	setLang(loadSettings().Lang) // ngôn ngữ cần có sớm, trước cả hộp thoại lỗi
	h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(wstr("Local\\RemotePC_SingleInstance"))))
	if h != 0 && err == windows.ERROR_ALREADY_EXISTS {
		if w, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(wstr("RemotePCWindow"))), 0); w != 0 {
			pPostMessageW.Call(w, wmShowMain, 0, 0)
		}
		os.Exit(0)
	}
}

// -------------------------------------------------------------------- tray --
type notifyIconData struct {
	size        uint32
	hwnd        uintptr
	id          uint32
	flags       uint32
	callbackMsg uint32
	icon        uintptr
	tip         [128]uint16
	state       uint32
	stateMask   uint32
	info        [256]uint16
	version     uint32
	infoTitle   [64]uint16
	infoFlags   uint32
	guid        windows.GUID
	balloonIcon uintptr
}

func copyW(dst []uint16, s string) {
	p, _ := windows.UTF16FromString(s)
	if len(p) > len(dst) {
		p = p[:len(dst)]
		p[len(p)-1] = 0
	}
	copy(dst, p)
}

func trayData(hwnd, icon uintptr) notifyIconData {
	d := notifyIconData{hwnd: hwnd, id: 1, flags: 0x1 | 0x2 | 0x4, callbackMsg: wmTray, icon: icon}
	d.size = uint32(unsafe.Sizeof(d))
	copyW(d.tip[:], "Betagon Remote PC v"+Version+tr(" đang chạy ngầm"))
	return d
}

func trayAdd(hwnd, icon uintptr) {
	if wmTaskbarCreated == 0 {
		wmTaskbarCreated, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(wstr("TaskbarCreated"))))
	}
	d := trayData(hwnd, icon)
	pShellNotifyIconW.Call(0, uintptr(unsafe.Pointer(&d)))
}

func trayRemove(hwnd, icon uintptr) {
	d := trayData(hwnd, icon)
	pShellNotifyIconW.Call(2, uintptr(unsafe.Pointer(&d)))
}

func trayBalloon(hwnd, icon uintptr, title, text string) {
	d := trayData(hwnd, icon)
	d.flags |= 0x10 // NIF_INFO
	copyW(d.infoTitle[:], title)
	copyW(d.info[:], text)
	d.infoFlags = 0x1 // NIIF_INFO
	pShellNotifyIconW.Call(1, uintptr(unsafe.Pointer(&d)))
}

// trayMenu hiện menu chuột phải, trả về mã lệnh đã chọn.
func trayMenu(hwnd uintptr) int {
	m, _, _ := pCreatePopupMenu.Call()
	pAppendMenuW.Call(m, 0, 1, uintptr(unsafe.Pointer(wstr(tr("Mở Betagon Remote PC")))))
	pAppendMenuW.Call(m, 0x800, 0, 0) // MF_SEPARATOR
	pAppendMenuW.Call(m, 0, 2, uintptr(unsafe.Pointer(wstr(tr("Thoát hẳn Betagon Remote PC")))))
	var pt struct{ X, Y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hwnd)
	r, _, _ := pTrackPopupMenu.Call(m, 0x100|0x2, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	pDestroyMenu.Call(m)
	return int(r)
}

func showMain(hwnd uintptr) {
	pShowWindow.Call(hwnd, 9) // SW_RESTORE
	pShowWindow.Call(hwnd, 5)
	pSetForegroundWindow.Call(hwnd)
}

func hideMain(hwnd uintptr) { pShowWindow.Call(hwnd, 0) }
