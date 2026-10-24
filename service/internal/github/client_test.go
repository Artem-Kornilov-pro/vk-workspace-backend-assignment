package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ghreport/internal/trace"
)

var testNow = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

const testToken = "s3cr3t-token"

// testClient — клиент с мгновенными паузами, фиксированным временем и журналом в памяти.
type testClient struct {
	*Client
	sleeps []time.Duration
	trace  bytes.Buffer
	logs   []string
}

func newTestClient(baseURL string, configure ...func(*Options)) *testClient {
	tc := &testClient{}
	opts := Options{
		BaseURL: baseURL,
		Token:   testToken,
		Trace:   trace.New(&tc.trace),
		Sleep: func(_ context.Context, d time.Duration) error {
			tc.sleeps = append(tc.sleeps, d)
			return nil
		},
		Now:  func() time.Time { return testNow },
		Logf: func(format string, args ...any) { tc.logs = append(tc.logs, fmt.Sprintf(format, args...)) },
	}
	for _, f := range configure {
		f(&opts)
	}
	tc.Client = NewClient(opts)
	return tc
}

type traceLine struct {
	TS        string  `json:"ts"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	Status    int     `json:"status"`
	Attempt   int     `json:"attempt"`
	FromCache bool    `json:"from_cache"`
	Error     *string `json:"error"`
}

func (tc *testClient) traceLines(t *testing.T) []traceLine {
	t.Helper()
	var lines []traceLine
	for _, raw := range strings.Split(strings.TrimSpace(tc.trace.String()), "\n") {
		if raw == "" {
			continue
		}
		var l traceLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatalf("trace line %q: %v", raw, err)
		}
		lines = append(lines, l)
	}
	return lines
}

func (tc *testClient) statuses(t *testing.T) []int {
	var out []int
	for _, l := range tc.traceLines(t) {
		out = append(out, l.Status)
	}
	return out
}

// step — очередной ответ сервера.
type step struct {
	status int
	header map[string]string
	body   string
}

// scriptedServer отвечает шагами по очереди и запоминает запросы.
type scriptedServer struct {
	*httptest.Server
	mu       sync.Mutex
	steps    []step
	requests []*http.Request
}

func newScriptedServer(t *testing.T, steps ...step) *scriptedServer {
	s := &scriptedServer{steps: steps}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r)
		if len(s.steps) == 0 {
			s.mu.Unlock()
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		st := s.steps[0]
		s.steps = s.steps[1:]
		s.mu.Unlock()
		for k, v := range st.header {
			w.Header().Set(k, strings.ReplaceAll(v, "{server}", s.URL))
		}
		w.WriteHeader(st.status)
		w.Write([]byte(st.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *scriptedServer) issuesURL() string {
	return s.URL + "/repos/octo/repo/issues?state=open&sort=created&direction=asc&per_page=100"
}

func ok(body string) step {
	return step{status: http.StatusOK, body: body}
}

func TestRetriesServerErrors(t *testing.T) {
	srv := newScriptedServer(t, step{status: 500}, step{status: 503}, ok(`[]`))
	tc := newTestClient(srv.URL)
	if _, _, err := tc.get(context.Background(), srv.issuesURL()); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(tc.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", tc.sleeps, want)
	}
	lines := tc.traceLines(t)
	if len(lines) != 3 {
		t.Fatalf("trace = %s", tc.trace.String())
	}
	for i, l := range lines {
		if l.Attempt != i+1 || l.URL != srv.issuesURL() || l.Method != "GET" || l.Error != nil ||
			l.TS != "2026-10-14T09:00:00Z" || l.FromCache {
			t.Errorf("line %d = %+v", i, l)
		}
	}
	if got := tc.statuses(t); !reflect.DeepEqual(got, []int{500, 503, 200}) {
		t.Errorf("statuses = %v", got)
	}
}

func TestServerErrorsExhausted(t *testing.T) {
	srv := newScriptedServer(t, step{status: 502}, step{status: 502}, step{status: 502}, step{status: 502})
	tc := newTestClient(srv.URL)
	_, _, err := tc.get(context.Background(), srv.issuesURL())
	if CodeOf(err) != CodeRequestFailed {
		t.Fatalf("err = %v", err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !reflect.DeepEqual(tc.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", tc.sleeps, want)
	}
	if n := len(tc.traceLines(t)); n != 4 {
		t.Errorf("trace lines = %d", n)
	}
}

func TestNetworkErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()

	tc := newTestClient(base)
	_, _, err := tc.get(context.Background(), base+"/repos/octo/repo/issues")
	if CodeOf(err) != CodeRequestFailed {
		t.Fatalf("err = %v", err)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !reflect.DeepEqual(tc.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", tc.sleeps, want)
	}
	lines := tc.traceLines(t)
	if len(lines) != 4 {
		t.Fatalf("trace = %s", tc.trace.String())
	}
	for i, l := range lines {
		if l.Status != 0 || l.Attempt != i+1 || l.Error == nil || *l.Error != "network" {
			t.Errorf("line %d = %+v", i, l)
		}
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	tc := newTestClient(srv.URL, func(o *Options) {
		o.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	})
	_, _, err := tc.get(context.Background(), srv.URL+"/slow")
	if CodeOf(err) != CodeRequestFailed {
		t.Fatalf("err = %v", err)
	}
	lines := tc.traceLines(t)
	if len(lines) != 4 {
		t.Fatalf("trace = %s", tc.trace.String())
	}
	for _, l := range lines {
		if l.Status != 0 || l.Error == nil || *l.Error != "timeout" {
			t.Errorf("line = %+v", l)
		}
	}
}

func TestRateLimitWaits(t *testing.T) {
	tests := []struct {
		name string
		step step
		want time.Duration
	}{
		{"429 retry-after seconds",
			step{status: 429, header: map[string]string{"Retry-After": "7"}}, 7 * time.Second},
		{"403 retry-after http date",
			step{status: 403, header: map[string]string{"Retry-After": testNow.Add(5 * time.Second).Format(http.TimeFormat)}},
			5 * time.Second},
		{"403 remaining 0 and reset",
			step{status: 403, header: map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     strconv.FormatInt(testNow.Add(30*time.Second).Unix(), 10),
			}}, 31 * time.Second},
		{"429 reset in the past",
			step{status: 429, header: map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     strconv.FormatInt(testNow.Add(-time.Hour).Unix(), 10),
			}}, time.Second},
		{"403 rate limit in body",
			step{status: 403, body: `{"message": "API rate limit exceeded for 1.2.3.4."}`}, 60 * time.Second},
		{"429 secondary rate limit in body",
			step{status: 429, body: `{"message": "You have exceeded a secondary rate limit."}`}, 60 * time.Second},
		{"429 unparsable retry-after falls back to 60s",
			step{status: 429, header: map[string]string{"Retry-After": "soon"}}, 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedServer(t, tt.step, ok(`[]`))
			tc := newTestClient(srv.URL)
			if _, _, err := tc.get(context.Background(), srv.issuesURL()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tc.sleeps, []time.Duration{tt.want}) {
				t.Errorf("sleeps = %v, want [%v]", tc.sleeps, tt.want)
			}
			lines := tc.traceLines(t)
			if len(lines) != 2 || lines[0].Status != tt.step.status || lines[1].Status != 200 ||
				lines[0].Attempt != 1 || lines[1].Attempt != 2 {
				t.Errorf("trace = %s", tc.trace.String())
			}
		})
	}
}

func TestRateLimitExhausted(t *testing.T) {
	limited := step{status: 429, header: map[string]string{"Retry-After": "1"}}
	srv := newScriptedServer(t, limited, limited, limited, limited)
	tc := newTestClient(srv.URL)
	_, _, err := tc.get(context.Background(), srv.issuesURL())
	if CodeOf(err) != CodeRateLimited {
		t.Fatalf("err = %v", err)
	}
	if len(tc.sleeps) != 3 || len(tc.traceLines(t)) != 4 {
		t.Errorf("sleeps = %v, trace = %s", tc.sleeps, tc.trace.String())
	}
}

func TestRateLimitWaitTooLong(t *testing.T) {
	srv := newScriptedServer(t, step{status: 429, header: map[string]string{"Retry-After": "3600"}})
	tc := newTestClient(srv.URL)
	_, _, err := tc.get(context.Background(), srv.issuesURL())
	if CodeOf(err) != CodeRateLimited || len(tc.sleeps) != 0 {
		t.Fatalf("err = %v, sleeps = %v", err, tc.sleeps)
	}
}

func TestForbiddenWithoutRateLimitSigns(t *testing.T) {
	for _, status := range []int{403, 429} {
		srv := newScriptedServer(t, step{status: status, body: `{"message": "Resource not accessible"}`})
		tc := newTestClient(srv.URL)
		_, _, err := tc.get(context.Background(), srv.issuesURL())
		if CodeOf(err) != CodeRequestFailed || len(tc.sleeps) != 0 || len(srv.requests) != 1 {
			t.Errorf("%d: err = %v, sleeps = %v, requests = %d", status, err, tc.sleeps, len(srv.requests))
		}
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		status int
		check  func(error) bool
	}{
		{401, func(err error) bool { return errors.Is(err, ErrUnauthorized) }},
		{404, func(err error) bool { return CodeOf(err) == CodeNotFound }},
		{410, func(err error) bool { return CodeOf(err) == CodeRequestFailed }},
		{303, func(err error) bool { return CodeOf(err) == CodeRequestFailed }},
	}
	for _, tt := range tests {
		srv := newScriptedServer(t, step{status: tt.status, header: map[string]string{"Location": "/x"}})
		tc := newTestClient(srv.URL)
		_, _, err := tc.get(context.Background(), srv.issuesURL())
		if !tt.check(err) {
			t.Errorf("%d: err = %v", tt.status, err)
		}
		if len(srv.requests) != 1 || len(tc.sleeps) != 0 {
			t.Errorf("%d: requests = %d, sleeps = %v", tt.status, len(srv.requests), tc.sleeps)
		}
	}
}

func TestRedirectsSameHost(t *testing.T) {
	for _, status := range []int{301, 302, 307, 308} {
		srv := newScriptedServer(t,
			step{status: status, header: map[string]string{"Location": "/repos/octo/renamed/issues?page=1"}},
			ok(`[]`))
		tc := newTestClient(srv.URL)
		_, final, err := tc.get(context.Background(), srv.issuesURL())
		if err != nil {
			t.Fatalf("%d: %v", status, err)
		}
		if final != srv.URL+"/repos/octo/renamed/issues?page=1" {
			t.Errorf("%d: final = %s", status, final)
		}
		if len(srv.requests) != 2 || srv.requests[1].URL.Path != "/repos/octo/renamed/issues" {
			t.Fatalf("%d: requests = %d", status, len(srv.requests))
		}
		for _, r := range srv.requests {
			if r.Header.Get("Authorization") != "Bearer "+testToken {
				t.Errorf("%d: %s without Authorization", status, r.URL)
			}
		}
		if got := tc.statuses(t); !reflect.DeepEqual(got, []int{status, 200}) {
			t.Errorf("%d: statuses = %v", status, got)
		}
	}
}

func TestRedirectToOtherHostDropsAuthorization(t *testing.T) {
	other := newScriptedServer(t, ok(`[]`))
	srv := newScriptedServer(t, step{status: 307, header: map[string]string{"Location": other.issuesURL()}})
	tc := newTestClient(srv.URL)
	if _, _, err := tc.get(context.Background(), srv.issuesURL()); err != nil {
		t.Fatal(err)
	}
	if srv.requests[0].Header.Get("Authorization") == "" {
		t.Error("base host request without Authorization")
	}
	if _, ok := other.requests[0].Header["Authorization"]; ok {
		t.Error("Authorization sent to another host")
	}
}

func TestPermanentRedirectRemembered(t *testing.T) {
	srv := newScriptedServer(t,
		step{status: 301, header: map[string]string{"Location": "/moved"}},
		ok(`[]`),
		ok(`[]`))
	tc := newTestClient(srv.URL)
	for i := 0; i < 2; i++ {
		if _, _, err := tc.get(context.Background(), srv.issuesURL()); err != nil {
			t.Fatal(err)
		}
	}
	if len(srv.requests) != 3 || srv.requests[2].URL.Path != "/moved" {
		t.Fatalf("requests = %d", len(srv.requests))
	}
}

func TestRedirectErrors(t *testing.T) {
	loop := step{status: 302, header: map[string]string{"Location": "/loop"}}
	srv := newScriptedServer(t, loop, loop, loop, loop, loop, loop)
	tc := newTestClient(srv.URL)
	if _, _, err := tc.get(context.Background(), srv.issuesURL()); CodeOf(err) != CodeRequestFailed {
		t.Errorf("loop: err = %v", err)
	}

	srv = newScriptedServer(t, step{status: 302})
	tc = newTestClient(srv.URL)
	if _, _, err := tc.get(context.Background(), srv.issuesURL()); CodeOf(err) != CodeInvalidResponse {
		t.Errorf("no location: err = %v", err)
	}
}

func TestContextCancelStopsRetries(t *testing.T) {
	srv := newScriptedServer(t, step{status: 500})
	ctx, cancel := context.WithCancel(context.Background())
	tc := newTestClient(srv.URL, func(o *Options) {
		o.Sleep = func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		}
	})
	_, _, err := tc.get(ctx, srv.issuesURL())
	if CodeOf(err) != CodeRequestFailed || len(srv.requests) != 1 {
		t.Fatalf("err = %v, requests = %d", err, len(srv.requests))
	}
}

func TestTokenNeverLeaks(t *testing.T) {
	srv := newScriptedServer(t,
		step{status: 429, header: map[string]string{"Retry-After": "1"}},
		step{status: 302, header: map[string]string{"Location": "/next"}},
		step{status: 500},
		step{status: 404})
	tc := newTestClient(srv.URL)
	_, _, err := tc.get(context.Background(), srv.issuesURL())
	if CodeOf(err) != CodeNotFound {
		t.Fatalf("err = %v", err)
	}
	everything := tc.trace.String() + err.Error() + strings.Join(tc.logs, "\n")
	for _, secret := range []string{testToken, "Authorization", "Bearer"} {
		if strings.Contains(everything, secret) {
			t.Errorf("%q leaked: %s", secret, everything)
		}
	}
}
