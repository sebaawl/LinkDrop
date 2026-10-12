package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

type youtubeVideo struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Uploader  string  `json:"uploader"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail"`
}

func canonicalVideoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if err := validURL(raw); err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	var id string
	switch host {
	case "youtu.be", "www.youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		id = u.Query().Get("v")
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if id == "" && len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "live" || parts[0] == "embed") {
			id = parts[1]
		}
	default:
		return raw, nil
	}
	if !youtubeID.MatchString(id) {
		return "", errors.New("Pega el enlace de un video individual de YouTube")
	}
	return "https://www.youtube.com/watch?v=" + id, nil
}

func parseYouTubeResults(data []byte) ([]youtubeVideo, error) {
	var response struct {
		Entries []struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			Uploader   string  `json:"uploader"`
			Channel    string  `json:"channel"`
			Duration   float64 `json:"duration"`
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if response.Entries == nil {
		return nil, errors.New("No se recibió una lista de resultados")
	}
	videos := make([]youtubeVideo, 0, len(response.Entries))
	seen := map[string]bool{}
	for _, entry := range response.Entries {
		if !youtubeID.MatchString(entry.ID) || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		thumbnail := "https://i.ytimg.com/vi/" + entry.ID + "/hqdefault.jpg"
		for _, image := range entry.Thumbnails {
			if strings.HasPrefix(image.URL, "https://") {
				thumbnail = image.URL
			}
		}
		uploader := entry.Uploader
		if uploader == "" {
			uploader = entry.Channel
		}
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			title = "Video de YouTube"
		}
		videos = append(videos, youtubeVideo{URL: "https://www.youtube.com/watch?v=" + entry.ID, Title: title, Uploader: uploader, Duration: entry.Duration, Thumbnail: thumbnail})
	}
	return videos, nil
}

func (a *app) apiYouTubeSearch(w http.ResponseWriter, r *http.Request) {
	if !a.authPost(w, r) {
		return
	}
	var request struct {
		Query string `json:"query"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		sendError(w, 400, "Datos inválidos")
		return
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || len([]rune(request.Query)) > 200 {
		sendError(w, 400, "Escribe una búsqueda de hasta 200 caracteres")
		return
	}
	engine := a.ytDLPPath()
	if engine == "" {
		sendError(w, 409, "Primero prepara los componentes")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, engine, "--ignore-config", "--encoding", "utf-8", "--flat-playlist", "--dump-single-json", "--skip-download", "--no-warnings", "--socket-timeout", "15", "--retries", "1", "--extractor-retries", "1", "--", "ytsearch20:"+request.Query)
	hideCmdWindow(cmd)
	data, err := cmd.Output()
	if err != nil {
		sendError(w, 502, "No se pudo buscar en YouTube. Comprueba la conexión y vuelve a intentarlo.")
		return
	}
	results, err := parseYouTubeResults(data)
	if err != nil {
		sendError(w, 502, "YouTube no devolvió resultados válidos")
		return
	}
	sendJSON(w, 200, map[string]any{"results": results, "query": request.Query})
}
