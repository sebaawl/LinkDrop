package main

import "testing"

func TestYouTubeURLNormalization(t *testing.T) {
	for _, input := range []string{"https://youtu.be/abcdefghijk?t=1", "https://www.youtube.com/watch?v=abcdefghijk&list=ignored", "https://youtube.com/shorts/abcdefghijk", "https://youtube.com/live/abcdefghijk"} {
		url, err := canonicalVideoURL(input)
		if err != nil || url != "https://www.youtube.com/watch?v=abcdefghijk" {
			t.Fatalf("normalization failed for %s", input)
		}
	}
	for _, input := range []string{"https://youtube.com/playlist?list=123", "https://youtube.com/watch?v=invalid", "file:///bad"} {
		if _, err := canonicalVideoURL(input); err == nil {
			t.Fatalf("invalid video accepted: %s", input)
		}
	}
}
func TestYouTubeResultParsing(t *testing.T) {
	data := []byte(`{"entries":[{"id":"abcdefghijk","title":"Música & <prueba>","channel":"Canal","duration":120,"thumbnails":[{"url":"http://bad"},{"url":"https://example.com/thumb.jpg"}]},{"id":"abcdefghijk"},{"id":"invalid"}]}`)
	videos, err := parseYouTubeResults(data)
	if err != nil || len(videos) != 1 || videos[0].Title != "Música & <prueba>" || videos[0].Uploader != "Canal" || videos[0].Thumbnail != "https://example.com/thumb.jpg" {
		t.Fatal("search result parsing failed")
	}
	if _, err := parseYouTubeResults([]byte("not json")); err == nil {
		t.Fatal("invalid engine output accepted")
	}
}
