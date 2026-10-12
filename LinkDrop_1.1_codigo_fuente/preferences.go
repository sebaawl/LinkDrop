package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type preferences struct {
	Theme string `json:"theme"`
}

func validTheme(theme string) bool {
	return theme == "system" || theme == "light" || theme == "dark"
}

func (a *app) settingsPath() string {
	return filepath.Join(filepath.Dir(a.directory), "settings.json")
}

func loadTheme(path string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		var settings preferences
		if json.Unmarshal(data, &settings) == nil && validTheme(settings.Theme) {
			return settings.Theme
		}
	}
	return "system"
}

func saveTheme(path, theme string) error {
	return saveJSON(path, preferences{Theme: theme})
}

func saveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(value); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (a *app) apiTheme(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request preferences
	if json.NewDecoder(r.Body).Decode(&request) != nil || !validTheme(request.Theme) {
		sendError(w, 400, "Tema inválido")
		return
	}
	if err := saveTheme(a.settingsPath(), request.Theme); err != nil {
		sendError(w, 500, "No se pudo guardar el tema")
		return
	}
	if a.applyTheme != nil {
		if err := a.applyTheme(request.Theme); err != nil {
			sendError(w, 500, "No se pudo aplicar el tema")
			return
		}
	}
	a.mu.Lock()
	a.theme = request.Theme
	a.mu.Unlock()
	sendJSON(w, 200, request)
}

func (a *app) apiClipboard(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	if a.readClipboard == nil {
		sendError(w, 501, "El portapapeles nativo está disponible en la aplicación Windows")
		return
	}
	text, err := a.readClipboard()
	if err != nil {
		sendError(w, 400, err.Error())
		return
	}
	sendJSON(w, 200, map[string]string{"text": text})
}

func (a *app) apiBrowseFolder(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		Folder string `json:"folder"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	initial := strings.TrimSpace(request.Folder)
	if initial == "" {
		initial = a.outputDefault
	}
	if !filepath.IsAbs(initial) {
		sendError(w, 400, "Carpeta inválida")
		return
	}
	if a.chooseFolder == nil {
		sendError(w, 501, "El selector de carpetas está disponible en la aplicación Windows")
		return
	}
	folder, err := a.chooseFolder(initial)
	if err != nil {
		sendError(w, 500, err.Error())
		return
	}
	if folder == "" {
		sendJSON(w, 200, map[string]any{"canceled": true, "folder": ""})
		return
	}
	info, err := os.Stat(folder)
	if !filepath.IsAbs(folder) || err != nil || !info.IsDir() {
		sendError(w, 400, "La carpeta seleccionada no es válida")
		return
	}
	sendJSON(w, 200, map[string]any{"canceled": false, "folder": folder})
}
