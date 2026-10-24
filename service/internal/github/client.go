// Package github получает открытые issue репозиториев через REST API GitHub
// или совместимого с ним сервера.
package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	UserAgent      = "ghreport/1.0 (vk-workspace-backend-assignment)"
	APIVersion     = "2026-03-10"
	RequestTimeout = 30 * time.Second

	maxBodySize = 64 << 20
)

// Options — параметры клиента. Нулевые значения заменяются значениями по умолчанию.
type Options struct {
	BaseURL    string // без завершающего слеша
	Token      string
	HTTPClient *http.Client
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(opts Options) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: RequestTimeout}
	}
	return &Client{baseURL: opts.BaseURL, token: opts.Token, http: httpClient}
}

type response struct {
	header http.Header
	body   []byte
}

// fetch выполняет один GET и проверяет код ответа.
func (c *Client) fetch(ctx context.Context, url string) (*response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errorf(CodeRequestFailed, "%v", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errorf(CodeRequestFailed, "%v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, errorf(CodeRequestFailed, "чтение ответа %s: %v", url, err)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return &response{header: resp.Header, body: body}, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return nil, errorf(CodeNotFound, "%s: 404 Not Found", url)
	default:
		return nil, errorf(CodeRequestFailed, "%s: неожиданный ответ %s", url, statusText(resp.StatusCode))
	}
}

func statusText(code int) string {
	return fmt.Sprintf("%d %s", code, http.StatusText(code))
}
