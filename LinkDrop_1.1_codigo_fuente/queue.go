package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxQueueItems = 500

type queueItem struct {
	MetadataPending bool            `json:"metadataPending"`
	ID              string          `json:"id"`
	URL             string          `json:"url"`
	Title           string          `json:"title"`
	Thumbnail       string          `json:"thumbnail"`
	Uploader        string          `json:"uploader"`
	UseGlobal       bool            `json:"useGlobal"`
	Settings        downloadRequest `json:"settings"`
	State           string          `json:"state"`
	Progress        float64         `json:"progress"`
	Error           string          `json:"error"`
	File            string          `json:"file"`
}
type queueData struct {
	Global downloadRequest `json:"global"`
	Items  []queueItem     `json:"items"`
}
type queueSnapshot struct {
	queueData
	Running      bool   `json:"running"`
	Current      string `json:"current"`
	StorageError string `json:"storageError"`
}

func defaultDownloadSettings() downloadRequest {
	return downloadRequest{Mode: "video", VideoFormat: "mp4", Resolution: "best", AudioFormat: "mp3", AudioQuality: "192K"}
}
func (a *app) queuePath() string { return filepath.Join(filepath.Dir(a.directory), "queue.json") }
func (a *app) ensureQueueLocked() {
	if a.queue.Global.Mode == "" {
		a.queue.Global = defaultDownloadSettings()
	}
	if a.queue.Items == nil {
		a.queue.Items = []queueItem{}
	}
}
func (a *app) loadQueue() {
	a.ensureQueueLocked()
	data, err := os.ReadFile(a.queuePath())
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		a.queueStorageError = "No se pudo leer la cola guardada"
		return
	}
	var saved queueData
	if json.Unmarshal(data, &saved) != nil || validateSettings(saved.Global) != nil || len(saved.Items) > maxQueueItems {
		a.queueStorageError = "No se pudo cargar la cola guardada"
		return
	}
	seen := map[string]bool{}
	for i := range saved.Items {
		item := &saved.Items[i]
		if item.ID == "" || seen[item.ID] || validateSettings(item.Settings) != nil {
			a.queueStorageError = "La cola guardada contiene datos inválidos"
			return
		}
		if _, err := canonicalVideoURL(item.URL); err != nil {
			a.queueStorageError = "La cola guardada contiene enlaces inválidos"
			return
		}
		seen[item.ID] = true
		switch item.State {
		case "downloading":
			item.State = "pending"
			item.Progress = 0
			item.Error = ""
		case "pending", "completed", "failed", "canceled":
		default:
			item.State = "pending"
		}
	}
	a.queue = saved
	a.ensureQueueLocked()
}
func validateSettings(settings downloadRequest) error {
	if _, err := makeArgs(settings, ""); err != nil {
		return err
	}
	if folder := strings.TrimSpace(settings.Folder); folder != "" && !filepath.IsAbs(folder) {
		return errors.New("Elige una ruta completa para la carpeta de destino")
	}
	return nil
}
func (a *app) saveQueueLocked() error {
	err := saveJSON(a.queuePath(), a.queue)
	if err != nil {
		a.queueStorageError = "No se pudo guardar la cola. Mantén la aplicación abierta para conservar los cambios."
	} else {
		a.queueStorageError = ""
	}
	return err
}
func (a *app) copyQueueLocked() queueData {
	return queueData{Global: a.queue.Global, Items: append([]queueItem{}, a.queue.Items...)}
}
func (a *app) saveQueueChangeLocked(previous queueData) error {
	err := a.saveQueueLocked()
	if err != nil {
		a.queue = previous
	}
	return err
}
func (a *app) queueState() queueSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureQueueLocked()
	items := append([]queueItem{}, a.queue.Items...)
	for i := range items {
		if items[i].ID == a.queueCurrent && items[i].State == "downloading" {
			items[i].Progress = a.jobPercent
			if a.jobTitle != "" {
				items[i].Title = a.jobTitle
			}
		}
	}
	return queueSnapshot{queueData: queueData{Global: a.queue.Global, Items: items}, Running: a.queueRunning, Current: a.queueCurrent, StorageError: a.queueStorageError}
}
func (a *app) apiQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || !a.authorized(r) {
		http.Error(w, "Acceso denegado", 403)
		return
	}
	sendJSON(w, 200, a.queueState())
}

