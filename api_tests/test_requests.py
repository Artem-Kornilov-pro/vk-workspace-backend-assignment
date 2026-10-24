"""Запросы службы к серверу: адрес, заголовки, последовательность, переход по Link.

Проверяется на мок-сервере, который записывает все полученные запросы.
"""

CONTRACT_QUERY = {"state": ["open"], "sort": ["created"], "direction": ["asc"], "per_page": ["100"]}


def test_request_headers(mock, report_of):
    report_of(mock, [mock.repo], "--limit", "150")
    assert len(mock.mock.requests) >= 2
    for request in mock.mock.requests:
        assert request.method == "GET"
        assert request.headers.get("accept") == "application/vnd.github+json", request.headers
        assert request.headers.get("user-agent", "").strip(), "пустой User-Agent"
        assert request.headers.get("x-github-api-version") == "2026-03-10", request.headers
        assert request.headers.get("authorization") == f"Bearer {mock.token}", "нет Authorization: Bearer"


def test_no_authorization_without_token(mock, report_of):
    report_of(mock, ["mock/small"], token=None)
    for request in mock.mock.requests:
        assert "authorization" not in request.headers


def test_first_page_address(mock, report_of):
    report_of(mock, ["mock/small", "mock/second"], "--since", "2025-03-05", "--limit", "3")
    first_pages = [r for r in mock.mock.requests if r.path.startswith("/repos/")]
    assert [r.path for r in first_pages] == ["/repos/mock/small/issues", "/repos/mock/second/issues"]
    for request in first_pages:
        assert request.query == CONTRACT_QUERY, request.url


def test_next_pages_only_from_link(mock, report_of):
    report_of(mock, [mock.repo], "--limit", "1000")
    later = [r.url for r in mock.mock.requests[1:]]
    assert later == mock.mock.issued_links[:len(later)], "следующие страницы — только по Link rel=\"next\""
    assert len(later) >= 2


def test_requests_are_sequential(mock, report_of):
    report_of(mock, [mock.repo, "mock/small", "mock/second"], "--limit", "1000")
    assert mock.mock.max_inflight == 1, "запросы должны выполняться последовательно"


def test_trailing_slash_in_base_url(mock, report_of):
    report, _ = report_of(mock, ["mock/small"], base_url=mock.base_url + "/")
    assert report["repos"][0]["error"] is None
    assert all("//" not in r.path for r in mock.mock.requests)
