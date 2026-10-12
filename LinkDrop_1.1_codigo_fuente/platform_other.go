//go:build !windows

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
)

// The HTTP development runner is only built on non-Windows platforms.
func runApplication(a *app, handler http.Handler) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	defer a.stopDownload()
	link := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", listener.Addr().(*net.TCPAddr).Port, a.token)
	log.Println("LinkDrop available at", link)
	if err := openBrowser(link); err != nil {
		log.Printf("No graphical browser available: %v", err)
	}
	return http.Serve(listener, handler)
}

func showStartupError(message string) { log.Print(message) }

func hideCmdWindow(cmd *exec.Cmd) {}
func openBrowser(s string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", s).Start()
	}
	return exec.Command("xdg-open", s).Start()
}
func openFolder(s string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", s).Start()
	}
	return exec.Command("xdg-open", s).Start()
}
