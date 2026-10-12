package main

import "strings"

func (a *app) pageHTML() (string, error) {
	page, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		return "", err
	}
	script, err := webFiles.ReadFile("web/queue.js")
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	theme := a.theme
	a.mu.Unlock()
	return strings.NewReplacer("__TOKEN__", a.token, "__THEME__", theme, "__QUEUE_SCRIPT__", string(script)).Replace(string(page)), nil
}
