//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const appIconResource = 2 // rsrc allocates ID 1 to the manifest, then ID 2 to the icon group.

func setDesktopIdentity() {
	name, _ := syscall.UTF16PtrFromString("sebaawl.LinkDrop")
	setID := syscall.NewLazyDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID")
	if setID.Find() == nil {
		setID.Call(uintptr(unsafe.Pointer(name)))
	}
}

func setWindowIcons(hwnd uintptr) {
	module, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	dpi := windowDPI(hwnd)
	for _, item := range []struct {
		size, kind, class uintptr
	}{
		{uintptr(scaledPixels(32, dpi)), 1, ^uintptr(13)}, // GCLP_HICON
		{uintptr(scaledPixels(16, dpi)), 0, ^uintptr(33)}, // GCLP_HICONSM
	} {
		icon, _, _ := desktopUser32.NewProc("LoadImageW").Call(module, appIconResource, 1, item.size, item.size, 0x8000)
		if icon != 0 {
			desktopUser32.NewProc("SendMessageW").Call(hwnd, 0x80, item.kind, icon) // WM_SETICON
			desktopUser32.NewProc("SetClassLongPtrW").Call(hwnd, item.class, icon)
		}
	}
}

// Operations that touch COM dialogs, clipboard ownership or window state are
// dispatched onto the locked WebView UI thread. HTTP/bridge workers wait for
// the result while the native dialog runs its own Windows modal message loop.
func onUIThread(w webview.WebView, closed <-chan struct{}, operation func() error) error {
	result := make(chan error, 1)
	select {
	case <-closed:
		return errors.New("La aplicación se está cerrando")
	default:
	}
	w.Dispatch(func() { result <- operation() })
	select {
	case err := <-result:
		return err
	case <-closed:
		return errors.New("La aplicación se está cerrando")
	}
}

func clipboardText(hwnd uintptr) (string, error) {
	ok, _, _ := desktopUser32.NewProc("OpenClipboard").Call(hwnd)
	if ok == 0 {
		return "", errors.New("El portapapeles está ocupado. Vuelve a pulsar Pegar")
	}
	defer desktopUser32.NewProc("CloseClipboard").Call()
	handle, _, _ := desktopUser32.NewProc("GetClipboardData").Call(13) // CF_UNICODETEXT
	if handle == 0 {
		return "", errors.New("El portapapeles no contiene texto")
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	size, _, _ := kernel.NewProc("GlobalSize").Call(handle)
	if size == 0 || size > 128*1024 {
		return "", errors.New("El texto del portapapeles está vacío o es demasiado largo")
	}
	address, _, _ := kernel.NewProc("GlobalLock").Call(handle)
	if address == 0 {
		return "", errors.New("No se pudo leer el portapapeles")
	}
	defer kernel.NewProc("GlobalUnlock").Call(handle)
	units := unsafe.Slice((*uint16)(unsafe.Pointer(address)), int(size/2))
	end := 0
	for end < len(units) && units[end] != 0 {
		end++
	}
	text := string(utf16.Decode(units[:end]))
	if len(text) > 64*1024 {
		return "", errors.New("El texto del portapapeles es demasiado largo")
	}
	return text, nil
}

type folderDialog struct{ table *[29]uintptr }
type shellFolder struct{ table *[8]uintptr }

func (dialog *folderDialog) call(index int, args ...uintptr) uintptr {
	params := append([]uintptr{uintptr(unsafe.Pointer(dialog))}, args...)
	result, _, _ := syscall.SyscallN(dialog.table[index], params...)
	runtime.KeepAlive(dialog)
	return result
}

func (folder *shellFolder) call(index int, args ...uintptr) uintptr {
	params := append([]uintptr{uintptr(unsafe.Pointer(folder))}, args...)
	result, _, _ := syscall.SyscallN(folder.table[index], params...)
	runtime.KeepAlive(folder)
	return result
}

func chooseWindowsFolder(hwnd uintptr, initial string) (string, error) {
	ole := syscall.NewLazyDLL("ole32.dll")
	// Balance our COM initialization even when WebView2 initialized this STA.
	initialized, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(initialized) < 0 {
		return "", errors.New("No se pudo abrir el selector de carpetas")
	}
	defer ole.NewProc("CoUninitialize").Call()
	classID, _ := windows.GUIDFromString("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}")
	interfaceID, _ := windows.GUIDFromString("{D57C7288-D4AD-4768-BE02-9D969532D960}")
	var dialog *folderDialog
	result, _, _ := ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&classID)), 0, 1, uintptr(unsafe.Pointer(&interfaceID)), uintptr(unsafe.Pointer(&dialog)))
	if int32(result) < 0 || dialog == nil {
		return "", errors.New("No se pudo crear el selector de carpetas de Windows")
	}
	defer dialog.call(2) // IUnknown.Release
	var options uint32
	if int32(dialog.call(10, uintptr(unsafe.Pointer(&options)))) < 0 {
		return "", errors.New("No se pudo configurar el selector de carpetas")
	}
	// FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM | FOS_NOCHANGEDIR | FOS_PATHMUSTEXIST.
	if int32(dialog.call(9, uintptr(options|0x20|0x40|0x8|0x800))) < 0 {
		return "", errors.New("No se pudo configurar el selector de carpetas")
	}
	title, _ := syscall.UTF16PtrFromString("Seleccionar carpeta de destino")
	button, _ := syscall.UTF16PtrFromString("Seleccionar carpeta")
	dialog.call(17, uintptr(unsafe.Pointer(title)))
	runtime.KeepAlive(title)
	dialog.call(18, uintptr(unsafe.Pointer(button)))
	runtime.KeepAlive(button)
	itemID, _ := windows.GUIDFromString("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
	if path, err := syscall.UTF16PtrFromString(initial); err == nil {
		var start *shellFolder
		result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHCreateItemFromParsingName").Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&itemID)), uintptr(unsafe.Pointer(&start)))
		if int32(result) >= 0 && start != nil {
			dialog.call(12, uintptr(unsafe.Pointer(start))) // SetFolder; ignore nonexistent initial paths.
			start.call(2)
		}
	}
	result = dialog.call(3, hwnd)     // IModalWindow.Show, owned by the LinkDrop window.
	if uint32(result) == 0x800704C7 { // HRESULT_FROM_WIN32(ERROR_CANCELLED)
		return "", nil
	}
	if int32(result) < 0 {
		return "", fmt.Errorf("No se pudo seleccionar la carpeta (Windows 0x%08X)", uint32(result))
	}
	var selected *shellFolder
	if int32(dialog.call(20, uintptr(unsafe.Pointer(&selected)))) < 0 || selected == nil {
		return "", errors.New("No se pudo obtener la carpeta seleccionada")
	}
	defer selected.call(2)
	var path *uint16
	if int32(selected.call(5, 0x80058000, uintptr(unsafe.Pointer(&path)))) < 0 || path == nil { // SIGDN_FILESYSPATH
		return "", errors.New("La selección no es una carpeta del equipo")
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(path))
	return windows.UTF16PtrToString(path), nil
}
