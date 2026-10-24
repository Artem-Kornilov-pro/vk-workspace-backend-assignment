// Package github получает открытые issue репозиториев через REST API GitHub
// или совместимого с ним сервера.
package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"ghreport/internal/trace"
)

const (
	UserAgent      = "ghreport/1.0 (vk-workspace-backend-assignment)"
	APIVersion     = "2026-03-10"
	RequestTimeout = 30 * time.Second

	// DefaultRateLimitWait — пауза, если ни retry-after, ни x-ratelimit-reset не заданы.
	DefaultRateLimitWait = 60 * time.Second
	// DefaultMaxRateLimitWaits — сколько раз подряд ждать восстановления лимита
	// для одного адреса, прежде чем вернуть rate_limited.
	DefaultMaxRateLimitWaits = 3
	// DefaultMaxRateLimitWait — самая длинная допустимая пауза; если сервер
	// просит ждать дольше, репозиторий сразу получает rate_limited.
	DefaultMaxRateLimitWait = 15 * time.Minute

	maxRedirects = 5
	maxBodySize  = 64 << 20
)

// DefaultRetryDelays — паузы перед повторами при сетевых ошибках и ответах 5xx.
var DefaultRetryDelays = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// Options — параметры клиента. Нулевые значения заменяются значениями по умолчанию.
type Options struct {
	BaseURL string // без завершающего слеша
	Token   string

	HTTPClient        *http.Client  // таймаут запроса; CheckRedirect будет заменён
	Trace             *trace.Writer // nil — журнал не ведётся
	Logf              func(format string, args ...any)
	Sleep             func(ctx context.Context, d time.Duration) error
	Now               func() time.Time
	RetryDelays       []time.Duration
	MaxRateLimitWaits int
	MaxRateLimitWait  time.Duration
}

type Client struct {
	baseURL string
	base    *url.URL
	token   string

	http              *http.Client
	trace             *trace.Writer
	logf              func(format string, args ...any)
	sleep             func(ctx context.Context, d time.Duration) error
	now               func() time.Time
	retryDelays       []time.Duration
	maxRateLimitWaits int
	maxRateLimitWait  time.Duration

	moved map[string]string // постоянные редиректы 301/308, полученные за этот запуск
}

func NewClient(opts Options) *Client {
	c := &Client{
		baseURL:           opts.BaseURL,
		token:             opts.Token,
		trace:             opts.Trace,
		logf:              opts.Logf,
		sleep:             opts.Sleep,
		now:               opts.Now,
		retryDelays:       opts.RetryDelays,
		maxRateLimitWaits: opts.MaxRateLimitWaits,
		maxRateLimitWait:  opts.MaxRateLimitWait,
		moved:             make(map[string]string),
	}
	c.base, _ = url.Parse(opts.BaseURL)
	if c.base == nil {
		c.base = &url.URL{}
	}

	httpClient := http.Client{Timeout: RequestTimeout}
	if opts.HTTPClient != nil {
		httpClient = *opts.HTTPClient
	}
	// Редиректы обрабатываются вручную: каждая попытка попадает в журнал,
	// а Authorization не уходит на другой хост.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	c.http = &httpClient

	if c.logf == nil {
		c.logf = func(string, ...any) {}
	}
	if c.sleep == nil {
		c.sleep = sleepContext
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.retryDelays == nil {
		c.retryDelays = DefaultRetryDelays
	}
	if c.maxRateLimitWaits == 0 {
		c.maxRateLimitWaits = DefaultMaxRateLimitWaits
	}
	if c.maxRateLimitWait == 0 {
		c.maxRateLimitWait = DefaultMaxRateLimitWait
	}
	return c
}

// reply — ответ сервера с прочитанным телом.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// get выполняет GET с переходами по редиректам и возвращает успешный ответ 2xx
// вместе с адресом, который его вернул.
func (c *Client) get(ctx context.Context, rawURL string) (*reply, string, error) {
	target := c.resolveMoved(rawURL)
	for hops := 0; ; hops++ {
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, "", errorf(CodeInvalidResponse, "некорректный адрес %q", target)
		}
		r, err := c.do(ctx, u)
		if err != nil {
			return nil, "", err
		}

		switch {
		case r.status >= 200 && r.status < 300:
			if len(r.body) > maxBodySize {
				return nil, "", errorf(CodeInvalidResponse, "%s: ответ больше %d байт", target, maxBodySize)
			}
			return r, target, nil
		case isRedirect(r.status):
			if hops == maxRedirects {
				return nil, "", errorf(CodeRequestFailed, "%s: больше %d редиректов подряд", target, maxRedirects)
			}
			location := r.header.Get("Location")
			if location == "" {
				return nil, "", errorf(CodeInvalidResponse, "%s: редирект %d без Location", target, r.status)
			}
			next, err := u.Parse(location)
			if err != nil {
				return nil, "", errorf(CodeInvalidResponse, "%s: некорректный Location %q", target, location)
			}
			if r.status == http.StatusMovedPermanently || r.status == http.StatusPermanentRedirect {
				c.moved[target] = next.String()
			}
			c.logf("%s: редирект %d на %s", target, r.status, next)
			target = next.String()
		case r.status == http.StatusUnauthorized:
			return nil, "", ErrUnauthorized
		case r.status == http.StatusNotFound:
			return nil, "", errorf(CodeNotFound, "%s: %s", target, statusText(r.status))
		default:
			return nil, "", errorf(CodeRequestFailed, "%s: неожиданный ответ %s", target, statusText(r.status))
		}
	}
}