func (a *app) apiQueueAdd(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		Items []youtubeVideo `json:"items"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Items) == 0 || len(request.Items) > maxQueueItems {
		sendError(w, 400, "Añade entre 1 y 500 enlaces")
		return
	}
	prepared := make([]queueItem, 0, len(request.Items))
	for i, item := range request.Items {
		videoURL, err := canonicalVideoURL(item.URL)
		if err != nil {
			sendError(w, 400, fmt.Sprintf("Enlace %d: %s", i+1, err))
			return
		}
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			sendError(w, 500, "No se pudo añadir el video")
			return
		}
		title := strings.TrimSpace(item.Title)
		if len([]rune(title)) > 500 {
			sendError(w, 400, "El título es demasiado largo")
			return
		}
		if title == "" {
			title = "Video añadido por enlace"
		}
		thumbnail := item.Thumbnail
		if !strings.HasPrefix(thumbnail, "https://") {
			thumbnail = ""
		}
		prepared = append(prepared, queueItem{ID: hex.EncodeToString(id[:]), URL: videoURL, Title: title, MetadataPending: strings.TrimSpace(item.Title) == "", Thumbnail: thumbnail, Uploader: item.Uploader, State: "pending", UseGlobal: true, Settings: defaultDownloadSettings()})
	}
	a.mu.Lock()
	a.ensureQueueLocked()
	added, duplicates := 0, 0
	seen := map[string]bool{}
	for _, item := range a.queue.Items {
		if item.State == "pending" || item.State == "downloading" {
			seen[item.URL] = true
		}
	}
	unique := make([]queueItem, 0, len(prepared))
	for _, item := range prepared {
		if seen[item.URL] {
			duplicates++
			continue
		}
		seen[item.URL] = true
		unique = append(unique, item)
	}
	if len(a.queue.Items)+len(unique) > maxQueueItems {
		a.mu.Unlock()
		sendError(w, 400, "La cola admite hasta 500 videos. Elimina los terminados antes de añadir más.")
		return
	}
	previous := a.copyQueueLocked()
	a.queue.Items = append(a.queue.Items, unique...)
	added = len(unique)
	err := a.saveQueueChangeLocked(previous)
	a.mu.Unlock()
	if err != nil {
		sendError(w, 500, "No se pudo guardar la cola")
		return
	}
	for _, item := range unique {
		a.scheduleMetadata(item)
	}
	sendJSON(w, 200, map[string]int{"added": added, "duplicates": duplicates})
}

func (a *app) apiQueueConfig(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		Settings   downloadRequest `json:"settings"`
		ApplyToAll bool            `json:"applyToAll"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	request.Settings.URL = ""
	request.Settings.Folder = strings.TrimSpace(request.Settings.Folder)
	if err := validateSettings(request.Settings); err != nil {
		sendError(w, 400, err.Error())
		return
	}
	a.mu.Lock()
	a.ensureQueueLocked()
	previous := a.copyQueueLocked()
	a.queue.Global = request.Settings
	if request.ApplyToAll {
		for i := range a.queue.Items {
			if a.queue.Items[i].State != "downloading" && a.queue.Items[i].State != "completed" {
				a.queue.Items[i].UseGlobal = true
			}
		}
	}
	err := a.saveQueueChangeLocked(previous)
	a.mu.Unlock()
	if err != nil {
		sendError(w, 500, "No se pudo guardar la configuración")
		return
	}
	sendJSON(w, 200, map[string]string{"message": "Configuración guardada"})
}

