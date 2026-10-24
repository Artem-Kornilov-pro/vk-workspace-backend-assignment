package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"ghreport/internal/config"
	"ghreport/internal/report"
)

// Query — условия отбора issue.
type Query struct {
	Limit int       // не меньше 1
	Since time.Time // нулевое значение — без фильтра
}

// ListIssues возвращает открытые issue репозитория, отсортированные по
// created_at, затем по number.
func (c *Client) ListIssues(ctx context.Context, repo config.Repo, q Query) ([]report.Issue, error) {
	pageURL := c.issuesURL(repo)
	resp, err := c.fetch(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	items, err := decodePage(resp.body)
	if err != nil {
		return nil, errorf(CodeInvalidResponse, "%s: %v", pageURL, err)
	}

	issues := make([]report.Issue, 0, len(items))
	for _, it := range items {
		if it.pullRequest {
			continue
		}
		issues = append(issues, it.Issue)
		if len(issues) == q.Limit {
			break
		}
	}
	return issues, nil
}

// issuesURL — адрес первой страницы. Следующие страницы берутся только из Link.
func (c *Client) issuesURL(repo config.Repo) string {
	return fmt.Sprintf("%s/repos/%s/%s/issues?state=open&sort=created&direction=asc&per_page=100",
		c.baseURL, url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
}

// item — элемент страницы ответа.
type item struct {
	report.Issue
	created     time.Time
	pullRequest bool // в объекте есть ключ pull_request
}

// decodePage разбирает страницу ответа: JSON-массив объектов issue.
// Для PR проверяется только наличие ключа pull_request.
func decodePage(body []byte) ([]item, error) {
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("ожидался JSON-массив объектов: %v", err)
	}
	items := make([]item, 0, len(raw))
	for i, obj := range raw {
		if obj == nil {
			return nil, fmt.Errorf("элемент %d: ожидался объект", i)
		}
		if _, ok := obj["pull_request"]; ok {
			items = append(items, item{pullRequest: true})
			continue
		}
		it, err := decodeIssue(obj)
		if err != nil {
			return nil, fmt.Errorf("элемент %d: %v", i, err)
		}
		items = append(items, it)
	}
	return items, nil
}

func decodeIssue(obj map[string]json.RawMessage) (item, error) {
	var it item
	if err := decodeField(obj, "number", &it.Number, true); err != nil {
		return item{}, err
	}
	if err := decodeField(obj, "created_at", &it.CreatedAt, true); err != nil {
		return item{}, err
	}
	created, err := time.Parse(time.RFC3339, it.CreatedAt)
	if err != nil {
		return item{}, fmt.Errorf("created_at %q: ожидалось время RFC 3339", it.CreatedAt)
	}
	it.created = created
	if err := decodeField(obj, "title", &it.Title, false); err != nil {
		return item{}, err
	}
	if err := decodeField(obj, "html_url", &it.URL, false); err != nil {
		return item{}, err
	}

	var user *struct {
		Login string `json:"login"`
	}
	if err := decodeField(obj, "user", &user, false); err != nil {
		return item{}, err
	}
	if user != nil {
		it.Author = user.Login
	}

	var labels []json.RawMessage
	if err := decodeField(obj, "labels", &labels, false); err != nil {
		return item{}, err
	}
	it.Labels = make([]string, 0, len(labels))
	for _, raw := range labels {
		name, err := labelName(raw)
		if err != nil {
			return item{}, err
		}
		it.Labels = append(it.Labels, name)
	}
	return it, nil
}

// decodeField разбирает поле объекта. Отсутствующее или null необязательное
// поле оставляет dst без изменений.
func decodeField(obj map[string]json.RawMessage, name string, dst any, required bool) error {
	raw, ok := obj[name]
	if !ok || string(raw) == "null" {
		if required {
			return fmt.Errorf("нет обязательного поля %s", name)
		}
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("поле %s: %v", name, err)
	}
	return nil
}

// labelName принимает метку в виде объекта {"name": ...} или строки.
func labelName(raw json.RawMessage) (string, error) {
	var label struct {
		Name *string `json:"name"`
	}
	if err := json.Unmarshal(raw, &label); err == nil && label.Name != nil {
		return *label.Name, nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return name, nil
	}
	return "", errors.New("labels: ожидались объекты с полем name")
}
