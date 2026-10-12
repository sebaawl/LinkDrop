//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func hideCmdWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func openBrowser(s string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", s).Start()
}
func openFolder(s string) error { return exec.Command("explorer.exe", s).Start() }
