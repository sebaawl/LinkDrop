package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type metadataTestClient struct {
	do func(*http.Request) (*http.Response, error)
}

func (c metadataTestClient) Do(r *http.Request) (*http.Response, error) { return c.do(r) }
func TestPublicMetadataRequest(t *testing.T) {
	client := metadataTestClient{do: func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "www.youtube.com" || r.URL.Path != "/oembed" || r.URL.Query().Get("url") != "https://www.youtube.com/watch?v=abcdefghijk" {
			t.Fatal("metadata request did not target the public YouTube endpoint with the canonical video")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"title":"Video público","author_name":"Canal","thumbnail_url":"https://i.ytimg.com/vi/abcdefghijk/hqdefault.jpg"}`))}, nil
	}}
	result, err := lookupYouTubeMetadataWithClient(context.Background(), "https://youtu.be/abcdefghijk?t=5", client)
	if err != nil || result.Title != "Video público" || result.Uploader != "Canal" {
		t.Fatal("public metadata not parsed")
	}
	if _, err := lookupYouTubeMetadataWithClient(context.Background(), "https://example.com/video", client); err == nil {
		t.Fatal("non-YouTube URL sent to metadata endpoint")
	}
}
func TestQueueMetadataSuccessAndFailure(t *testing.T) {
	a, mux := queueTestApp(t, t.TempDir())
	queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://youtu.be/abcdefghijk"}, {URL: "https://youtu.be/lmnopqrstuv"}}}, 200)
	original := a.queueState().Items
	a.applyQueuedMetadata(original[0].ID, youtubeVideo{Title: "Título identificado", Uploader: "Canal", Thumbnail: "https://example.com/image.jpg"}, nil)
	a.applyQueuedMetadata(original[1].ID, youtubeVideo{}, errors.New("network unavailable"))
	state := a.queueState()
	if state.Items[0].Title != "Título identificado" || state.Items[0].URL != original[0].URL || state.Items[0].State != "pending" {
		t.Fatal("metadata lookup altered the queued source or download state")
	}
	if len(state.Items) != 2 || state.Items[1].URL != original[1].URL || state.Items[1].State != "pending" {
		t.Fatal("metadata failure lost the queued video")
	}
}
