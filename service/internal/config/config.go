// Package config разбирает аргументы командной строки и переменные окружения ghreport.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL  = "https://api.github.com"
	DefaultLimit    = 100
	DefaultOut      = "-"
	DefaultCacheDir = "./.ghreport-cache"
)

// Usage — справка, которую выводит --help.
const Usage = `Использование:
  ghreport --repos <owner/repo[,owner/repo...]>
           [--limit <N>] [--since <YYYY-MM-DD>]
           [--out <file|->] [--trace <file>] [--cache-dir <dir>]

Параметры:
  --repos      репозитории через запятую, обязателен
  --limit      максимум issue на репозиторий, не меньше 1 (по умолчанию 100)
  --since      только issue, созданные не раньше этой даты (00:00:00 UTC)
  --out        файл отчёта, "-" — stdout (по умолчанию "-")
  --trace      журнал HTTP-запросов в формате JSON Lines
  --cache-dir  каталог кэша ответов (по умолчанию ./.ghreport-cache)

Переменные окружения:
  GITHUB_TOKEN         токен доступа, необязателен
  GITHUB_API_BASE_URL  адрес API (по умолчанию https://api.github.com)
`

// ErrHelp возвращается, если запрошена справка.
var ErrHelp = flag.ErrHelp

// Repo — репозиторий owner/repo.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string {
	return r.Owner + "/" + r.Name
}

// Config — проверенные параметры запуска.
type Config struct {
	Repos    []Repo    // без дубликатов, в порядке первого упоминания
	Limit    int       // не меньше 1
	Since    time.Time // нулевое значение — фильтр не задан
	Out      string    // "-" — stdout
	Trace    string    // пусто — журнал не ведётся
	CacheDir string
	BaseURL  string // без завершающего слеша
	Token    string // пусто — запросы без авторизации
}

// Parse разбирает аргументы (без имени программы) и переменные окружения.
// Текст ошибки предназначен для пользователя.
func Parse(args []string, getenv func(string) string) (Config, error) {
	fs := flag.NewFlagSet("ghreport", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repos := fs.String("repos", "", "")
	limit := fs.String("limit", strconv.Itoa(DefaultLimit), "")
	since := fs.String("since", "", "")
	out := fs.String("out", DefaultOut, "")
	trace := fs.String("trace", "", "")
	cacheDir := fs.String("cache-dir", DefaultCacheDir, "")

	if err := fs.Parse(args); err != nil {
		return Config{}, translateFlagError(err)
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("лишние аргументы: %s", strings.Join(fs.Args(), " "))
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	cfg := Config{Out: *out, Trace: *trace, CacheDir: *cacheDir}
	var err error
	if cfg.Repos, err = parseRepos(*repos); err != nil {
		return Config{}, err
	}
	if cfg.Limit, err = parseLimit(*limit); err != nil {
		return Config{}, err
	}
	if set["since"] {
		if cfg.Since, err = parseSince(*since); err != nil {
			return Config{}, err
		}
	}
	if cfg.Out == "" {
		return Config{}, errors.New("--out не может быть пустым, для stdout укажите \"-\"")
	}
	if set["trace"] && cfg.Trace == "" {
		return Config{}, errors.New("--trace не может быть пустым")
	}
	if cfg.CacheDir == "" {
		return Config{}, errors.New("--cache-dir не может быть пустым")
	}
	if cfg.BaseURL, err = parseBaseURL(getenv("GITHUB_API_BASE_URL")); err != nil {
		return Config{}, err
	}
	cfg.Token = strings.TrimSpace(getenv("GITHUB_TOKEN"))
	return cfg, nil
}

func translateFlagError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return ErrHelp
	}
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "flag provided but not defined: -"); ok {
		return fmt.Errorf("неизвестный параметр --%s", name)
	}
	if name, ok := strings.CutPrefix(msg, "flag needs an argument: -"); ok {
		return fmt.Errorf("параметру --%s нужно значение", name)
	}
	if bad, ok := strings.CutPrefix(msg, "bad flag syntax: "); ok {
		return fmt.Errorf("неверный синтаксис параметра %s", bad)
	}
	return err
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func parseRepos(value string) ([]Repo, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errors.New("--repos обязателен: укажите репозитории owner/repo через запятую")
	}
	var repos []Repo
	seen := make(map[Repo]bool)
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		owner, name, ok := strings.Cut(item, "/")
		if !ok || !validRepoPart(owner) || !validRepoPart(name) {
			return nil, fmt.Errorf("--repos: неверный репозиторий %q, ожидается owner/repo", item)
		}
		repo := Repo{Owner: owner, Name: name}
		if !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}
	return repos, nil
}

func validRepoPart(s string) bool {
	return repoPart.MatchString(s) && s != "." && s != ".."
}

func parseLimit(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 {
		return 0, errors.New("--limit должен быть целым числом, не меньшим единицы")
	}
	return n, nil
}

func parseSince(value string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.UTC)
	if err != nil {
		return time.Time{}, errors.New("--since должен быть датой в формате YYYY-MM-DD")
	}
	return t, nil
}

func parseBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultBaseURL, nil
	}
	base := strings.TrimRight(raw, "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("GITHUB_API_BASE_URL: ожидается адрес вида https://host[/путь], получено %q", raw)
	}
	return base, nil
}
