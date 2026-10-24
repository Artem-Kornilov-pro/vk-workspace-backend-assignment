"""Покрытие 3: --since оставляет только issue, созданные не раньше даты (00:00:00 UTC)."""

from datetime import datetime, time, timezone

import pytest

from server_client import expected_issues, parse_time, select_issues, tie_safe_limit


def midnight(value: str) -> datetime:
    day = parse_time(value).astimezone(timezone.utc).date()
    return datetime.combine(day, time(), tzinfo=timezone.utc)


def run_since(server, report_of, since: datetime, desired: int):
    walk = server.walk()
    walk.ensure_issues(desired + 1, since)
    limit = tie_safe_limit(select_issues(walk.items, desired + 1, since), desired)
    if limit == 0:
        pytest.skip("нет issue не раньше выбранной даты")
    report, result = report_of(server, [server.repo], "--limit", str(limit),
                               "--since", since.strftime("%Y-%m-%d"))
    issues = report["repos"][0]["issues"]
    assert issues == expected_issues(walk, limit, since), result.describe()
    for issue in issues:
        assert parse_time(issue["created_at"]) >= since, f"issue {issue['number']} создан раньше {since}"
    return issues


def test_since_filters_by_creation_date(server, report_of):
    walk = server.walk()
    walk.ensure_issues(150)
    issues = select_issues(walk.items, 150)
    since = midnight(issues[len(issues) // 2]["created_at"])
    assert any(parse_time(i["created_at"]) < since for i in issues), "нужны issue раньше даты"
    run_since(server, report_of, since, 40)


def test_since_includes_issue_created_at_midnight(mock, report_of):
    walk = mock.walk()
    walk.ensure_all()
    at_midnight = [it for it in select_issues(walk.items, 10**6) if it["created_at"].endswith("T00:00:00Z")]
    assert at_midnight, "в данных мок-сервера должен быть issue, созданный ровно в полночь"
    since = midnight(at_midnight[0]["created_at"])
    issues = run_since(mock, report_of, since, 30)
    assert issues[0]["number"] == at_midnight[0]["number"]


def test_since_in_future_gives_empty_list(server, report_of):
    walk = server.walk()
    walk.ensure_issues(1)
    if not select_issues(walk.items, 1):
        pytest.skip("на сервере нет issue")
    report, _ = report_of(server, [server.repo], "--limit", "1", "--since", "2999-01-01")
    entry = report["repos"][0]
    assert entry["error"] is None and entry["issues"] == []


def test_since_does_not_use_api_parameter(mock, report_of):
    report_of(mock, [mock.repo], "--limit", "5", "--since", "2025-04-01")
    for request in mock.mock.requests:
        assert "since" not in request.query, f"параметр since передан серверу: {request.url}"
