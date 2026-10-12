package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// oEmbed supplies public titles/thumbnails for pasted YouTube links without
// starting one extractor process per queued link or requiring an API key.
type metadataClient interface {
	Do(*http.Request) (*http.Response, error)
}

func lookupYouTubeMetadata(ctx context.Context, raw string) (youtubeVideo, error) {
	return lookupYouTubeMetadataWithClient(ctx, raw, &http.Client{Timeout: 12 * time.Second})
}
func lookupYouTubeMetadataWithClient(ctx context.Context, raw string, client metadataClient) (youtubeVideo, error) {
	videoURL, err := canonicalVideoURL(raw)
	if err != nil {
		return youtubeVideo{}, err
	}
	u, _ := url.Parse(videoURL)
	if u.Hostname() != "www.youtube.com" {
		return youtubeVideo{}, errors.New("No es un video de YouTube")
	}
	endpoint := "https://www.youtube.com/oembed?format=json&url=" + url.QueryEscape(videoURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return youtubeVideo{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return youtubeVideo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return youtubeVideo{}, errors.New("YouTube no devolvió los datos del video")
	}
	var data struct {
		Title     string `json:"title"`
		Author    string `json:"author_name"`
		Thumbnail string `json:"thumbnail_url"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&data) != nil || strings.TrimSpace(data.Title) == "" {
		return youtubeVideo{}, errors.New("Datos del video inválidos")
	}
	return youtubeVideo{URL: videoURL, Title: data.Title, Uploader: data.Author, Thumbnail: data.Thumbnail}, nil
}
func (a *app) scheduleMetadata(item queueItem) {
	if !item.MetadataPending || a.metadataJobs == nil {
		return
	}
	select {
	case a.metadataJobs <- item:
	default:
	}
}
func (a *app) startMetadataLookup() {
	ctx, cancel := context.WithCancel(context.Background())
	a.metadataCancel = cancel
	a.metadataJobs = make(chan queueItem, maxQueueItems)
	savedItems := a.queueState().Items
	for i := 0; i < 2; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case item := <-a.metadataJobs:
					if ctx.Err() != nil {
						return
					}
					metadata, err := lookupYouTubeMetadata(ctx, item.URL)
					if ctx.Err() != nil {
						return
					}
					a.applyQueuedMetadata(item.ID, metadata, err)
				}
			}
		}()
	}
	for _, item := range savedItems {
		a.scheduleMetadata(item)
	}
}
func (a *app) applyQueuedMetadata(id string, metadata youtubeVideo, lookupErr error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.queue.Items {
		item := &a.queue.Items[i]
		if item.ID != id || !item.MetadataPending {
			continue
		}
		item.MetadataPending = false
		if lookupErr == nil && item.State != "downloading" && item.State != "completed" {
			title := strings.TrimSpace(metadata.Title)
			if title != "" && len([]rune(title)) <= 500 {
				item.Title = title
			}
			item.Uploader = metadata.Uploader
			if strings.HasPrefix(metadata.Thumbnail, "https://") {
				item.Thumbnail = metadata.Thumbnail
			}
		}
		a.saveQueueLocked()
		return
	}
}
