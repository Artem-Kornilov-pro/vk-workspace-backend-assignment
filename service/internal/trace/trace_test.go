package trace

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestRecord(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf)
	ts := time.Date(2026, 10, 14, 12, 0, 1, 999, time.FixedZone("MSK", 3*3600))
	w.Record(Entry{Time: ts, Method: "GET", URL: "https://api.github.com/repos/cli/cli/issues?a=1&b=2",
		Status: 200, Attempt: 1})
	w.Record(Entry{Time: ts, Method: "GET", URL: "http://x/", Status: 0, Attempt: 2, Error: ErrTimeout})
	w.Record(Entry{Time: ts, Method: "GET", URL: "http://x/", Status: 304, Attempt: 1, FromCache: true})

	want := `{"ts":"2026-10-14T09:00:01Z","method":"GET","url":"https://api.github.com/repos/cli/cli/issues?a=1&b=2","status":200,"attempt":1,"from_cache":false,"error":null}
{"ts":"2026-10-14T09:00:01Z","method":"GET","url":"http://x/","status":0,"attempt":2,"from_cache":false,"error":"timeout"}
{"ts":"2026-10-14T09:00:01Z","method":"GET","url":"http://x/","status":304,"attempt":1,"from_cache":true,"error":null}
`
	if got := buf.String(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if w.Err() != nil {
		t.Fatal(w.Err())
	}
}

func TestNilWriter(t *testing.T) {
	var w *Writer
	w.Record(Entry{})
	if w.Err() != nil {
		t.Fatal(w.Err())
	}
}

type failingWriter struct{ calls int }

func (f *failingWriter) Write([]byte) (int, error) {
	f.calls++
	return 0, errors.New("disk full")
}

func TestWriteErrorStopsJournal(t *testing.T) {
	fw := &failingWriter{}
	w := New(fw)
	w.Record(Entry{})
	w.Record(Entry{})
	if fw.calls != 1 || w.Err() == nil {
		t.Fatalf("calls = %d, err = %v", fw.calls, w.Err())
	}
}
