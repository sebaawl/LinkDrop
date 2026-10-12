package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func queueTestApp(t *testing.T, bin string) (*app, *http.ServeMux) {
	t.Helper()
	a := &app{token: "test-token", directory: bin, outputDefault: t.TempDir(), installed: true, theme: "system"}
	a.ensureQueueLocked()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/queue", a.apiQueue)
	mux.HandleFunc("/api/queue/add", a.apiQueueAdd)
	mux.HandleFunc("/api/queue/item", a.apiQueueItem)
	mux.HandleFunc("/api/queue/config", a.apiQueueConfig)
	mux.HandleFunc("/api/queue/control", a.apiQueueControl)
	mux.HandleFunc("/api/youtube/search", a.apiYouTubeSearch)
	return a, mux
}
func queueCall(t *testing.T, a *app, mux *http.ServeMux, path string, body any, want int) desktopResponse {
	t.Helper()
	data, _ := json.Marshal(body)
	response := desktopRequest(mux, a.token, "POST", path, string(data))
	if response.Status != want {
		t.Fatalf("%s returned %d: %s; want %d", path, response.Status, response.Body, want)
	}
	return response
}
func waitForQueue(t *testing.T, a *app, predicate func(queueSnapshot) bool) queueSnapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state := a.queueState()
		if predicate(state) {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queue did not reach the expected state")
	return queueSnapshot{}
}
func fixtureEngine(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	os.Mkdir(bin, 0700)
	source := `package main
import("encoding/json";"fmt";"os";"path/filepath";"strings";"time";"net/url")
func event(kind string,args []string){f,_:=os.OpenFile(filepath.Join(filepath.Dir(os.Args[0]),"events.jsonl"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);defer f.Close();json.NewEncoder(f).Encode(map[string]any{"kind":kind,"args":args})}
func main(){args:=os.Args[1:];last:=args[len(args)-1];if strings.HasPrefix(last,"ytsearch20:"){event("search",args);fmt.Print("{\"entries\":[{\"id\":\"abcdefghijk\",\"title\":\"Video de prueba\",\"channel\":\"Canal\",\"duration\":123,\"thumbnails\":[]}]}\n");return};event("start",args);fmt.Println("LINKDROP_TITLE:Video de prueba");fmt.Println("LDPROGRESS:50%|1MiB/s|00:01");if strings.Contains(last,"slow"){time.Sleep(2*time.Second)}else{time.Sleep(40*time.Millisecond)};event("end",args);if strings.Contains(last,"fail"){fmt.Fprintln(os.Stderr,"fixture failure");os.Exit(2)};folder:="";format:="mp4";for i,arg:=range args{if arg=="--paths"{folder=args[i+1]};if arg=="--audio-format"||arg=="--merge-output-format"{format=args[i+1]}};u,_:=url.Parse(last);name:=filepath.Base(u.Path);if u.Query().Get("v")!=""{name=u.Query().Get("v")};file:=filepath.Join(folder,name+"."+format);os.WriteFile(file,[]byte("fixture"),0600);fmt.Println("ARCHIVO_FINAL:"+file)}
`
	file := filepath.Join(root, "fixture.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBinary += ".exe"
	}
	cmd := exec.Command(goBinary, "build", "-o", filepath.Join(bin, "yt-dlp.exe"), file)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %v %s", err, output)
	}
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(strings.Repeat("x", 2048)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}
func events(t *testing.T, bin string) []struct {
	Kind string   `json:"kind"`
	Args []string `json:"args"`
} {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(bin, "events.jsonl"))
	result := []struct {
		Kind string   `json:"kind"`
		Args []string `json:"args"`
	}{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var event struct {
			Kind string   `json:"kind"`
			Args []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		result = append(result, event)
	}
	return result
}

