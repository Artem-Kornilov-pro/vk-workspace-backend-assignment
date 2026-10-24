package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ghreport/internal/cache"
)

func issueJSON(number int, created string) string {
	return fmt.Sprintf(`{"number": %d, "title": "issue %d", "user": {"login": "u%d"}, "labels": [{"name": "l%d"}],
		"created_at": %q, "html_url": "https://github.com/octo/repo/issues/%d"}`, number, number, number, number, created, number)
}

func prJSON(number int, created string) string {
	return fmt.Sprintf(`{"number": %d, "created_at": %q, "pull_request": {"url": "x"}}`, number, created)
}

func day(d int) string {
	return fmt.Sprintf("2026-01-%02dT10:00:00Z", d)
}

// fakeAPI отдаёт страницы issue репозитория octo/repo. Первая страница — по
// контрактному адресу, следующие — по непрозрачным адресам /cursor/<n> из Link.
type fakeAPI struct {
	*httptest.Server
	mu       sync.Mutex
	pages    []string // JSON-тела страниц
	etags    bool
	links    map[int]string // переопределение Link для страницы
	requests []*http.Request
}

func newFakeAPI(t *testing.T, pages ...[]string) *fakeAPI {
	api := &fakeAPI{etags: true, links: map[int]string{}}
	for _, items := range pages {
		api.pages = append(api.pages, "["+strings.Join(items, ",")+"]")
	}
	api.Server = httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(api.Close)
	return api
}

