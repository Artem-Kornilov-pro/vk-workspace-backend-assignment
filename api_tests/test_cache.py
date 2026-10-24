"""Покрытие 4: повторный запуск с тем же кэшем использует 304 и даёт тот же отчёт."""

from server_client import select_issues, tie_safe_limit


def cache_text(cache_dir) -> str:
    return "\n".join(p.read_text(encoding="utf-8", errors="replace")
                     for p in cache_dir.rglob("*") if p.is_file())


def test_second_run_is_served_from_cache(server, report_of, tmp_path):
    walk = server.walk()
    walk.ensure_issues(150)
    limit = tie_safe_limit(select_issues(walk.items, 150), 150)
    cache = tmp_path / "shared-cache"

    first, first_run = report_of(server, [server.repo], "--limit", str(limit), cache_dir=cache)
    assert all(not line["from_cache"] for line in first_run.trace), first_run.trace_text
    assert any(cache.rglob("*")), "кэш пуст после первого запуска"

    second, second_run = report_of(server, [server.repo], "--limit", str(limit), cache_dir=cache)
    trace = second_run.trace
    assert all(line["from_cache"] for line in trace), f"второй запуск не из кэша:\n{second_run.trace_text}"
    assert all(line["status"] == 304 for line in trace), second_run.trace_text
    assert [line["url"] for line in trace] == [line["url"] for line in first_run.trace]
    assert second["repos"] == first["repos"]

    if server.token:
        assert server.token not in cache_text(cache), "токен попал в кэш"


def test_conditional_requests_use_etag(mock, report_of, tmp_path):
    cache = tmp_path / "shared-cache"
    report_of(mock, [mock.repo], "--limit", "150", cache_dir=cache)
    first = list(mock.mock.requests)
    assert all("if-none-match" not in r.headers for r in first), "If-None-Match без кэша"

    report_of(mock, [mock.repo], "--limit", "150", cache_dir=cache)
    second = mock.mock.requests[len(first):]
    assert [r.url for r in second] == [r.url for r in first]
    for request in second:
        assert request.headers.get("if-none-match"), f"нет If-None-Match: {request.url}"


def test_changed_data_is_refetched(mock, report_of, tmp_path):
    cache = tmp_path / "shared-cache"
    report_of(mock, ["mock/small"], cache_dir=cache)
    items = mock.mock.repos["mock/small"]
    index = next(i for i, it in enumerate(items) if "pull_request" not in it)
    items[index] = dict(items[index], title="изменённый заголовок")
    number = items[index]["number"]

    # Данные изменились: 200 и новое содержимое, кэш обновляется.
    second, run = report_of(mock, ["mock/small"], cache_dir=cache)
    assert [line["from_cache"] for line in run.trace] == [False], run.trace_text
    assert {i["number"]: i["title"] for i in second["repos"][0]["issues"]}[number] == "изменённый заголовок"

    # Следующий запуск снова из кэша, уже с новыми данными.
    third, run = report_of(mock, ["mock/small"], cache_dir=cache)
    assert [line["from_cache"] for line in run.trace] == [True], run.trace_text
    assert third["repos"] == second["repos"]