func (a *app) apiQueueItem(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		ID        string          `json:"id"`
		Action    string          `json:"action"`
		UseGlobal bool            `json:"useGlobal"`
		Settings  downloadRequest `json:"settings"`
		Title     string          `json:"title"`
		Thumbnail string          `json:"thumbnail"`
		Uploader  string          `json:"uploader"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	if request.Action == "configure" {
		request.Settings.URL = ""
		if err := validateSettings(request.Settings); err != nil {
			sendError(w, 400, err.Error())
			return
		}
	}
	a.mu.Lock()
	a.ensureQueueLocked()
	index := -1
	for i, item := range a.queue.Items {
		if item.ID == request.ID {
			index = i
			break
		}
	}
	if index < 0 {
		a.mu.Unlock()
		sendError(w, 404, "El video ya no está en la cola")
		return
	}
	item := &a.queue.Items[index]
	if item.State == "downloading" {
		a.mu.Unlock()
		sendError(w, 409, "No puedes modificar el video que se está descargando")
		return
	}
	if item.State == "completed" && request.Action == "configure" {
		a.mu.Unlock()
		sendError(w, 409, "Vuelve a poner el video pendiente antes de editar sus opciones")
		return
	}
	previous := a.copyQueueLocked()
	switch request.Action {
	case "configure":
		item.Settings = request.Settings
		item.UseGlobal = request.UseGlobal
	case "metadata":
		item.MetadataPending = false
		if title := strings.TrimSpace(request.Title); title != "" {
			item.Title = title
		}
		if strings.HasPrefix(request.Thumbnail, "https://") {
			item.Thumbnail = request.Thumbnail
		}
		item.Uploader = request.Uploader
	case "remove":
		a.queue.Items = append(a.queue.Items[:index], a.queue.Items[index+1:]...)
	case "retry":
		item.State = "pending"
		item.Progress = 0
		item.Error = ""
		item.File = ""
	case "up":
		if index > 0 && a.queue.Items[index-1].State != "downloading" {
			a.queue.Items[index], a.queue.Items[index-1] = a.queue.Items[index-1], a.queue.Items[index]
		}
	case "down":
		if index+1 < len(a.queue.Items) && a.queue.Items[index+1].State != "downloading" {
			a.queue.Items[index], a.queue.Items[index+1] = a.queue.Items[index+1], a.queue.Items[index]
		}
	default:
		a.mu.Unlock()
		sendError(w, 400, "Operación inválida")
		return
	}
	err := a.saveQueueChangeLocked(previous)
	a.mu.Unlock()
	if err != nil {
		sendError(w, 500, "No se pudo guardar la cola")
		return
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}

func (a *app) apiQueueControl(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		Action string `json:"action"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	a.mu.Lock()
	a.ensureQueueLocked()
	previous := a.copyQueueLocked()
	switch request.Action {
	case "start":
		if a.queueRunning || a.jobActive || a.setupActive || !a.installed {
			a.mu.Unlock()
			sendError(w, 409, "Prepara los componentes y espera a que termine la operación actual")
			return
		}
		pending := false
		for _, item := range a.queue.Items {
			if item.State == "pending" {
				pending = true
				break
			}
		}
		if !pending {
			a.mu.Unlock()
			sendError(w, 400, "No hay videos pendientes en la cola")
			return
		}
		done := make(chan struct{})
		a.queueDone = done
		a.queueRunning = true
		a.queueStop = false
		a.mu.Unlock()
		go func() { defer close(done); a.runQueue() }()
		sendJSON(w, 202, map[string]string{"message": "Cola iniciada"})
		return
	case "stop":
		if !a.queueRunning {
			a.mu.Unlock()
			sendError(w, 409, "La cola no está en curso")
			return
		}
		a.queueStop = true
		cancel := a.cancel
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		sendJSON(w, 200, map[string]string{"message": "Deteniendo cola"})
		return
	case "clear-completed":
		kept := make([]queueItem, 0, len(a.queue.Items))
		for _, item := range a.queue.Items {
			if item.State != "completed" {
				kept = append(kept, item)
			}
		}
		a.queue.Items = kept
	case "clear":
		if a.queueRunning {
			a.mu.Unlock()
			sendError(w, 409, "Detén la cola antes de vaciarla")
			return
		}
		a.queue.Items = []queueItem{}
	default:
		a.mu.Unlock()
		sendError(w, 400, "Operación inválida")
		return
	}
	err := a.saveQueueChangeLocked(previous)
	a.mu.Unlock()
	if err != nil {
		sendError(w, 500, "No se pudo guardar la cola")
		return
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}

func (a *app) runQueue() {
	defer func() { a.mu.Lock(); a.queueRunning = false; a.queueCurrent = ""; a.mu.Unlock() }()
	for {
		a.mu.Lock()
		if a.queueStop {
			a.mu.Unlock()
			return
		}
		index := -1
		for i, item := range a.queue.Items {
			if item.State == "pending" {
				index = i
				break
			}
		}
		if index < 0 {
			a.mu.Unlock()
			return
		}
		item := a.queue.Items[index]
		settings := item.Settings
		if item.UseGlobal {
			settings = a.queue.Global
		}
		settings.URL = item.URL
		a.queue.Items[index].Settings = settings
		a.queue.Items[index].Settings.URL = ""
		a.queue.Items[index].State = "downloading"
		a.queue.Items[index].Progress = 0
		a.jobPercent = 0
		a.jobTitle = ""
		a.queueCurrent = item.ID
		a.saveQueueLocked()
		a.mu.Unlock()
		done, err := a.startDownload(settings, true)
		if err == nil {
			<-done
		}
		a.mu.Lock()
		for i := range a.queue.Items {
			if a.queue.Items[i].ID == item.ID {
				current := &a.queue.Items[i]
				switch {
				case a.queueStop:
					current.State = "canceled"
					current.Error = "Descarga detenida"
				case err != nil:
					current.State = "failed"
					current.Error = err.Error()
				case a.jobDone:
					current.State = "completed"
					current.Progress = 100
					current.File = a.jobFile
					current.Error = ""
				default:
					current.State = "failed"
					current.Error = a.jobError
					current.Progress = a.jobPercent
				}
				if err == nil && a.jobTitle != "" {
					current.Title = a.jobTitle
				}
				break
			}
		}
		a.queueCurrent = ""
		a.saveQueueLocked()
		stop := a.queueStop
		a.mu.Unlock()
		if stop {
			return
		}
	}
}