// do выполняет запрос к одному адресу. Сетевые ошибки и 5xx повторяются с
// паузами retryDelays, при исчерпании лимита клиент ждёт его восстановления.
// Остальные ответы, включая редиректы и ошибки 4xx, возвращаются как есть.
func (c *Client) do(ctx context.Context, u *url.URL) (*reply, error) {
	target := u.String()
	retries, waits := 0, 0
	for attempt := 1; ; attempt++ {
		started := c.now()
		r, err := c.send(ctx, u)
		if err != nil {
			status := 0
			if r != nil {
				status = r.status
			}
			c.record(started, target, status, attempt, false, classify(err))
			if ctx.Err() != nil {
				return nil, errorf(CodeRequestFailed, "%s: %v", target, ctx.Err())
			}
			if retries == len(c.retryDelays) {
				return nil, errorf(CodeRequestFailed, "%v (попыток: %d)", err, attempt)
			}
			if err := c.pause(ctx, c.retryDelays[retries]); err != nil {
				return nil, err
			}
			retries++
			continue
		}
		c.record(started, target, r.status, attempt, false, "")

		switch {
		case isRateLimited(r):
			if waits == c.maxRateLimitWaits {
				return nil, errorf(CodeRateLimited, "%s: лимит запросов исчерпан, ожиданий: %d", target, waits)
			}
			wait := c.rateLimitDelay(r.header)
			if wait > c.maxRateLimitWait {
				return nil, errorf(CodeRateLimited, "%s: лимит восстановится через %s — дольше допустимых %s",
					target, wait, c.maxRateLimitWait)
			}
			c.logf("%s: лимит запросов исчерпан (%d), ожидание %s", target, r.status, wait)
			if err := c.pause(ctx, wait); err != nil {
				return nil, err
			}
			waits++
		case r.status >= 500:
			if retries == len(c.retryDelays) {
				return nil, errorf(CodeRequestFailed, "%s: сервер ответил %s (попыток: %d)",
					target, statusText(r.status), attempt)
			}
			if err := c.pause(ctx, c.retryDelays[retries]); err != nil {
				return nil, err
			}
			retries++
		default:
			return r, nil
		}
	}
}

// send выполняет одну попытку. При ошибке чтения тела возвращает и ответ, и ошибку.
func (c *Client) send(ctx context.Context, u *url.URL) (*reply, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	if c.token != "" && c.isBaseHost(u) {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	return &reply{status: resp.StatusCode, header: resp.Header, body: body}, err
}

// isBaseHost сообщает, относится ли адрес к серверу из GITHUB_API_BASE_URL:
// только туда отправляется токен.
func (c *Client) isBaseHost(u *url.URL) bool {
	return u.Scheme == c.base.Scheme && strings.EqualFold(u.Host, c.base.Host)
}

func (c *Client) resolveMoved(target string) string {
	for i := 0; i < maxRedirects; i++ {
		next, ok := c.moved[target]
		if !ok {
			break
		}
		target = next
	}
	return target
}

func (c *Client) record(started time.Time, target string, status, attempt int, fromCache bool, errKind string) {
	c.trace.Record(trace.Entry{
		Time:      started,
		Method:    http.MethodGet,
		URL:       target,
		Status:    status,
		Attempt:   attempt,
		FromCache: fromCache,
		Error:     errKind,
	})
}

func (c *Client) pause(ctx context.Context, d time.Duration) error {
	if err := c.sleep(ctx, d); err != nil {
		return errorf(CodeRequestFailed, "ожидание прервано: %v", err)
	}
	return nil
}

// isRateLimited: код 403 или 429 и признак исчерпания лимита — упоминание
// rate limit в теле, заголовок retry-after или x-ratelimit-remaining: 0.
func isRateLimited(r *reply) bool {
	if r.status != http.StatusForbidden && r.status != http.StatusTooManyRequests {
		return false
	}
	if r.header.Get("Retry-After") != "" {
		return true
	}
	if strings.TrimSpace(r.header.Get("X-RateLimit-Remaining")) == "0" {
		return true
	}
	body := strings.ToLower(string(r.body))
	return strings.Contains(body, "rate limit") || strings.Contains(body, "rate-limit") ||
		strings.Contains(body, "ratelimit")
}

// rateLimitDelay: retry-after (секунды или HTTP-дата), иначе до
// x-ratelimit-reset (epoch UTC) с запасом в секунду, иначе 60 с.
func (c *Client) rateLimitDelay(h http.Header) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
		if at, err := http.ParseTime(v); err == nil {
			return max(at.Sub(c.now()), 0)
		}
	}
	if v := strings.TrimSpace(h.Get("X-RateLimit-Reset")); v != "" {
		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil {
			return max(time.Unix(epoch, 0).Sub(c.now()), 0) + time.Second
		}
	}
	return DefaultRateLimitWait
}

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

func classify(err error) string {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout()) {
		return trace.ErrTimeout
	}
	return trace.ErrNetwork
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func statusText(code int) string {
	return fmt.Sprintf("%d %s", code, http.StatusText(code))
}
