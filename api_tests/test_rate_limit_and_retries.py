"""Покрытие 5: исчерпание лимита (429/403) и повторы при сетевых ошибках и 5xx.

Мок-сервер следит за паузой: повтор раньше срока снова получает 429/403, поэтому
служба, которая не ждёт, не сможет получить данные. Ожиданий в самих тестах нет —
только таймаут запуска службы.
"""

import json
import time

from mock_server import Reply
from server_client import expected_issues, select_issues, tie_safe_limit

RATE_LIMIT_BODY = json.dumps({"message": "API rate limit exceeded for 127.0.0.1."})
TOLERANCE = 0.05  # погрешность измерения интервалов, с


def repo_requests(mock, repo):
    return mock.mock.requests_for(repo)


def assert_gaps(requests, minimal):
    times = [r.time for r in requests]
    gaps = [b - a for a, b in zip(times, times[1:])]
    for gap, want in zip(gaps, minimal):
        assert gap >= want - TOLERANCE, f"пауза {gap:.2f} с, ожидалось не меньше {want} с (паузы: {gaps})"


def test_429_with_retry_after_waits_and_succeeds(mock, report_of):
    mock.mock.script("mock/small", {0: Reply(429, {"Retry-After": "1"}, RATE_LIMIT_BODY)})
    report, result = report_of(mock, ["mock/small"])

    walk = mock.walk("mock/small")
    assert report["repos"][0]["issues"] == expected_issues(walk, 100)
    assert result.duration >= 1 - TOLERANCE
    trace = result.trace
    assert [(t["status"], t["attempt"]) for t in trace] == [(429, 1), (200, 2)], result.trace_text
    assert trace[0]["url"] == trace[1]["url"]
    assert_gaps(repo_requests(mock, "mock/small"), [1])


def test_403_with_exhausted_limit_waits_until_reset(mock, report_of):
    reset = int(time.time()) + 2
    mock.mock.script("mock/small", {0: Reply(403, {"X-RateLimit-Remaining": "0",
                                                   "X-RateLimit-Reset": str(reset)}, RATE_LIMIT_BODY)})
    report, result = report_of(mock, ["mock/small"])
    assert report["repos"][0]["error"] is None
    assert [t["status"] for t in result.trace] == [403, 200], result.trace_text
    second = repo_requests(mock, "mock/small")[1]
    assert time.time() - (time.monotonic() - second.time) >= reset - TOLERANCE, "повтор раньше x-ratelimit-reset"


def test_rate_limit_in_the_middle_of_pagination(mock, report_of):
    walk = mock.walk()
    walk.ensure_issues(150)
    limit = tie_safe_limit(select_issues(walk.items, 150), 150)
    mock.mock.script(mock.repo, {1: Reply(429, {"Retry-After": "1"}, RATE_LIMIT_BODY)})
    report, result = report_of(mock, [mock.repo], "--limit", str(limit))
    assert report["repos"][0]["issues"] == expected_issues(walk, limit)
    statuses = [t["status"] for t in result.trace]
    assert statuses[:3] == [200, 429, 200], result.trace_text


def test_server_errors_are_retried(mock, report_of):
    mock.mock.script("mock/small", {0: Reply(500), 1: Reply(502)})
    report, result = report_of(mock, ["mock/small"])
    assert report["repos"][0]["issues"] == expected_issues(mock.walk("mock/small"), 100)
    assert [(t["status"], t["attempt"]) for t in result.trace] == [(500, 1), (502, 2), (200, 3)], \
        result.trace_text
    assert_gaps(repo_requests(mock, "mock/small"), [1, 2])


def test_server_errors_exhaust_retries(mock, report_of):
    mock.mock.always("mock/small", Reply(503))
    report, result = report_of(mock, ["mock/small", "mock/second"], expect_code=1)
    failed, ok = report["repos"]
    assert failed["error"] == "request_failed" and ok["error"] is None
    small = [t for t in result.trace if "/repos/mock/small/" in t["url"]]
    assert [(t["status"], t["attempt"]) for t in small] == [(503, 1), (503, 2), (503, 3), (503, 4)], \
        result.trace_text
    assert_gaps(repo_requests(mock, "mock/small"), [1, 2, 4])


def test_dropped_connection_is_retried(mock, report_of):
    mock.mock.script("mock/small", {0: Reply(0, drop=True)})
    report, result = report_of(mock, ["mock/small"])
    assert report["repos"][0]["error"] is None
    first, second = result.trace[:2]
    assert first["status"] == 0 and first["error"] is not None, result.trace_text
    assert (second["status"], second["attempt"]) == (200, 2), result.trace_text
