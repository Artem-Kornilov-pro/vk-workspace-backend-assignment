package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ghreport/internal/config"
	"ghreport/internal/report"
)

var testRepo = config.Repo{Owner: "octo", Name: "repo"}

func TestDecodePage(t *testing.T) {
	body := `[
	  {"number": 1, "title": "Bug", "user": {"login": "octocat"},
	   "labels": [{"name": "bug", "color": "red"}, "plain"],
	   "created_at": "2026-01-05T10:00:00Z", "html_url": "https://github.com/octo/repo/issues/1",
	   "extra": {"ignored": true}},
	  {"number": 2, "pull_request": {"url": "x"}},
	  {"number": 3, "pull_request": null, "created_at": "bad"},
	  {"number": 4, "title": null, "user": null, "labels": null, "created_at": "2026-01-06T00:00:00+03:00"}
	]`
	items, err := decodePage([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || !items[1].pullRequest || !items[2].pullRequest || items[0].pullRequest {
		t.Fatalf("items = %+v", items)
	}
	want := report.Issue{
		Number:    1,
		Title:     "Bug",
		Author:    "octocat",
		Labels:    []string{"bug", "plain"},
		CreatedAt: "2026-01-05T10:00:00Z",
		URL:       "https://github.com/octo/repo/issues/1",
	}
	if !reflect.DeepEqual(items[0].Issue, want) {
		t.Fatalf("got %+v, want %+v", items[0].Issue, want)
	}
	if items[3].Labels == nil || items[3].CreatedAt != "2026-01-06T00:00:00+03:00" {
		t.Fatalf("got %+v", items[3])
	}
}

func TestDecodePageErrors(t *testing.T) {
	for _, body := range []string{
		``,
		`not json`,
		`{"message": "object instead of array"}`,
		`[1]`,
		`[null]`,
		`[{"title": "no number", "created_at": "2026-01-05T10:00:00Z"}]`,
		`[{"number": 1}]`,
		`[{"number": 1.5, "created_at": "2026-01-05T10:00:00Z"}]`,
		`[{"number": 1, "created_at": "2026-01-05"}]`,
		`[{"number": 1, "created_at": "2026-01-05T10:00:00Z", "labels": [1]}]`,
		`[{"number": 1, "created_at": "2026-01-05T10:00:00Z", "user": "octocat"}]`,
	} {
		if _, err := decodePage([]byte(body)); err == nil {
			t.Errorf("decodePage(%q): ожидалась ошибка", body)
		}
	}
}

func TestListIssuesRequest(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`[{"number": 5, "created_at": "2026-01-05T10:00:00Z"}, {"number": 6, "pull_request": {}}]`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, Token: "s3cr3t"})
	issues, err := c.ListIssues(context.Background(), testRepo, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 5 {
		t.Fatalf("issues = %+v", issues)
	}
	if got.URL.RequestURI() != "/repos/octo/repo/issues?state=open&sort=created&direction=asc&per_page=100" {
		t.Errorf("request URI = %s", got.URL.RequestURI())
	}
	for header, want := range map[string]string{
		"Accept":               "application/vnd.github+json",
		"User-Agent":           UserAgent,
		"X-Github-Api-Version": "2026-03-10",
		"Authorization":        "Bearer s3cr3t",
	} {
		if v := got.Header.Get(header); v != want {
			t.Errorf("%s = %q, want %q", header, v, want)
		}
	}
}

func TestListIssuesWithoutToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Error("Authorization sent without token")
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := NewClient(Options{BaseURL: srv.URL}).ListIssues(context.Background(), testRepo, Query{Limit: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestListIssuesStatusErrors(t *testing.T) {
	tests := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusUnauthorized, `{}`, func(err error) bool { return errors.Is(err, ErrUnauthorized) }},
		{http.StatusNotFound, `{}`, func(err error) bool { return CodeOf(err) == CodeNotFound }},
		{http.StatusOK, `{"message": "x"}`, func(err error) bool { return CodeOf(err) == CodeInvalidResponse }},
		{http.StatusOK, `<html>`, func(err error) bool { return CodeOf(err) == CodeInvalidResponse }},
		{http.StatusGone, `{}`, func(err error) bool { return CodeOf(err) == CodeRequestFailed }},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			w.Write([]byte(tt.body))
		}))
		_, err := NewClient(Options{BaseURL: srv.URL, Token: "s3cr3t"}).
			ListIssues(context.Background(), testRepo, Query{Limit: 1})
		srv.Close()
		if !tt.check(err) {
			t.Errorf("status %d, body %q: err = %v", tt.status, tt.body, err)
		}
		if err != nil && strings.Contains(err.Error(), "s3cr3t") {
			t.Errorf("error leaks token: %v", err)
		}
	}
}
