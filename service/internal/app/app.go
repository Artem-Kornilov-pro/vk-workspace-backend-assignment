// Package app связывает разбор аргументов, получение issue и вывод отчёта.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"ghreport/internal/config"
	"ghreport/internal/github"
	"ghreport/internal/report"
)

// Коды возврата.
const (
	ExitOK      = 0 // все репозитории получены
	ExitPartial = 1 // часть репозиториев с ошибкой
	ExitUsage   = 2 // некорректные аргументы
	ExitFatal   = 3 // 401 или ошибка по всем репозиториям
)

// IssueLister — источник issue репозитория.
type IssueLister interface {
	ListIssues(ctx context.Context, repo config.Repo, q github.Query) ([]report.Issue, error)
}

// Run выполняет команду ghreport и возвращает код возврата.
func Run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.Parse(args, getenv)
	if errors.Is(err, config.ErrHelp) {
		fmt.Fprint(stdout, config.Usage)
		return ExitOK
	}
	if err != nil {
		return usageError(stderr, err)
	}
	if err := checkOutPath(cfg.Out); err != nil {
		return usageError(stderr, err)
	}

	client := github.NewClient(github.Options{BaseURL: cfg.BaseURL, Token: cfg.Token})
	return run(ctx, cfg, client, stdout, stderr, time.Now)
}

func usageError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ghreport: %v\n", err)
	return ExitUsage
}

func run(ctx context.Context, cfg config.Config, lister IssueLister, stdout, stderr io.Writer,
	now func() time.Time) int {
	results := make([]report.RepoResult, 0, len(cfg.Repos))
	failed := 0
	for _, repo := range cfg.Repos {
		issues, err := lister.ListIssues(ctx, repo, github.Query{Limit: cfg.Limit, Since: cfg.Since})
		switch {
		case errors.Is(err, github.ErrUnauthorized):
			fmt.Fprintf(stderr, "ghreport: %s: %v\n", repo, err)
			report.EncodeFatal(stdout, report.FatalUnauthorized)
			return ExitFatal
		case err != nil:
			fmt.Fprintf(stderr, "ghreport: %s: %v\n", repo, err)
			results = append(results, report.Failed(repo.String(), github.CodeOf(err)))
			failed++
		default:
			results = append(results, report.Succeeded(repo.String(), issues))
		}
	}
	if failed == len(cfg.Repos) {
		report.EncodeFatal(stdout, report.FatalAllFailed)
		return ExitFatal
	}

	rep := report.Report{GeneratedAt: now().UTC().Format(time.RFC3339), Repos: results}
	if err := writeReport(cfg.Out, stdout, rep); err != nil {
		fmt.Fprintf(stderr, "ghreport: запись отчёта: %v\n", err)
		return ExitPartial
	}
	if failed > 0 {
		return ExitPartial
	}
	return ExitOK
}

// checkOutPath заранее проверяет, что отчёт можно будет записать по пути out.
func checkOutPath(out string) error {
	if out == config.DefaultOut {
		return nil
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return fmt.Errorf("--out: %s — каталог", out)
	}
	dir := filepath.Dir(out)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("--out: каталог %s не существует", dir)
	}
	return nil
}

func writeReport(out string, stdout io.Writer, rep report.Report) error {
	var buf bytes.Buffer
	if err := report.Encode(&buf, rep); err != nil {
		return err
	}
	if out == config.DefaultOut {
		_, err := stdout.Write(buf.Bytes())
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}
