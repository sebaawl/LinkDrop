//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
)

func hideCmdWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func runApplication(a *app, handler http.Handler) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	enableHighDPI()
	setDesktopIdentity()
	version, err := webviewloader.GetInstalledVersion()
	if err != nil || version == "" {
		return fmt.Errorf("LinkDrop necesita Microsoft Edge WebView2 Runtime. Instálalo desde https://developer.microsoft.com/microsoft-edge/webview2/ y vuelve a abrir LinkDrop")
	}
	html, err := a.pageHTML()
	if err != nil {
		return fmt.Errorf("no se pudo cargar la interfaz: %w", err)
	}
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:      filepath.Join(filepath.Dir(a.directory), "WebView2"),
		AutoFocus:     true,
		WindowOptions: webview.WindowOptions{Title: "LinkDrop", Width: 800, Height: 600, IconId: appIconResource, Center: true},
	})
	if w == nil {
		return fmt.Errorf("no se pudo crear la ventana de LinkDrop")
	}
	defer w.Destroy()
	defer a.stopDownload()
	closed := make(chan struct{})
	defer close(closed)
	windowTheme := a.theme
	configureDesktopWindow(w, func() string { return windowTheme })
	a.quit = func() { w.Dispatch(w.Terminate) }
	a.applyTheme = func(theme string) error {
		return onUIThread(w, closed, func() error {
			windowTheme = theme
			updateWindowTheme(uintptr(w.Window()), theme)
			return nil
		})
	}
	a.readClipboard = func() (string, error) {
		var text string
		err := onUIThread(w, closed, func() error {
			var err error
			text, err = clipboardText(uintptr(w.Window()))
			return err
		})
		return text, err
	}
	a.chooseFolder = func(initial string) (string, error) {
		var folder string
		err := onUIThread(w, closed, func() error {
			var err error
			folder, err = chooseWindowsFolder(uintptr(w.Window()), initial)
			return err
		})
		return folder, err
	}
	// Network extraction can take time. Run it off the UI thread, and deliver
	// its result through the WebView message bridge without opening a socket.
	err = w.Bind("linkdropRequest", func(id, method, path, body string) {
		go func() {
			response := desktopRequest(handler, a.token, method, path, body)
			encoded, err := json.Marshal(response)
			if err != nil {
				return
			}
			encodedID, _ := json.Marshal(id)
			select {
			case <-closed:
				return
			default:
			}
			w.Dispatch(func() {
				w.Eval("window.linkdropRespond(" + string(encodedID) + "," + string(encoded) + ")")
			})
		}()
	})
	if err != nil {
		return fmt.Errorf("no se pudo conectar la interfaz: %w", err)
	}
	w.SetHtml(html)
	w.Run()
	return nil
}

func showStartupError(message string) {
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("LinkDrop")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}

func openFolder(s string) error { return exec.Command("explorer.exe", s).Start() }
