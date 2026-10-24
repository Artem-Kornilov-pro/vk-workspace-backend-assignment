package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse([]string{"--repos", "cli/cli"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Repos:    []Repo{{"cli", "cli"}},
		Limit:    100,
		Out:      "-",
		CacheDir: "./.ghreport-cache",
		BaseURL:  "https://api.github.com",
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestParseAllFlags(t *testing.T) {
	args := []string{
		"--repos=cli/cli, golang/go,cli/cli,a.b/c-d_e",
		"--limit", "7",
		"--since", "2026-01-05",
		"--out", "report.json",
		"--trace", "trace.jsonl",
		"--cache-dir", "/tmp/cache",
	}
	cfg, err := Parse(args, env(map[string]string{
		"GITHUB_API_BASE_URL": "http://127.0.0.1:8080/api/v3//",
		"GITHUB_TOKEN":        " secret \n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Repos:    []Repo{{"cli", "cli"}, {"golang", "go"}, {"a.b", "c-d_e"}},
		Limit:    7,
		Since:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Out:      "report.json",
		Trace:    "trace.jsonl",
		CacheDir: "/tmp/cache",
		BaseURL:  "http://127.0.0.1:8080/api/v3",
		Token:    "secret",
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestParseBaseURLTrailingSlash(t *testing.T) {
	for _, raw := range []string{"https://ghe.example.com/", "https://ghe.example.com"} {
		cfg, err := Parse([]string{"--repos", "a/b"}, env(map[string]string{"GITHUB_API_BASE_URL": raw}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BaseURL != "https://ghe.example.com" {
			t.Errorf("%q: BaseURL = %q", raw, cfg.BaseURL)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"no repos", nil, nil, "--repos обязателен"},
		{"empty repos", []string{"--repos", " "}, nil, "--repos обязателен"},
		{"repo without owner", []string{"--repos", "cli"}, nil, `неверный репозиторий "cli"`},
		{"repo with extra slash", []string{"--repos", "a/b/c"}, nil, `неверный репозиторий "a/b/c"`},
		{"empty repo in list", []string{"--repos", "a/b,"}, nil, `неверный репозиторий ""`},
		{"dot repo", []string{"--repos", "a/.."}, nil, "неверный репозиторий"},
		{"bad chars", []string{"--repos", "a/b?c"}, nil, "неверный репозиторий"},
		{"zero limit", []string{"--repos", "a/b", "--limit", "0"}, nil,
			"--limit должен быть целым числом, не меньшим единицы"},
		{"negative limit", []string{"--repos", "a/b", "--limit=-3"}, nil, "--limit должен быть"},
		{"text limit", []string{"--repos", "a/b", "--limit", "ten"}, nil, "--limit должен быть"},
		{"float limit", []string{"--repos", "a/b", "--limit", "1.5"}, nil, "--limit должен быть"},
		{"bad since", []string{"--repos", "a/b", "--since", "2026-13-01"}, nil, "--since должен быть датой"},
		{"since with time", []string{"--repos", "a/b", "--since", "2026-01-01T00:00:00Z"}, nil, "--since"},
		{"empty since", []string{"--repos", "a/b", "--since="}, nil, "--since"},
		{"empty out", []string{"--repos", "a/b", "--out", ""}, nil, "--out не может быть пустым"},
		{"empty trace", []string{"--repos", "a/b", "--trace", ""}, nil, "--trace не может быть пустым"},
		{"empty cache dir", []string{"--repos", "a/b", "--cache-dir", ""}, nil, "--cache-dir не может быть пустым"},
		{"unknown flag", []string{"--repos", "a/b", "--verbose"}, nil, "неизвестный параметр --verbose"},
		{"missing value", []string{"--repos"}, nil, "параметру --repos нужно значение"},
		{"positional", []string{"--repos", "a/b", "extra"}, nil, "лишние аргументы: extra"},
		{"bad base url", []string{"--repos", "a/b"}, map[string]string{"GITHUB_API_BASE_URL": "ftp://x"},
			"GITHUB_API_BASE_URL"},
		{"base url without host", []string{"--repos", "a/b"}, map[string]string{"GITHUB_API_BASE_URL": "http://"},
			"GITHUB_API_BASE_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args, env(tt.env))
			if err == nil {
				t.Fatalf("ожидалась ошибка с %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ошибка %q не содержит %q", err, tt.want)
			}
		})
	}
}

func TestParseHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		if _, err := Parse([]string{arg}, env(nil)); !errors.Is(err, ErrHelp) {
			t.Errorf("%s: err = %v, want ErrHelp", arg, err)
		}
	}
}

func TestErrorsDoNotLeakToken(t *testing.T) {
	_, err := Parse([]string{"--limit", "0", "--repos", "a/b"}, env(map[string]string{"GITHUB_TOKEN": "s3cr3t"}))
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Fatalf("err = %v", err)
	}
}
