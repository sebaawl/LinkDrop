package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// The desktop bridge reuses the same handlers entirely in memory. Windows
// does not start an HTTP server or navigate to a localhost URL.
type desktopResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type desktopWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *desktopWriter) Header() http.Header { return w.header }
func (w *desktopWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *desktopWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func desktopRequest(handler http.Handler, token, method, path, body string) desktopResponse {
	allowed := map[string]string{
		"/api/status":         http.MethodGet,
		"/api/setup":          http.MethodPost,
		"/api/info":           http.MethodPost,
		"/api/download":       http.MethodPost,
		"/api/cancel":         http.MethodPost,
		"/api/open-folder":    http.MethodPost,
		"/api/browse-folder":  http.MethodPost,
		"/api/clipboard":      http.MethodPost,
		"/api/theme":          http.MethodPost,
		"/api/quit":           http.MethodPost,
		"/api/youtube/search": http.MethodPost,
		"/api/queue":          http.MethodGet,
		"/api/queue/add":      http.MethodPost,
		"/api/queue/config":   http.MethodPost,
		"/api/queue/item":     http.MethodPost,
		"/api/queue/control":  http.MethodPost,
	}
	if expected, ok := allowed[path]; !ok || method != expected {
		return desktopResponse{http.StatusBadRequest, json.RawMessage(`{"error":"Operación no permitida"}`)}
	}
	if len(body) > 64*1024 {
		return desktopResponse{http.StatusRequestEntityTooLarge, json.RawMessage(`{"error":"Petición demasiado grande"}`)}
	}
	request, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		return desktopResponse{http.StatusBadRequest, json.RawMessage(`{"error":"Petición inválida"}`)}
	}
	request.Header.Set("X-LinkDrop-Token", token)
	request.Header.Set("Content-Type", "application/json")
	w := &desktopWriter{header: make(http.Header)}
	handler.ServeHTTP(w, request)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	result := w.body.Bytes()
	if !json.Valid(result) {
		result, _ = json.Marshal(map[string]string{"error": strings.TrimSpace(string(result))})
	}
	return desktopResponse{w.status, result}
}
