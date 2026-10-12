//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

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
