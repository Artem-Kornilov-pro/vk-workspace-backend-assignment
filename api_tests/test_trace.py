"""Покрытие 8: журнал --trace — набор полей, одна строка на попытку, нет Authorization.

Набор полей и типы каждой строки проверяются в report_of (checks.check_trace)
во всех тестах; здесь — соответствие запросам и отсутствие секретов.
"""

from urllib.parse import parse_qs, urlsplit

import pytest

from server_client import select_issues, tie_safe_limit

CONTRACT_QUERY = {"state": ["open"], "sort": ["created"], "direction": ["asc"], "per_page": ["100"]}


def test_first_request_matches_contract(server, report_of):
    _, result = report_of(server, [server.repo], "--limit", "1")
    first = urlsplit(result.trace[0]["url"])
    base = urlsplit(server.base_url)
    assert (first.scheme, first.netloc) == (base.scheme, base.netloc)
    assert first.path == f"{base.path.rstrip('/')}/repos/{server.repo}/issues"
    assert parse_qs(first.query, keep_blank_values=True) == CONTRACT_QUERY
    assert result.trace[0]["attempt"] == 1


def test_one_line_per_request(mock, report_of):
    _, result = report_of(mock, [mock.repo, "mock/small"], "--limit", "150")
    requests = mock.mock.requests
    assert len(result.trace) == len(requests)
    assert [line["url"] for line in result.trace] == [r.url for r in requests]
    assert [line["status"] for line in result.trace] == [200] * len(requests)


def test_no_secrets_in_outputs(server, report_of):
    if not server.token:
        pytest.skip("токен не задан — проверять нечего")
    walk = server.walk()
    walk.ensure_issues(150)
    limit = tie_safe_limit(select_issues(walk.items, 150), 150)
    _, result = report_of(server, [server.repo, server.missing_repo()], "--limit", str(limit), expect_code=1)
    cache = "\n".join(p.read_text(encoding="utf-8", errors="replace")
                      for p in result.cache_dir.rglob("*") if p.is_file())
    for name, text in {"журнал": result.trace_text, "stdout": result.stdout,
                       "stderr": result.stderr, "кэш": cache}.items():
        assert server.token not in text, f"токен попал в {name}"
    lowered = result.trace_text.lower()
    assert "authorization" not in lowered and "bearer" not in lowered, "заголовок Authorization в журнале"

