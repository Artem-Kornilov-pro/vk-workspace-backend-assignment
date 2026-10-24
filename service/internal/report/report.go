// Package report описывает формат отчёта ghreport и его сериализацию.
package report

import (
	"encoding/json"
	"io"
)

// Коды фатальных ошибок, при которых вместо отчёта выводится {"error": "<код>"}.
const (
	FatalUnauthorized = "unauthorized"
	FatalAllFailed    = "all_failed"
)

type Report struct {
	GeneratedAt string       `json:"generated_at"`
	Repos       []RepoResult `json:"repos"`
}

type RepoResult struct {
	Repo   string  `json:"repo"`
	Error  *string `json:"error"`
	Count  int     `json:"count"`
	Issues []Issue `json:"issues"`
}

type Issue struct {
	Number    int64    `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	CreatedAt string   `json:"created_at"`
	URL       string   `json:"url"`
}

// Succeeded — результат репозитория без ошибки. Пустые списки сериализуются
// как [], а не null.
func Succeeded(repo string, issues []Issue) RepoResult {
	if issues == nil {
		issues = []Issue{}
	}
	for i := range issues {
		if issues[i].Labels == nil {
			issues[i].Labels = []string{}
		}
	}
	return RepoResult{Repo: repo, Count: len(issues), Issues: issues}
}

// Failed — результат репозитория с кодом ошибки.
func Failed(repo, code string) RepoResult {
	return RepoResult{Repo: repo, Error: &code, Count: 0, Issues: []Issue{}}
}

// Encode пишет отчёт в виде JSON с отступами. HTML-символы не экранируются.
func Encode(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// EncodeFatal пишет {"error":"<code>"}.
func EncodeFatal(w io.Writer, code string) error {
	return json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}
