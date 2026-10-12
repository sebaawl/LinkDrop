package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

const appName = "LinkDrop"
const appVersion = "1.1.0"
const ytDLPURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
const ffmpegURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
const ffmpegSmallURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.7z"

var progressRX = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*%`)

type app struct {
	mu            sync.Mutex
	token         string
	directory     string
	outputDefault string
	setupActive   bool
	setupStatus   string
	setupPercent  float64
	installed     bool
	jobActive     bool
	jobStatus     string
	jobPercent    float64
	jobSpeed      string
	jobETA        string
	jobTitle      string
	jobError      string
	jobDone       bool
	jobFile       string
	logLines      []string
	cancel        context.CancelFunc
}

type status struct {
	Ready          bool     `json:"ready"`
	Installing     bool     `json:"installing"`
	SetupStatus    string   `json:"setupStatus"`
	SetupPercent   float64  `json:"setupPercent"`
	Downloading    bool     `json:"downloading"`
	JobStatus      string   `json:"jobStatus"`
	JobPercent     float64  `json:"jobPercent"`
	JobSpeed       string   `json:"jobSpeed"`
	JobETA         string   `json:"jobETA"`
	JobTitle       string   `json:"jobTitle"`
	JobError       string   `json:"jobError"`
	JobDone        bool     `json:"jobDone"`
	JobFile        string   `json:"jobFile"`
	Logs           []string `json:"logs"`
	DownloadFolder string   `json:"downloadFolder"`
	Version        string   `json:"version"`
}

type downloadRequest struct {
	URL          string `json:"url"`
	Mode         string `json:"mode"`
	VideoFormat  string `json:"videoFormat"`
	Resolution   string `json:"resolution"`
	AudioFormat  string `json:"audioFormat"`
	AudioQuality string `json:"audioQuality"`
	Folder       string `json:"folder"`
}

type infoResult struct {
	Title       string `json:"title"`
	Uploader    string `json:"uploader"`
	Duration    int    `json:"duration"`
	Thumbnail   string `json:"thumbnail"`
	Resolutions []int  `json:"resolutions"`
}

func main() {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatal(err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	dir := filepath.Join(configDir, appName, "bin")
	home, _ := os.UserHomeDir()
	defaultFolder := filepath.Join(home, "Downloads")
	a := &app{token: hex.EncodeToString(tokenBytes), directory: dir, outputDefault: defaultFolder}
	a.installed = a.checkEngines()
	if a.installed {
		a.setupStatus = "Componentes listos"
	} else {
		a.setupStatus = "Faltan componentes. La descarga rápida de FFmpeg usa el archivo compacto de 34 MB."
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("/api/status", a.apiStatus)
	mux.HandleFunc("/api/setup", a.apiSetup)
	mux.HandleFunc("/api/info", a.apiInfo)
	mux.HandleFunc("/api/download", a.apiDownload)
	mux.HandleFunc("/api/cancel", a.apiCancel)
	mux.HandleFunc("/api/open-folder", a.apiOpenFolder)
	mux.HandleFunc("/api/quit", a.apiQuit)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	link := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", listener.Addr().(*net.TCPAddr).Port, a.token)
	go func() {
		time.Sleep(250 * time.Millisecond)
		if err := openBrowser(link); err != nil {
			log.Printf("Open %s manually: %v", link, err)
		}
	}()
	log.Println("LinkDrop available at", link)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatal(err)
	}
}

func (a *app) authorized(r *http.Request) bool { return r.Header.Get("X-LinkDrop-Token") == a.token }
func (a *app) authPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return false
	}
	if !a.authorized(r) {
		http.Error(w, "Acceso denegado", http.StatusForbidden)
		return false
	}
	if r.ContentLength > 64*1024 {
		http.Error(w, "Petición demasiado grande", http.StatusRequestEntityTooLarge)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	return true
}
func sendJSON(w http.ResponseWriter, status int, obj any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(obj)
}
func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

func (a *app) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("t") != a.token {
		http.Error(w, "URL no válida", http.StatusForbidden)
		return
	}
	b, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "UI no disponible", 500)
		return
	}
	html := strings.ReplaceAll(string(b), "__TOKEN__", a.token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src https: data:; connect-src 'self'; base-uri 'none'; form-action 'none'")
	_, _ = io.WriteString(w, html)
}

// A portable distribution can carry the authentic Windows binaries in a
// sibling bin/ directory. We also reuse earlier installs, or components that
// are already installed on Windows, rather than re-downloading them.
func validEngine(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 1024
}
func (a *app) portableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "bin")
}
func (a *app) engineDirs() []string {
	dirs := []string{a.portableDir(), a.directory}
	// os.LookPath can locate executables installed with winget, Scoop, or a
	// conventional PATH entry. Do not automatically invoke any updater.
	return dirs
}
func (a *app) ytDLPPath() string {
	for _, dir := range a.engineDirs() {
		if dir != "" {
			f := filepath.Join(dir, "yt-dlp.exe")
			if validEngine(f) {
				return f
			}
		}
	}
	if f, err := exec.LookPath("yt-dlp.exe"); err == nil && validEngine(f) {
		return f
	}
	return ""
}
func (a *app) ffmpegDir() string {
	for _, dir := range a.engineDirs() {
		if dir != "" && validEngine(filepath.Join(dir, "ffmpeg.exe")) && validEngine(filepath.Join(dir, "ffprobe.exe")) {
			return dir
		}
	}
	ff, e1 := exec.LookPath("ffmpeg.exe")
	fp, e2 := exec.LookPath("ffprobe.exe")
	if e1 == nil && e2 == nil && validEngine(ff) && validEngine(fp) {
		// yt-dlp uses PATH with ffmpeg present, but using the directory when
		// both tools live alongside each other is deterministic.
		if filepath.Dir(ff) == filepath.Dir(fp) {
			return filepath.Dir(ff)
		}
	}
	return ""
}
func (a *app) checkEngines() bool { return a.ytDLPPath() != "" && a.ffmpegDir() != "" }

func (a *app) snapshot() status {
	a.mu.Lock()
	defer a.mu.Unlock()
	logs := append([]string(nil), a.logLines...)
	return status{Ready: a.installed, Installing: a.setupActive, SetupStatus: a.setupStatus, SetupPercent: a.setupPercent,
		Downloading: a.jobActive, JobStatus: a.jobStatus, JobPercent: a.jobPercent, JobSpeed: a.jobSpeed,
		JobETA: a.jobETA, JobTitle: a.jobTitle, JobError: a.jobError, JobDone: a.jobDone, JobFile: a.jobFile,
		Logs: logs, DownloadFolder: a.outputDefault, Version: appVersion}
}
func (a *app) apiStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || !a.authorized(r) {
		http.Error(w, "Acceso denegado", 403)
		return
	}
	sendJSON(w, 200, a.snapshot())
}
func (a *app) setSetup(status string, p float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setupStatus = status
	a.setupPercent = p
}
func (a *app) log(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(line) > 360 {
		line = line[:360] + "…"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logLines = append(a.logLines, line)
	if len(a.logLines) > 70 {
		a.logLines = a.logLines[len(a.logLines)-70:]
	}
}
func (a *app) apiSetup(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	a.mu.Lock()
	if a.setupActive || a.jobActive {
		a.mu.Unlock()
		sendError(w, 409, "Ya hay una operación en curso")
		return
	}
	a.setupActive = true
	a.setupPercent = 0
	a.setupStatus = "Preparando..."
	a.mu.Unlock()
	go a.installEngines()
	sendJSON(w, 202, map[string]string{"message": "Preparación iniciada"})
}
func (a *app) installEngines() {
	defer func() { a.mu.Lock(); a.setupActive = false; a.installed = a.checkEngines(); a.mu.Unlock() }()
	if a.checkEngines() {
		a.setSetup("Los componentes ya están disponibles, sin descargar nada", 100)
		return
	}
	if err := os.MkdirAll(a.directory, 0755); err != nil {
		a.setSetup("Error: "+err.Error(), 0)
		return
	}
	if a.ytDLPPath() == "" {
		if err := a.downloadFile(ytDLPURL, filepath.Join(a.directory, "yt-dlp.exe"), "Descargando yt-dlp", 0, 35); err != nil {
			a.setSetup("Error con yt-dlp: "+err.Error(), 0)
			return
		}
	} else {
		a.setSetup("yt-dlp existente, sin descargar de nuevo", 35)
	}
	if a.ffmpegDir() == "" {
		a.setSetup("Descargando FFmpeg compacto (aprox. 34 MB)", 38)
		archive := filepath.Join(a.directory, "ffmpeg-small.7z")
		if _, err := exec.LookPath("tar.exe"); err == nil {
			// Windows includes bsdtar/libarchive; recent Windows versions
			// support extraction of 7z archives. If not, use the ZIP fallback.
			err := a.downloadFile(ffmpegSmallURL, archive, "Descargando FFmpeg compacto", 38, 86)
			if err == nil {
				a.setSetup("Extrayendo FFmpeg compacto", 89)
				err = unpackFFmpeg7z(archive, a.directory)
			}
			_ = os.Remove(archive)
			if err != nil {
				a.log("No se pudo abrir el paquete compacto: " + err.Error() + "; probando ZIP.")
			}
		}
		if a.ffmpegDir() == "" {
			archive = filepath.Join(a.directory, "ffmpeg.zip")
			defer os.Remove(archive)
			if err := a.downloadFile(ffmpegURL, archive, "Descargando FFmpeg ZIP alternativo", 38, 87); err != nil {
				a.setSetup("Error al descargar FFmpeg: "+err.Error(), 0)
				return
			}
			a.setSetup("Extrayendo FFmpeg", 90)
			if err := unpackFFmpeg(archive, a.directory); err != nil {
				a.setSetup("Error al extraer FFmpeg: "+err.Error(), 0)
				return
			}
		}
	} else {
		a.setSetup("FFmpeg encontrado; no hace falta descargarlo", 90)
	}
	if !a.checkEngines() {
		a.setSetup("No se encontraron los ejecutables necesarios", 0)
		return
	}
	a.setSetup("Componentes listos. No tendrás que repetir este proceso.", 100)
	a.log("Motor de descarga listo. Puedes pegar un enlace.")
}

// Extract a 7z archive with Windows' built-in bsdtar. No external 7zip
// installation is needed when the Windows tar version supports 7z.
func unpackFFmpeg7z(archivePath, destination string) error {
	tmp, err := os.MkdirTemp(destination, "ffmpeg-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tar.exe", "-xf", archivePath, "-C", tmp)
	hideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	found := map[string]string{}
	err = filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if (name == "ffmpeg.exe" || name == "ffprobe.exe") && info.Size() > 1024 && info.Size() < 250*1024*1024 {
			found[name] = path
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) != 2 {
		return errors.New("El paquete no contiene FFmpeg y FFprobe")
	}
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		// Staying on the same volume makes renaming efficient and atomic.
		target := filepath.Join(destination, name)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(found[name], target); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) downloadFile(src, destination, label string, low, high float64) error {
	// Download the engine only from its trusted, fixed HTTPS URL.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "LinkDrop/1.0")
	cli := &http.Client{Timeout: 0}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := destination + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	buffer := make([]byte, 256*1024)
	var nread int64
	last := time.Time{}
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := f.Write(buffer[:n]); err != nil {
				f.Close()
				return err
			}
			nread += int64(n)
			if time.Since(last) > 200*time.Millisecond {
				pc := low
				extra := ""
				if resp.ContentLength > 0 {
					pc += float64(nread) / float64(resp.ContentLength) * (high - low)
					extra = fmt.Sprintf(" • %.0f%%", float64(nread)/float64(resp.ContentLength)*100)
				} else {
					extra = fmt.Sprintf(" • %.1f MB", float64(nread)/1048576)
				}
				a.setSetup(label+extra, pc)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			return readErr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if nread == 0 {
		return errors.New("Archivo vacío")
	}
	// On Windows os.Rename does not replace an existing destination.
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmp, destination); err != nil {
		return err
	}
	a.setSetup(label+" • terminado", high)
	return nil
}
func unpackFFmpeg(zipPath, destination string) error {
	file, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer file.Close()
	s, err := file.Stat()
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(file, s.Size())
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, part := range archive.File {
		name := strings.ToLower(filepath.Base(strings.ReplaceAll(part.Name, "\\", "/")))
		if name != "ffmpeg.exe" && name != "ffprobe.exe" {
			continue
		}
		if part.UncompressedSize64 > 250*1024*1024 {
			return errors.New("Binario inesperadamente grande")
		}
		r, err := part.Open()
		if err != nil {
			return err
		}
		target := filepath.Join(destination, name)
		tmp := target + ".partial"
		out, err := os.Create(tmp)
		if err != nil {
			r.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(r, 250*1024*1024+1))
		e1 := out.Close()
		e2 := r.Close()
		if err != nil {
			return err
		}
		if e1 != nil {
			return e1
		}
		if e2 != nil {
			return e2
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(tmp, target); err != nil {
			return err
		}
		found[name] = true
	}
	if !found["ffmpeg.exe"] || !found["ffprobe.exe"] {
		return errors.New("El ZIP no contiene ffmpeg.exe y ffprobe.exe")
	}
	return nil
}
func validURL(raw string) error {
	if raw == "" || len(raw) > 4096 {
		return errors.New("Pega una URL válida")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("Solo se permiten enlaces HTTP o HTTPS")
	}
	if u.User != nil {
		return errors.New("No se admiten credenciales en la URL")
	}
	return nil
}
func (a *app) apiInfo(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	request.URL = strings.TrimSpace(request.URL)
	if err := validURL(request.URL); err != nil {
		sendError(w, 400, err.Error())
		return
	}
	a.mu.Lock()
	available := a.installed
	busy := a.setupActive
	a.mu.Unlock()
	if !available || busy {
		sendError(w, 409, "Primero prepara los componentes")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.ytDLPPath(), "--ignore-config", "--no-playlist", "--dump-single-json", "--skip-download", "--no-warnings", "--", request.URL)
	hideCmdWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		msg := "No se pudo consultar el enlace. Puede ser privado, protegido o no compatible."
		var ex *exec.ExitError
		if errors.As(err, &ex) {
			detail := strings.TrimSpace(string(ex.Stderr))
			if len(detail) > 420 {
				detail = detail[len(detail)-420:]
			}
			if detail != "" {
				msg = detail
			}
		}
		sendError(w, 400, msg)
		return
	}
	var raw struct {
		Title     string  `json:"title"`
		Uploader  string  `json:"uploader"`
		Duration  float64 `json:"duration"`
		Thumbnail string  `json:"thumbnail"`
		Formats   []struct {
			Height int    `json:"height"`
			VCodec string `json:"vcodec"`
		} `json:"formats"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		sendError(w, 400, "No se pudieron leer los formatos")
		return
	}
	heights := map[int]bool{}
	for _, f := range raw.Formats {
		if f.Height >= 144 && f.Height <= 8640 && f.VCodec != "none" {
			heights[f.Height] = true
		}
	}
	keys := make([]int, 0, len(heights))
	for k := range heights {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	if len(keys) > 20 {
		keys = keys[:20]
	}
	sendJSON(w, 200, infoResult{Title: raw.Title, Uploader: raw.Uploader, Duration: int(raw.Duration), Thumbnail: raw.Thumbnail, Resolutions: keys})
}
func (a *app) apiDownload(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var req downloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if err := validURL(req.URL); err != nil {
		sendError(w, 400, err.Error())
		return
	}
	args, err := makeArgs(req, a.directory)
	if err != nil {
		sendError(w, 400, err.Error())
		return
	}
	folder := strings.TrimSpace(req.Folder)
	if folder == "" {
		folder = a.outputDefault
	}
	if !filepath.IsAbs(folder) {
		sendError(w, 400, "Elige una ruta completa para guardar")
		return
	}
	if err := os.MkdirAll(folder, 0755); err != nil {
		sendError(w, 400, "No se puede crear la carpeta: "+err.Error())
		return
	}
	a.mu.Lock()
	if !a.installed || a.setupActive {
		a.mu.Unlock()
		sendError(w, 409, "Primero prepara los componentes")
		return
	}
	if a.jobActive {
		a.mu.Unlock()
		sendError(w, 409, "Ya hay una descarga en curso")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.jobActive = true
	a.jobDone = false
	a.jobPercent = 0
	a.jobError = ""
	a.jobFile = ""
	a.jobStatus = "Conectando..."
	a.jobSpeed = ""
	a.jobETA = ""
	a.jobTitle = ""
	a.logLines = nil
	a.mu.Unlock()
	// Prevent one user's yt-dlp config from changing the app's behavior.
	args = append([]string{"--ignore-config", "--no-playlist", "--no-overwrites", "--no-simulate", "--progress", "--newline", "--no-color", "--ffmpeg-location", a.ffmpegDir(), "--paths", folder, "--output", "%(title).180B [%(id)s].%(ext)s", "--progress-template", "download:LDPROGRESS:%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s", "--print", "after_move:ARCHIVO_FINAL:%(filepath)s"}, args...)
	args = append(args, "--", req.URL)
	go a.runDownload(ctx, cancel, args)
	sendJSON(w, 202, map[string]string{"message": "Descarga iniciada"})
}
func makeArgs(r downloadRequest, engineDir string) ([]string, error) {
	args := []string{}
	switch r.Mode {
	case "audio":
		switch r.AudioFormat {
		case "mp3", "m4a", "flac", "wav", "opus", "aac":
		default:
			return nil, errors.New("Formato de audio no admitido")
		}
		switch r.AudioQuality {
		case "best", "320K", "256K", "192K", "128K", "96K":
		default:
			return nil, errors.New("Calidad de audio no admitida")
		}
		q := r.AudioQuality
		if q == "best" {
			q = "0"
		}
		args = append(args, "-f", "bestaudio/best", "--extract-audio", "--audio-format", r.AudioFormat, "--audio-quality", q)
	case "video":
		switch r.VideoFormat {
		case "mp4", "mkv", "webm":
		default:
			return nil, errors.New("Formato de video no admitido")
		}
		var height int
		if r.Resolution != "best" {
			val, e := strconv.Atoi(r.Resolution)
			if e != nil || val < 144 || val > 8640 {
				return nil, errors.New("Resolución no admitida")
			}
			height = val
		}
		constraint := ""
		if height > 0 {
			constraint = fmt.Sprintf("[height<=%d]", height)
		}
		selector := "bv*" + constraint + "+ba/b" + constraint
		if r.VideoFormat == "mp4" {
			selector = "bv*" + constraint + "[ext=mp4]+ba[ext=m4a]/bv*" + constraint + "+ba/b" + constraint
			args = append(args, "-f", selector, "--merge-output-format", "mp4", "--recode-video", "mp4")
		} else if r.VideoFormat == "webm" {
			selector = "bv*" + constraint + "[ext=webm]+ba[ext=webm]/bv*" + constraint + "+ba/b" + constraint
			args = append(args, "-f", selector, "--merge-output-format", "webm", "--recode-video", "webm")
		} else {
			args = append(args, "-f", selector, "--merge-output-format", "mkv", "--remux-video", "mkv")
		}
	default:
		return nil, errors.New("Elige video o audio")
	}
	return args, nil
}
func (a *app) updateProgress(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	a.log(line)
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.HasPrefix(line, "LDPROGRESS:") {
		values := strings.SplitN(strings.TrimPrefix(line, "LDPROGRESS:"), "|", 3)
		if len(values) > 0 {
			if m := progressRX.FindStringSubmatch(values[0]); len(m) == 2 {
				n, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
				a.jobPercent = n
			}
		}
		if len(values) > 1 {
			a.jobSpeed = strings.TrimSpace(values[1])
		}
		if len(values) > 2 {
			a.jobETA = strings.TrimSpace(values[2])
		}
		a.jobStatus = "Descargando..."
	} else if strings.HasPrefix(line, "ARCHIVO_FINAL:") {
		a.jobFile = strings.TrimPrefix(line, "ARCHIVO_FINAL:")
		a.jobPercent = 100
		a.jobStatus = "Archivo guardado"
	} else if strings.Contains(line, "[Merger]") || strings.Contains(line, "[ExtractAudio]") || strings.Contains(line, "[VideoConvertor]") || strings.Contains(line, "[VideoRemuxer]") {
		a.jobStatus = "Procesando archivo con FFmpeg..."
	} else if strings.Contains(line, "[download] Destination:") {
		a.jobTitle = strings.TrimSpace(strings.SplitN(line, "Destination:", 2)[1])
	}
}
func (a *app) runDownload(ctx context.Context, cancel context.CancelFunc, args []string) {
	defer cancel()
	defer func() { a.mu.Lock(); a.jobActive = false; a.cancel = nil; a.mu.Unlock() }()
	cmd := exec.CommandContext(ctx, a.ytDLPPath(), args...)
	hideCmdWindow(cmd)
	read, write := io.Pipe()
	cmd.Stdout = write
	cmd.Stderr = write
	if err := cmd.Start(); err != nil {
		write.Close()
		read.Close()
		a.mu.Lock()
		a.jobError = "No se pudo iniciar yt-dlp: " + err.Error()
		a.jobStatus = "Error"
		a.mu.Unlock()
		return
	}
	done := make(chan error, 1)
	go func() { err := cmd.Wait(); _ = write.Close(); done <- err }()
	scanner := bufio.NewScanner(read)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		a.updateProgress(scanner.Text())
	}
	_ = read.Close()
	err := <-done
	a.mu.Lock()
	defer a.mu.Unlock()
	if errors.Is(ctx.Err(), context.Canceled) {
		a.jobStatus = "Descarga cancelada"
		a.jobError = "Cancelada por el usuario"
		a.jobDone = false
		return
	}
	if err != nil {
		a.jobStatus = "No se pudo completar"
		a.jobError = "La descarga falló. Revisa el registro; el sitio puede no estar disponible, requerir inicio de sesión o usar DRM."
		a.jobDone = false
		return
	}
	a.jobDone = true
	a.jobPercent = 100
	a.jobStatus = "¡Descarga completada!"
}
func (a *app) apiCancel(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
		sendJSON(w, 200, map[string]string{"message": "Cancelando"})
	} else {
		sendError(w, 409, "No hay una descarga activa")
	}
}
func (a *app) apiOpenFolder(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var req struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	folder := strings.TrimSpace(req.Folder)
	if folder == "" {
		folder = a.outputDefault
	}
	if !filepath.IsAbs(folder) {
		sendError(w, 400, "Carpeta inválida")
		return
	}
	if _, err := os.Stat(folder); err != nil {
		sendError(w, 400, "La carpeta no existe")
		return
	}
	if err := openFolder(folder); err != nil {
		sendError(w, 500, err.Error())
		return
	}
	sendJSON(w, 200, map[string]string{"message": "Carpeta abierta"})
}
func (a *app) apiQuit(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	a.mu.Lock()
	busy := a.jobActive || a.setupActive
	a.mu.Unlock()
	if busy {
		sendError(w, 409, "Espera a que termine el proceso o cancela la descarga")
		return
	}
	sendJSON(w, 200, map[string]string{"message": "Cerrando"})
	go func() { time.Sleep(200 * time.Millisecond); os.Exit(0) }()
}