func (api *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.requests = append(api.requests, r)

	page := -1
	switch {
	case r.URL.Path == "/repos/octo/repo/issues":
		page = 0
	case strings.HasPrefix(r.URL.Path, "/cursor/"):
		page, _ = strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/cursor/"))
	}
	if page < 0 || page >= len(api.pages) {
		http.NotFound(w, r)
		return
	}
	if api.etags {
		etag := fmt.Sprintf(`"p%d-%d"`, page, len(api.pages[page]))
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			// Как и GitHub, 304 отдаётся без заголовка Link.
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	if link, ok := api.links[page]; ok {
		if link != "" {
			w.Header().Set("Link", strings.ReplaceAll(link, "{server}", api.URL))
		}
	} else if page+1 < len(api.pages) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/cursor/%d>; rel="next", <%s/cursor/%d>; rel="last"`,
			api.URL, page+1, api.URL, len(api.pages)-1))
	}
	w.Write([]byte(api.pages[page]))
}

func (api *fakeAPI) paths() []string {
	api.mu.Lock()
	defer api.mu.Unlock()
	var out []string
	for _, r := range api.requests {
		out = append(out, r.URL.Path)
	}
	return out
}

func numbers(t *testing.T, tc *testClient, q Query) []int64 {
	t.Helper()
	issues, err := tc.ListIssues(context.Background(), testRepo, q)
	if err != nil {
		t.Fatal(err)
	}
	var out []int64
	for _, is := range issues {
		out = append(out, is.Number)
	}
	return out
}

func TestListIssuesFollowsLinkPages(t *testing.T) {
	api := newFakeAPI(t,
		[]string{issueJSON(1, day(1)), prJSON(2, day(1)), issueJSON(3, day(2))},
		[]string{issueJSON(3, day(2)), issueJSON(4, day(3)), prJSON(5, day(3))},
		[]string{issueJSON(6, day(4))},
	)
	tc := newTestClient(api.URL)
	if got := numbers(t, tc, Query{Limit: 100}); !reflect.DeepEqual(got, []int64{1, 3, 4, 6}) {
		t.Fatalf("numbers = %v", got)
	}
	if got := api.paths(); !reflect.DeepEqual(got, []string{"/repos/octo/repo/issues", "/cursor/1", "/cursor/2"}) {
		t.Fatalf("paths = %v", got)
	}
	if q := api.requests[0].URL.RawQuery; q != "state=open&sort=created&direction=asc&per_page=100" {
		t.Errorf("query = %s", q)
	}
	for _, r := range api.requests {
		if r.URL.Query().Has("since") || r.Header.Get("Authorization") != "Bearer "+testToken {
			t.Errorf("request %s", r.URL)
		}
	}
}

func TestListIssuesStopsAtLimit(t *testing.T) {
	pages := [][]string{
		{issueJSON(1, day(1)), prJSON(2, day(1)), issueJSON(3, day(2))},
		{issueJSON(4, day(3)), issueJSON(5, day(4))},
		{issueJSON(6, day(5))},
	}
	tests := []struct {
		limit    int
		want     []int64
		requests int
	}{
		{1, []int64{1}, 1},
		{2, []int64{1, 3}, 1},
		{3, []int64{1, 3, 4}, 2},
		{4, []int64{1, 3, 4, 5}, 2},
		{5, []int64{1, 3, 4, 5, 6}, 3},
		{50, []int64{1, 3, 4, 5, 6}, 3},
	}
	for _, tt := range tests {
		api := newFakeAPI(t, pages...)
		tc := newTestClient(api.URL)
		if got := numbers(t, tc, Query{Limit: tt.limit}); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("limit %d: numbers = %v, want %v", tt.limit, got, tt.want)
		}
		if n := len(api.requests); n != tt.requests {
			t.Errorf("limit %d: requests = %d, want %d", tt.limit, n, tt.requests)
		}
	}
}

func TestListIssuesSince(t *testing.T) {
	api := newFakeAPI(t,
		[]string{issueJSON(1, day(1)), issueJSON(2, "2026-01-02T23:59:59Z"), issueJSON(3, "2026-01-03T00:00:00Z")},
		[]string{issueJSON(4, "2026-01-03T02:00:00+03:00"), issueJSON(5, day(4))},
	)
	tc := newTestClient(api.URL)
	since := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	if got := numbers(t, tc, Query{Limit: 2, Since: since}); !reflect.DeepEqual(got, []int64{3, 5}) {
		t.Fatalf("numbers = %v", got)
	}
}

func TestListIssuesSortsByCreatedThenNumber(t *testing.T) {
	api := newFakeAPI(t,
		[]string{issueJSON(9, day(3)), issueJSON(7, day(1)), issueJSON(8, day(1))},
		[]string{issueJSON(2, "2026-01-01T12:00:00+03:00"), issueJSON(1, day(5))},
	)
	tc := newTestClient(api.URL)
	if got := numbers(t, tc, Query{Limit: 10}); !reflect.DeepEqual(got, []int64{2, 7, 8, 9, 1}) {
		t.Fatalf("numbers = %v", got)
	}
}

func TestListIssuesKeepsCreatedAtAsIs(t *testing.T) {
	api := newFakeAPI(t, []string{issueJSON(1, "2026-01-01T12:00:00+03:00")})
	issues, err := newTestClient(api.URL).ListIssues(context.Background(), testRepo, Query{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if issues[0].CreatedAt != "2026-01-01T12:00:00+03:00" || issues[0].Author != "u1" ||
		!reflect.DeepEqual(issues[0].Labels, []string{"l1"}) {
		t.Fatalf("issue = %+v", issues[0])
	}
}

func TestListIssuesRelativeLinkAndLoop(t *testing.T) {
	api := newFakeAPI(t, []string{issueJSON(1, day(1))}, []string{issueJSON(2, day(2))})
	api.links[0] = `</cursor/1>; rel="next"`
	api.links[1] = `<{server}/cursor/1>; rel="next"`
	tc := newTestClient(api.URL)
	if got := numbers(t, tc, Query{Limit: 10}); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("numbers = %v", got)
	}
	if got := api.paths(); len(got) != 2 {
		t.Fatalf("paths = %v", got)
	}
}

func TestListIssuesBadPages(t *testing.T) {
	api := newFakeAPI(t, []string{issueJSON(1, day(1))}, []string{`{"broken"`})
	api.pages[1] = `{"broken"`
	if _, err := newTestClient(api.URL).ListIssues(context.Background(), testRepo, Query{Limit: 10}); CodeOf(err) != CodeInvalidResponse {
		t.Errorf("broken page: err = %v", err)
	}

	api = newFakeAPI(t, []string{issueJSON(1, day(1))})
	api.links[0] = `<ftp://example.com/x>; rel="next"`
	if _, err := newTestClient(api.URL).ListIssues(context.Background(), testRepo, Query{Limit: 10}); CodeOf(err) != CodeInvalidResponse {
		t.Errorf("ftp link: err = %v", err)
	}
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(data))
	}
	return out
}

func TestCacheSecondRunUsesNotModified(t *testing.T) {
	api := newFakeAPI(t,
		[]string{issueJSON(1, day(1)), prJSON(2, day(1))},
		[]string{issueJSON(3, day(2))},
	)
	dir := t.TempDir()
	store, err := cache.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	withCache := func(o *Options) { o.Cache = store }

	first := newTestClient(api.URL, withCache)
	want := numbers(t, first, Query{Limit: 10})
	if files := cacheFiles(t, dir); len(files) != 2 {
		t.Fatalf("cache files = %d", len(files))
	}
	for _, l := range first.traceLines(t) {
		if l.FromCache || l.Status != 200 {
			t.Errorf("first run: %+v", l)
		}
	}

	second := newTestClient(api.URL, withCache)
	if got := numbers(t, second, Query{Limit: 10}); !reflect.DeepEqual(got, want) {
		t.Fatalf("second run = %v, first = %v", got, want)
	}
	lines := second.traceLines(t)
	if len(lines) != 2 {
		t.Fatalf("trace = %s", second.trace.String())
	}
	for _, l := range lines {
		if !l.FromCache || l.Status != 304 {
			t.Errorf("second run: %+v", l)
		}
	}
	for _, r := range api.requests[2:] {
		if r.Header.Get("If-None-Match") == "" {
			t.Errorf("%s without If-None-Match", r.URL)
		}
	}
	for _, r := range api.requests[:2] {
		if _, ok := r.Header["If-None-Match"]; ok {
			t.Errorf("first run sent If-None-Match to %s", r.URL)
		}
	}
	for _, content := range cacheFiles(t, dir) {
		if strings.Contains(content, testToken) {
			t.Errorf("token in cache: %s", content)
		}
	}
}

func TestCacheUpdatedWhenDataChanges(t *testing.T) {
	api := newFakeAPI(t, []string{issueJSON(1, day(1))})
	store, _ := cache.Open(t.TempDir())
	withCache := func(o *Options) { o.Cache = store }
	numbers(t, newTestClient(api.URL, withCache), Query{Limit: 10})

	api.mu.Lock()
	api.pages[0] = "[" + issueJSON(1, day(1)) + "," + issueJSON(2, day(2)) + "]"
	api.mu.Unlock()
	tc := newTestClient(api.URL, withCache)
	if got := numbers(t, tc, Query{Limit: 10}); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("numbers = %v", got)
	}
	if l := tc.traceLines(t)[0]; l.Status != 200 || l.FromCache {
		t.Fatalf("trace = %+v", l)
	}
	entry, ok := store.Get(api.URL + "/repos/octo/repo/issues?state=open&sort=created&direction=asc&per_page=100")
	if !ok || !strings.Contains(entry.Body, `"number": 2`) {
		t.Fatalf("cache entry = %+v", entry)
	}
}

func TestCacheWithoutETag(t *testing.T) {
	api := newFakeAPI(t, []string{issueJSON(1, day(1))})
	api.etags = false
	dir := t.TempDir()
	store, _ := cache.Open(dir)
	numbers(t, newTestClient(api.URL, func(o *Options) { o.Cache = store }), Query{Limit: 10})
	if files := cacheFiles(t, dir); len(files) != 0 {
		t.Fatalf("cache files = %v", files)
	}
}

func TestNotModifiedWithoutCachedEntry(t *testing.T) {
	srv := newScriptedServer(t, step{status: http.StatusNotModified})
	tc := newTestClient(srv.URL)
	_, err := tc.ListIssues(context.Background(), testRepo, Query{Limit: 1})
	if CodeOf(err) != CodeInvalidResponse {
		t.Fatalf("err = %v", err)
	}
	if l := tc.traceLines(t)[0]; l.Status != 304 || l.FromCache {
		t.Fatalf("trace = %+v", l)
	}
}
