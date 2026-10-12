//go:build windows

package main

import (
	"syscall"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

var desktopUser32 = syscall.NewLazyDLL("user32.dll")

type desktopRect struct{ Left, Top, Right, Bottom int32 }
type desktopMonitorInfo struct {
	Size    uint32
	Monitor desktopRect
	Work    desktopRect
	Flags   uint32
}

// The embedded manifest enables PerMonitorV2 at process creation. Keep a
// runtime fallback for builds made without the resource file.
func enableHighDPI() {
	setContext := desktopUser32.NewProc("SetProcessDpiAwarenessContext")
	if setContext.Find() == nil {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the pseudo-handle -4.
		ok, _, err := setContext.Call(^uintptr(3))
		if ok != 0 || err == syscall.ERROR_ACCESS_DENIED {
			return // The manifest may already have established DPI awareness.
		}
	}
	setAwareness := syscall.NewLazyDLL("shcore.dll").NewProc("SetProcessDpiAwareness")
	if setAwareness.Find() == nil {
		result, _, _ := setAwareness.Call(2)
		if result == 0 || result == 0x80070005 {
			return
		}
	}
	desktopUser32.NewProc("SetProcessDPIAware").Call()
}

func windowDPI(hwnd uintptr) int {
	getDPI := desktopUser32.NewProc("GetDpiForWindow")
	if getDPI.Find() == nil {
		dpi, _, _ := getDPI.Call(hwnd)
		if dpi != 0 {
			return int(dpi)
		}
	}
	return 96
}

func monitorWorkArea(hwnd uintptr) desktopRect {
	monitor, _, _ := desktopUser32.NewProc("MonitorFromWindow").Call(hwnd, 2)
	info := desktopMonitorInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ := desktopUser32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok != 0 {
		return info.Work
	}
	width, _, _ := desktopUser32.NewProc("GetSystemMetrics").Call(0)
	height, _, _ := desktopUser32.NewProc("GetSystemMetrics").Call(1)
	return desktopRect{Right: int32(width), Bottom: int32(height)}
}

func scaledPixels(value, dpi int) int { return (value*dpi + 48) / 96 }

func updateWindowTheme(hwnd uintptr, theme string) {
	light := uint64(1)
	if theme == "dark" {
		light = 0
	} else if theme != "light" {
		key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
		if err != nil {
			return
		}
		defer key.Close()
		light, _, err = key.GetIntegerValue("AppsUseLightTheme")
		if err != nil {
			return
		}
	}
	var dark int32
	if light == 0 {
		dark = 1
	}
	setAttribute := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if setAttribute.Find() == nil {
		// DWMWA_USE_IMMERSIVE_DARK_MODE keeps the native title bar in the
		// same Windows application theme as the embedded controls.
		setAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	}
}

func configureDesktopWindow(w webview.WebView, theme func() string) {
	hwnd := uintptr(w.Window())
	updateWindowTheme(hwnd, theme())
	setWindowIcons(hwnd)
	dpi := windowDPI(hwnd)
	work := monitorWorkArea(hwnd)
	availableW, availableH := int(work.Right-work.Left)-24, int(work.Bottom-work.Top)-24
	width := min(scaledPixels(1120, dpi), availableW)
	height := min(scaledPixels(760, dpi), availableH)
	w.SetSize(min(scaledPixels(640, dpi), availableW), min(scaledPixels(500, dpi), availableH), webview.HintMin)
	x, y := int(work.Left)+(int(work.Right-work.Left)-width)/2, int(work.Top)+(int(work.Bottom-work.Top)-height)/2
	desktopUser32.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x14)

	// The webview library handles WM_SIZE but does not apply the suggested
	// rectangle from WM_DPICHANGED. A subclass keeps monitor transitions sharp
	// and updates the minimum size in the new monitor's physical pixels.
	controls := syscall.NewLazyDLL("comctl32.dll")
	setSubclass := controls.NewProc("SetWindowSubclass")
	defSubclass := controls.NewProc("DefSubclassProc")
	removeSubclass := controls.NewProc("RemoveWindowSubclass")
	if setSubclass.Find() != nil || defSubclass.Find() != nil || removeSubclass.Find() != nil {
		return
	}
	var callback uintptr
	callback = syscall.NewCallback(func(hwnd, message, wp, lp, id, data uintptr) uintptr {
		switch message {
		case 0x001A: // WM_SETTINGCHANGE
			updateWindowTheme(hwnd, theme())
		case 0x02E0: // WM_DPICHANGED
			dpi := int(wp & 0xffff)
			r := (*desktopRect)(unsafe.Pointer(lp))
			work := monitorWorkArea(hwnd)
			w.SetSize(min(scaledPixels(640, dpi), int(work.Right-work.Left)-24), min(scaledPixels(500, dpi), int(work.Bottom-work.Top)-24), webview.HintMin)
			desktopUser32.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x14)
			setWindowIcons(hwnd)
			return 0
		case 0x0082: // WM_NCDESTROY
			removeSubclass.Call(hwnd, callback, id)
		}
		result, _, _ := defSubclass.Call(hwnd, message, wp, lp)
		return result
	})
	setSubclass.Call(hwnd, callback, 1, 0)
}
