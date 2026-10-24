package github

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// nextLink возвращает адрес из заголовка Link с rel="next" или "", если
// следующей страницы нет. Относительный адрес разрешается относительно base —
// адреса, вернувшего страницу.
func nextLink(header http.Header, base string) (string, error) {
	for _, value := range header.Values("Link") {
		for _, part := range splitLinkValue(value) {
			target, rels, ok := parseLinkPart(part)
			if !ok {
				continue
			}
			for _, rel := range rels {
				if !strings.EqualFold(rel, "next") {
					continue
				}
				baseURL, err := url.Parse(base)
				if err != nil {
					return "", err
				}
				next, err := baseURL.Parse(target)
				if err != nil {
					return "", fmt.Errorf("некорректный адрес в Link: %q", target)
				}
				return next.String(), nil
			}
		}
	}
	return "", nil
}

// splitLinkValue делит значение Link на ссылки по запятым вне <...> и кавычек.
func splitLinkValue(value string) []string {
	var parts []string
	inTarget, quoted, start := false, false, 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '<':
			if !quoted {
				inTarget = true
			}
		case '>':
			if !quoted {
				inTarget = false
			}
		case '"':
			if !inTarget {
				quoted = !quoted
			}
		case ',':
			if !inTarget && !quoted {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, value[start:])
}

// parseLinkPart разбирает `<url>; rel="next last"; ...`.
func parseLinkPart(part string) (target string, rels []string, ok bool) {
	part = strings.TrimSpace(part)
	if !strings.HasPrefix(part, "<") {
		return "", nil, false
	}
	end := strings.IndexByte(part, '>')
	if end < 0 {
		return "", nil, false
	}
	target = strings.TrimSpace(part[1:end])
	for _, param := range strings.Split(part[end+1:], ";") {
		key, value, found := strings.Cut(param, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "rel") {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		rels = append(rels, strings.Fields(value)...)
	}
	return target, rels, true
}
