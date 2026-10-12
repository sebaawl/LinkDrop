package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestURL(t *testing.T) {
	for _, u := range []string{"https://www.youtube.com/watch?v=123", "http://example.com/vid", "https://vimeo.com/123"} {
		if err := validURL(u); err != nil {
			t.Fatalf("accepted %s: %v", u, err)
		}
	}
	for _, u := range []string{"", "file:///etc/passwd", "ftp://example.com", "javascript:alert(1)", "https://username:pass@example.com", "htt://test", "https:///"} {
		if err := validURL(u); err == nil {
			t.Errorf("accepted bad URL %q", u)
		}
	}
}
func TestArgSelection(t *testing.T) {
	args, err := makeArgs(downloadRequest{Mode: "video", VideoFormat: "mp4", Resolution: "1080"}, "")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{"height<=1080", "--merge-output-format mp4", "--recode-video mp4"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("missing %s from %s", expected, joined)
		}
	}
	args, err = makeArgs(downloadRequest{Mode: "audio", AudioFormat: "flac", AudioQuality: "best"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--audio-format flac --audio-quality 0") {
		t.Fatal(args)
	}
	for _, r := range []downloadRequest{{Mode: "video", VideoFormat: "mov", Resolution: "best"}, {Mode: "video", VideoFormat: "mp4", Resolution: "not_a_resolution"}, {Mode: "audio", AudioFormat: "mp3", AudioQuality: "bad"}, {Mode: "nothing"}} {
		if _, err := makeArgs(r, ""); err == nil {
			t.Errorf("accepted invalid request %+v", r)
		}
	}
}
func TestProgress(t *testing.T) {
	a := &app{}
	a.updateProgress("LDPROGRESS:  54.6%| 1.2MiB/s| 00:04")
	s := a.snapshot()
	if s.JobPercent != 54.6 || s.JobSpeed != "1.2MiB/s" || s.JobETA != "00:04" {
		t.Fatalf("bad progress %+v", s)
	}
	a.updateProgress("ARCHIVO_FINAL:C:\\Users\\Me\\Downloads\\test.mp3")
	if s = a.snapshot(); s.JobFile == "" || s.JobPercent != 100 {
		t.Fatalf("missing final file %+v", s)
	}
}
func TestUnzipEngines(t *testing.T) {
	d := t.TempDir()
	zp := filepath.Join(d, "ffmpeg.zip")
	f, e := os.Create(zp)
	if e != nil {
		t.Fatal(e)
	}
	zw := zip.NewWriter(f)
	for _, nm := range []string{"ffmpeg-9.0-essentials/bin/ffmpeg.exe", "ffmpeg-9.0-essentials/bin/ffprobe.exe", "other/random.txt"} {
		w, e := zw.Create(nm)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = w.Write([]byte("abc"))
	}
	if e := zw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	if e := unpackFFmpeg(zp, d); e != nil {
		t.Fatal(e)
	}
	for _, nm := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		if _, e := os.Stat(filepath.Join(d, nm)); e != nil {
			t.Fatal(e)
		}
	}
}

// Once yt-dlp/FFmpeg already exist on the PC, the setup button must be able to
// skip all network downloads instead of repeating the expensive FFmpeg step.
func TestReusePreviousEngines(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("X", 4096)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{directory: dir}
	if !a.checkEngines() {
		t.Fatal("existing engines should be reused")
	}
	if got := a.ffmpegDir(); got != dir {
		t.Fatalf("ffmpeg dir: %q, want %q", got, dir)
	}
	if got := a.ytDLPPath(); got != filepath.Join(dir, "yt-dlp.exe") {
		t.Fatalf("yt-dlp path: %q", got)
	}
}
