package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func preferenceApp(t *testing.T) (*app, *http.ServeMux) {
	t.Helper()
	root := t.TempDir()
	a := &app{token: "test-token", directory: filepath.Join(root, "bin"), outputDefault: root, theme: "system"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/theme", a.apiTheme)
	mux.HandleFunc("/api/clipboard", a.apiClipboard)
	mux.HandleFunc("/api/browse-folder", a.apiBrowseFolder)
	return a, mux
}

func TestThemePersistsAndApplies(t *testing.T) {
	a, mux := preferenceApp(t)
	var applied string
	a.applyTheme = func(theme string) error { applied = theme; return nil }
	for _, theme := range []string{"dark", "light", "system"} {
		response := desktopRequest(mux, a.token, "POST", "/api/theme", `{"theme":"`+theme+`"}`)
		if response.Status != 200 {
			t.Fatalf("theme status: %d", response.Status)
		}
		if applied != theme || a.snapshot().Theme != theme || loadTheme(a.settingsPath()) != theme {
			t.Fatal("theme was not applied and retained")
		}
	}
	response := desktopRequest(mux, a.token, "POST", "/api/theme", `{"theme":"invalid"}`)
	if response.Status != 400 || loadTheme(a.settingsPath()) != "system" {
		t.Fatal("invalid preference altered the saved theme")
	}
}

func TestMissingOrCorruptThemeUsesSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if loadTheme(path) != "system" {
		t.Fatal("missing file should follow system theme")
	}
	for _, data := range []string{"{", `{"theme":"invalid"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if loadTheme(path) != "system" {
			t.Fatal("invalid preferences should follow system theme")
		}
	}
}

func TestNativeActionsRequireAuthorization(t *testing.T) {
	a, mux := preferenceApp(t)
	a.readClipboard = func() (string, error) { t.Fatal("unauthorized clipboard read"); return "", nil }
	a.chooseFolder = func(string) (string, error) { t.Fatal("unauthorized folder dialog"); return "", nil }
	a.applyTheme = func(string) error { t.Fatal("unauthorized theme change"); return nil }
	for _, path := range []string{"/api/clipboard", "/api/browse-folder", "/api/theme"} {
		response := desktopRequest(mux, "wrong-token", "POST", path, `{}`)
		if response.Status != 403 {
			t.Fatalf("%s: status %d", path, response.Status)
		}
	}
	if _, err := os.Stat(a.settingsPath()); !os.IsNotExist(err) {
		t.Fatal("unauthorized request wrote preferences")
	}
}

func TestClipboardResponseAndErrors(t *testing.T) {
	a, mux := preferenceApp(t)
	calls := 0
	a.readClipboard = func() (string, error) { calls++; return "https://example.com/video?title=á", nil }
	response := desktopRequest(mux, a.token, "POST", "/api/clipboard", `{}`)
	var result struct {
		Text string `json:"text"`
	}
	if response.Status != 200 || json.Unmarshal(response.Body, &result) != nil || result.Text != "https://example.com/video?title=á" || calls != 1 {
		t.Fatal("clipboard text was not delivered intact")
	}
	a.readClipboard = func() (string, error) { return "", errors.New("El portapapeles está ocupado") }
	response = desktopRequest(mux, a.token, "POST", "/api/clipboard", `{}`)
	if response.Status != 400 || !json.Valid(response.Body) {
		t.Fatal("clipboard error was not delivered")
	}
}

func TestFolderSelectionAndCancel(t *testing.T) {
	a, mux := preferenceApp(t)
	selected := filepath.Join(a.outputDefault, "Carpeta con espacio y ñ")
	if err := os.Mkdir(selected, 0700); err != nil {
		t.Fatal(err)
	}
	a.chooseFolder = func(initial string) (string, error) {
		if initial != a.outputDefault {
			t.Fatal("dialog did not receive the initial folder")
		}
		return selected, nil
	}
	response := desktopRequest(mux, a.token, "POST", "/api/browse-folder", `{"folder":""}`)
	var result struct {
		Folder   string `json:"folder"`
		Canceled bool   `json:"canceled"`
	}
	if response.Status != 200 || json.Unmarshal(response.Body, &result) != nil || result.Folder != selected || result.Canceled {
		t.Fatal("folder selection was not returned")
	}
	a.chooseFolder = func(string) (string, error) { return "", nil }
	response = desktopRequest(mux, a.token, "POST", "/api/browse-folder", `{"folder":""}`)
	if response.Status != 200 || json.Unmarshal(response.Body, &result) != nil || !result.Canceled || result.Folder != "" {
		t.Fatal("cancel must leave the current folder unchanged")
	}
	a.chooseFolder = func(string) (string, error) { return selected + "/missing", nil }
	response = desktopRequest(mux, a.token, "POST", "/api/browse-folder", `{"folder":""}`)
	if response.Status != 400 {
		t.Fatal("nonexistent folder was accepted")
	}
}
