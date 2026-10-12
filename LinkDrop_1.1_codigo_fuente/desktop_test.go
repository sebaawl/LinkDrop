package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDesktopBridgeStatus(t *testing.T) {
	a := &app{token: "desktop-token", installed: true, outputDefault: t.TempDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", a.apiStatus)
	response := desktopRequest(mux, a.token, "GET", "/api/status", "")
	if response.Status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Status, response.Body)
	}
	var result status
	if err := json.Unmarshal(response.Body, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.Version != appVersion || result.DownloadFolder != a.outputDefault {
		t.Fatalf("unexpected status: %+v", result)
	}
	// The adapter must preserve the handlers' authorization checks.
	response = desktopRequest(mux, "wrong-token", "GET", "/api/status", "")
	if response.Status != http.StatusForbidden || !json.Valid(response.Body) {
		t.Fatalf("unexpected denial: %+v", response)
	}
}

func TestDesktopBridgeValidation(t *testing.T) {
	a := &app{token: "desktop-token", installed: true}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", a.apiInfo)
	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"unknown operation", "POST", "/api/unknown", "{}", 400},
		{"external URL", "GET", "https://example.com/api/status", "", 400},
		{"wrong method", "GET", "/api/info", "", 400},
		{"query injection", "GET", "/api/status?x=1", "", 400},
		{"oversized body", "POST", "/api/info", strings.Repeat("x", 64*1024+1), 413},
		{"invalid JSON", "POST", "/api/info", "{", 400},
		{"invalid media URL", "POST", "/api/info", `{"url":"file:///etc/passwd"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := desktopRequest(mux, a.token, tc.method, tc.path, tc.body)
			if response.Status != tc.want || !json.Valid(response.Body) {
				t.Fatalf("response = %+v, want status %d and JSON body", response, tc.want)
			}
		})
	}
}

func TestWindowCloseCancelsDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a := &app{cancel: cancel, downloadDone: done}
	go func() {
		<-ctx.Done()
		close(done)
	}()
	a.stopDownload()
	if ctx.Err() != context.Canceled {
		t.Fatal("active download was not canceled")
	}
	(&app{}).stopDownload()
}
