package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const testURL = "https://api.github.com/repos/cli/cli/issues?state=open&sort=created&direction=asc&per_page=100"

func TestPutGetDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(testURL); ok {
		t.Fatal("empty cache returned entry")
	}

	want := Entry{URL: testURL, ETag: `W/"abc"`, Link: `<https://x/2>; rel="next"`, Body: `[{"title": "<b> & \"q\""}]`}
	if err := s.Put(want); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(testURL)
	if !ok || got != want {
		t.Fatalf("got %+v, %v", got, ok)
	}

	// Перезапись тем же адресом.
	want.ETag = `"def"`
	if err := s.Put(want); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(testURL); got.ETag != `"def"` {
		t.Fatalf("got %+v", got)
	}

	files, _ := os.ReadDir(dir)
	if len(files) != 1 || filepath.Ext(files[0].Name()) != ".json" || len(files[0].Name()) != 64+5 {
		t.Fatalf("files = %v", files)
	}
	var raw map[string]any
	data, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) != 4 {
		t.Fatalf("file = %s", data)
	}

	if err := s.Delete(testURL); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(testURL); ok {
		t.Fatal("entry survived Delete")
	}
	if err := s.Delete(testURL); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestGetIgnoresBrokenEntries(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		`not json`,
		`{"url": "https://other", "etag": "x", "body": "[]"}`,
		`{"url": "` + testURL + `", "etag": "", "body": "[]"}`,
	} {
		if err := os.WriteFile(s.path(testURL), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Get(testURL); ok {
			t.Errorf("Get returned entry for %s", content)
		}
	}
}

func TestNilStore(t *testing.T) {
	var s *Store
	if _, ok := s.Get(testURL); ok {
		t.Fatal("nil store returned entry")
	}
	if s.Put(Entry{URL: testURL}) != nil || s.Delete(testURL) != nil {
		t.Fatal("nil store returned error")
	}
}

func TestOpenFailsOnFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(file); err == nil {
		t.Fatal("Open succeeded on a regular file")
	}
}
