package report

import (
	"bytes"
	"testing"
)

func TestEncode(t *testing.T) {
	rep := Report{
		GeneratedAt: "2026-10-14T09:00:01Z",
		Repos: []RepoResult{
			Succeeded("cli/cli", []Issue{{
				Number:    1,
				Title:     "<b> & \"quotes\"",
				Author:    "octocat",
				Labels:    []string{"bug"},
				CreatedAt: "2026-01-05T10:00:00Z",
				URL:       "https://github.com/cli/cli/issues/1",
			}, {
				Number:    2,
				CreatedAt: "2026-01-06T10:00:00Z",
			}}),
			Succeeded("empty/repo", nil),
			Failed("example/missing", "not_found"),
		},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, rep); err != nil {
		t.Fatal(err)
	}
	want := `{
  "generated_at": "2026-10-14T09:00:01Z",
  "repos": [
    {
      "repo": "cli/cli",
      "error": null,
      "count": 2,
      "issues": [
        {
          "number": 1,
          "title": "<b> & \"quotes\"",
          "author": "octocat",
          "labels": [
            "bug"
          ],
          "created_at": "2026-01-05T10:00:00Z",
          "url": "https://github.com/cli/cli/issues/1"
        },
        {
          "number": 2,
          "title": "",
          "author": "",
          "labels": [],
          "created_at": "2026-01-06T10:00:00Z",
          "url": ""
        }
      ]
    },
    {
      "repo": "empty/repo",
      "error": null,
      "count": 0,
      "issues": []
    },
    {
      "repo": "example/missing",
      "error": "not_found",
      "count": 0,
      "issues": []
    }
  ]
}
`
	if got := buf.String(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEncodeFatal(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeFatal(&buf, FatalAllFailed); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "{\"error\":\"all_failed\"}\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