func TestQueueValidationAndPersistence(t *testing.T) {
	a, mux := queueTestApp(t, filepath.Join(t.TempDir(), "bin"))
	queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://youtu.be/abcdefghijk"}, {URL: "https://www.youtube.com/watch?v=abcdefghijk&t=30"}, {URL: "https://example.com/video"}}}, 200)
	if len(a.queueState().Items) != 2 {
		t.Fatal("canonical duplicates were not omitted")
	}
	queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://example.com/good"}, {URL: "file:///bad"}}}, 400)
	if len(a.queueState().Items) != 2 {
		t.Fatal("invalid batch partly changed the queue")
	}
	settings := defaultDownloadSettings()
	settings.Mode = "audio"
	settings.AudioFormat = "flac"
	id := a.queueState().Items[1].ID
	queueCall(t, a, mux, "/api/queue/item", map[string]any{"id": id, "action": "configure", "useGlobal": false, "settings": settings}, 200)
	settings.Mode = "video"
	settings.VideoFormat = "webm"
	queueCall(t, a, mux, "/api/queue/config", map[string]any{"settings": settings, "applyToAll": false}, 200)
	if a.queueState().Items[1].UseGlobal {
		t.Fatal("global change discarded an individual override")
	}
	queueCall(t, a, mux, "/api/queue/config", map[string]any{"settings": settings, "applyToAll": true}, 200)
	if !a.queueState().Items[1].UseGlobal {
		t.Fatal("apply-to-all did not restore inheritance")
	}
	a.mu.Lock()
	a.queue.Items[0].State = "downloading"
	a.saveQueueLocked()
	a.mu.Unlock()
	restored := &app{directory: a.directory}
	restored.loadQueue()
	state := restored.queueState()
	if state.Running || state.Items[0].State != "pending" || state.Global.VideoFormat != "webm" {
		t.Fatal("restored queue did not retain settings and safely reset an interrupted download")
	}
	denied := desktopRequest(mux, "wrong", "POST", "/api/queue/control", `{"action":"start"}`)
	if denied.Status != 403 {
		t.Fatal("unauthorized queue action was accepted")
	}
}
func TestQueueStorageFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "file")
	os.WriteFile(blocked, []byte("existing"), 0600)
	a, mux := queueTestApp(t, filepath.Join(blocked, "bin"))
	queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://example.com/video"}}}, 500)
	if len(a.queueState().Items) != 0 || a.queueState().StorageError == "" {
		t.Fatal("failed persistence should not silently keep an added item")
	}
}
func TestQueueEngineWorkflow(t *testing.T) {
	bin := fixtureEngine(t)
	t.Run("YouTube search command", func(t *testing.T) {
		a, mux := queueTestApp(t, bin)
		response := queueCall(t, a, mux, "/api/youtube/search", map[string]string{"query": "--exec calc"}, 200)
		var result struct {
			Results []youtubeVideo `json:"results"`
		}
		if json.Unmarshal(response.Body, &result) != nil || len(result.Results) != 1 || result.Results[0].URL != "https://www.youtube.com/watch?v=abcdefghijk" {
			t.Fatal("search results not normalized")
		}
		args := events(t, bin)[0].Args
		for _, arg := range args {
			if arg == "--exec" {
				t.Fatal("query was interpreted as an engine option")
			}
		}
		if args[len(args)-1] != "ytsearch20:--exec calc" {
			t.Fatal("search expression missing")
		}
		queueCall(t, a, mux, "/api/youtube/search", map[string]string{"query": ""}, 400)
	})
	t.Run("ordered jobs with per-item settings and failure continuation", func(t *testing.T) {
		os.Remove(filepath.Join(bin, "events.jsonl"))
		a, mux := queueTestApp(t, bin)
		queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://example.com/fail"}, {URL: "https://example.com/second"}, {URL: "https://example.com/third"}}}, 200)
		global := defaultDownloadSettings()
		global.VideoFormat = "webm"
		queueCall(t, a, mux, "/api/queue/config", map[string]any{"settings": global}, 200)
		custom := defaultDownloadSettings()
		custom.Mode = "audio"
		custom.AudioFormat = "flac"
		queueCall(t, a, mux, "/api/queue/item", map[string]any{"id": a.queueState().Items[1].ID, "action": "configure", "settings": custom, "useGlobal": false}, 200)
		queueCall(t, a, mux, "/api/queue/control", map[string]string{"action": "start"}, 202)
		state := waitForQueue(t, a, func(state queueSnapshot) bool { return !state.Running && state.Items[2].State == "completed" })
		if state.Items[0].State != "failed" || state.Items[1].State != "completed" || !strings.HasSuffix(state.Items[1].File, ".flac") || !strings.HasSuffix(state.Items[2].File, ".webm") {
			t.Fatal("individual/global settings or failure continuation are incorrect")
		}
		logs := events(t, bin)
		if len(logs) != 6 {
			t.Fatalf("expected 3 start/end pairs, got %d", len(logs))
		}
		for i, event := range logs {
			want := "start"
			if i%2 == 1 {
				want = "end"
			}
			if event.Kind != want {
				t.Fatal("engine jobs overlapped")
			}
		}
		for i, name := range []string{"fail", "second", "third"} {
			if !strings.HasSuffix(logs[i*2].Args[len(logs[i*2].Args)-1], name) {
				t.Fatal("queue order changed")
			}
		}
	})
	t.Run("stop preserves pending jobs and resume", func(t *testing.T) {
		os.Remove(filepath.Join(bin, "events.jsonl"))
		a, mux := queueTestApp(t, bin)
		queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://example.com/slow"}, {URL: "https://example.com/next"}}}, 200)
		queueCall(t, a, mux, "/api/queue/control", map[string]string{"action": "start"}, 202)
		waitForQueue(t, a, func(state queueSnapshot) bool { return len(events(t, bin)) > 0 && state.Current != "" })
		queueCall(t, a, mux, "/api/queue/item", map[string]string{"id": a.queueState().Items[0].ID, "action": "remove"}, 409)
		queueCall(t, a, mux, "/api/queue/control", map[string]string{"action": "stop"}, 200)
		state := waitForQueue(t, a, func(state queueSnapshot) bool { return !state.Running })
		if state.Items[0].State != "canceled" || state.Items[1].State != "pending" {
			t.Fatal("stop did not retain pending jobs")
		}
		queueCall(t, a, mux, "/api/queue/control", map[string]string{"action": "start"}, 202)
		state = waitForQueue(t, a, func(state queueSnapshot) bool { return !state.Running && state.Items[1].State == "completed" })
		if state.Items[0].State != "canceled" {
			t.Fatal("resume silently restarted a canceled item")
		}
	})
	t.Run("global updates affect only waiting jobs", func(t *testing.T) {
		os.Remove(filepath.Join(bin, "events.jsonl"))
		a, mux := queueTestApp(t, bin)
		queueCall(t, a, mux, "/api/queue/add", map[string]any{"items": []youtubeVideo{{URL: "https://example.com/slow-global"}, {URL: "https://example.com/after-global"}}}, 200)
		queueCall(t, a, mux, "/api/queue/control", map[string]string{"action": "start"}, 202)
		waitForQueue(t, a, func(state queueSnapshot) bool { return len(events(t, bin)) > 0 && state.Current != "" })
		global := defaultDownloadSettings()
		global.VideoFormat = "webm"
		queueCall(t, a, mux, "/api/queue/config", map[string]any{"settings": global, "applyToAll": true}, 200)
		active := a.queueState()
		if active.Items[0].Settings.VideoFormat != "mp4" {
			t.Fatal("active job settings were rewritten")
		}
		state := waitForQueue(t, a, func(state queueSnapshot) bool { return !state.Running && state.Items[1].State == "completed" })
		if !strings.HasSuffix(state.Items[0].File, ".mp4") || !strings.HasSuffix(state.Items[1].File, ".webm") {
			t.Fatal("global changes applied to the wrong jobs")
		}
	})

}
