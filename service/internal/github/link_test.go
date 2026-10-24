package github

import (
	"net/http"
	"testing"
)

func TestNextLink(t *testing.T) {
	const base = "https://api.github.com/repos/o/r/issues?per_page=100"
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"none", nil, ""},
		{"github style",
			[]string{`<https://api.github.com/repositories/1/issues?page=2>; rel="next", <https://api.github.com/repositories/1/issues?page=5>; rel="last"`},
			"https://api.github.com/repositories/1/issues?page=2"},
		{"next is not first",
			[]string{`<https://x/prev>; rel="prev", <https://x/next>; rel="next"`}, "https://x/next"},
		{"several rels", []string{`<https://x/n>; rel="next last"`}, "https://x/n"},
		{"unquoted and case", []string{`<https://x/n>; REL=Next`}, "https://x/n"},
		{"comma inside url", []string{`<https://x/n?cursor=a,b>; rel="next"`}, "https://x/n?cursor=a,b"},
		{"comma inside quoted param", []string{`<https://x/a>; title="a, b"; rel="prev", <https://x/b>; rel="next"`},
			"https://x/b"},
		{"several headers", []string{`<https://x/first>; rel="first"`, `<https://x/next>; rel="next"`}, "https://x/next"},
		{"relative", []string{`</repos/o/r/issues?cursor=abc>; rel="next"`},
			"https://api.github.com/repos/o/r/issues?cursor=abc"},
		{"only last", []string{`<https://x/last>; rel="last"`}, ""},
		{"garbage", []string{`nonsense, ; rel=next`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			for _, v := range tt.values {
				header.Add("Link", v)
			}
			got, err := nextLink(header, base)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
