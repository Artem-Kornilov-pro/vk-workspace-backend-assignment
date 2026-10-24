package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ghreport/internal/config"
	"ghreport/internal/github"
	"ghreport/internal/report"
)

// fakeLister возвращает заранее заданный результат для каждого репозитория.
type fakeLister struct {
	issues map[string][]report.Issue
	errs   map[string]error
	calls  []string
}

func (f *fakeLister) ListIssues(_ context.Context, repo config.Repo, _ github.Query) ([]report.Issue, error) {
	f.calls = append(f.calls, repo.String())
	if err := f.errs[repo.String()]; err != nil {
		return nil, err
	}
	return f.issues[repo.String()], nil
}

func fixedNow() time.Time {
	return time.Date(2026, 10, 14, 12, 0, 1, 500, time.FixedZone("MSK", 3*3600))
}

func repos(names ...string) []config.Repo {
	var out []config.Repo
	for _, name := range names {
		owner, repo, _ := strings.Cut(name, "/")
		out = append(out, config.Repo{Owner: owner, Name: repo})
	}
	return out
}

func runFake(t *testing.T, cfg config.Config, lister *fakeLister) (int, string, string) {
	t.Helper()
	if cfg.Out == "" {
		cfg.Out = "-"
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), cfg, lister, &stdout, &stderr, fixedNow)
	return code, stdout.String(), stderr.String()
}

func TestRunAllSucceeded(t *testing.T) {
	lister := &fakeLister{issues: map[string][]report.Issue{
		"a/one": {{Number: 1, Title: "t", Author: "u", CreatedAt: "2026-01-01T00:00:00Z", URL: "x"}},
	}}
	code, stdout, stderr := runFake(t, config.Config{Repos: repos("a/one", "a/two")}, lister)
	if code != ExitOK || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	var rep report.Report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.GeneratedAt != "2026-10-14T09:00:01Z" {
		t.Errorf("generated_at = %q", rep.GeneratedAt)
	}
	if len(rep.Repos) != 2 || rep.Repos[0].Repo != "a/one" || rep.Repos[1].Repo != "a/two" {
		t.Fatalf("repos = %+v", rep.Repos)
	}
	if rep.Repos[0].Count != 1 || rep.Repos[1].Count != 0 || rep.Repos[1].Issues == nil {
		t.Fatalf("repos = %+v", rep.Repos)
	}
}

func TestRunPartialFailure(t *testing.T) {
	lister := &fakeLister{errs: map[string]error{
		"a/missing": &github.Error{Code: github.CodeNotFound, Detail: "404"},
	}}
	code, stdout, stderr := runFake(t, config.Config{Repos: repos("a/ok", "a/missing")}, lister)
	if code != ExitPartial {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stderr, "a/missing") {
		t.Errorf("stderr = %q", stderr)
	}
	var rep report.Report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Repos[0].Error != nil || rep.Repos[1].Error == nil || *rep.Repos[1].Error != "not_found" {
		t.Fatalf("repos = %+v", rep.Repos)
	}
}

func TestRunAllFailed(t *testing.T) {
	lister := &fakeLister{errs: map[string]error{
		"a/missing": &github.Error{Code: github.CodeNotFound},
	}}
	code, stdout, _ := runFake(t, config.Config{Repos: repos("a/missing")}, lister)
	if code != ExitFatal || stdout != "{\"error\":\"all_failed\"}\n" {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

func TestRunUnauthorizedStopsImmediately(t *testing.T) {
	lister := &fakeLister{errs: map[string]error{"a/two": github.ErrUnauthorized}}
	code, stdout, _ := runFake(t, config.Config{Repos: repos("a/one", "a/two", "a/three")}, lister)
	if code != ExitFatal || stdout != "{\"error\":\"unauthorized\"}\n" {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
	if got := strings.Join(lister.calls, ","); got != "a/one,a/two" {
		t.Fatalf("calls = %s", got)
	}
}

func TestRunWritesReportToFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	code, stdout, _ := runFake(t, config.Config{Repos: repos("a/one"), Out: out}, &fakeLister{})
	if code != ExitOK || stdout != "" {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("report is not JSON: %s", data)
	}
}

func TestRunUsageErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bad limit", []string{"--repos", "a/b", "--limit", "0"},
			"ghreport: --limit должен быть целым числом, не меньшим единицы\n"},
		{"no repos", nil, "--repos обязателен"},
		{"out is dir", []string{"--repos", "a/b", "--out", dir}, "каталог"},
		{"out dir missing", []string{"--repos", "a/b", "--out", filepath.Join(dir, "no", "r.json")}, "не существует"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, func(string) string { return "" }, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("code = %d", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--help"}, func(string) string { return "" }, &stdout, &stderr)
	if code != ExitOK || !strings.Contains(stdout.String(), "--repos") {
		t.Fatalf("code = %d, stdout = %q", code, stdout.String())
	}
}

func TestRunEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo/repo/issues":
			w.Write([]byte(`[{"number": 1, "title": "a", "user": {"login": "u"}, "labels": [{"name": "bug"}],
				"created_at": "2026-01-05T10:00:00Z", "html_url": "https://github.com/octo/repo/issues/1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace.jsonl")
	getenv := func(key string) string {
		return map[string]string{"GITHUB_API_BASE_URL": srv.URL + "/", "GITHUB_TOKEN": "s3cr3t"}[key]
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--repos", "octo/repo,octo/missing", "--trace", tracePath, "--cache-dir", filepath.Join(dir, "cache")}
	code := Run(context.Background(), args, getenv, &stdout, &stderr)
	if code != ExitPartial {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	var rep report.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Repos[0].Count != 1 || *rep.Repos[1].Error != "not_found" {
		t.Fatalf("report = %s", stdout.String())
	}

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("trace = %s", data)
	}
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if len(entry) != 7 {
			t.Errorf("trace fields = %v", entry)
		}
	}
	if strings.Contains(string(data)+stderr.String(), "s3cr3t") {
		t.Error("token leaked")
	}
}

func TestRunTraceFileError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--repos", "a/b", "--trace", filepath.Join(t.TempDir(), "missing", "trace.jsonl")}
	code := Run(context.Background(), args, func(string) string { return "" }, &stdout, &stderr)
	if code != ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--trace") {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}
