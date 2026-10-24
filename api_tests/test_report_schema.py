"""Покрытие 7: схема отчёта — обязательные поля, типы, значения, сортировка.

Сама схема проверяется в report_of (checks.check_report) для каждого отчёта
во всех тестах; здесь — значения полей и особые случаи.
"""

import json
from datetime import datetime, timedelta, timezone

from checks import check_report
from server_client import expected_issues, parse_time, select_issues, tie_safe_limit


def test_fields_match_server_data(server, report_of):
    walk = server.walk()
    walk.ensure_issues(31)
    limit = tie_safe_limit(select_issues(walk.items, 31), 30)
    report, _ = report_of(server, [server.repo], "--limit", str(limit))
    entry = report["repos"][0]
    assert entry["repo"] == server.repo and entry["error"] is None
    # number, title, user.login, labels[].name по порядку, created_at как есть, html_url.
    assert entry["issues"] == expected_issues(walk, limit)


def test_generated_at_is_current_utc_time(server, report_of):
    before = datetime.now(timezone.utc).replace(microsecond=0)
    report, _ = report_of(server, [server.repo], "--limit", "1")
    after = datetime.now(timezone.utc) + timedelta(seconds=1)
    generated = parse_time(report["generated_at"])
    assert before - timedelta(seconds=1) <= generated <= after, report["generated_at"]


def test_repos_keep_order_and_drop_duplicates(server, report_of):
    missing = server.missing_repo()
    repos = [missing, server.repo, missing, server.repo]
    report, _ = report_of(server, repos, "--limit", "1", expect_code=1)
    assert [entry["repo"] for entry in report["repos"]] == [missing, server.repo]


def test_issues_with_same_created_at_are_ordered_by_number(mock, report_of):
    report, _ = report_of(mock, [mock.repo], "--limit", "1000")
    issues = report["repos"][0]["issues"]
    ties = [(a["number"], b["number"]) for a, b in zip(issues, issues[1:]) if a["created_at"] == b["created_at"]]
    assert ties, "в данных мок-сервера должны быть issue с одинаковым created_at"
    assert all(a < b for a, b in ties), f"при равном created_at порядок по number: {ties}"


def test_several_repositories(mock, report_of):
    report, _ = report_of(mock, ["mock/small", "mock/empty", "mock/second"], "--limit", "100")
    small, empty, second = report["repos"]
    assert small["issues"] == expected_issues(mock.walk("mock/small"), 100)
    assert empty == {"repo": "mock/empty", "error": None, "count": 0, "issues": []}
    assert second["issues"] == expected_issues(mock.walk("mock/second"), 100)


def test_report_to_file(server, run_ghreport, tmp_path):
    out = tmp_path / "report.json"
    result = run_ghreport(server, "--repos", server.repo, "--limit", "3", "--out", str(out))
    assert result.code == 0, result.describe()
    assert result.stdout == "", "при --out файл отчёт не должен попадать в stdout"
    report = json.loads(out.read_text(encoding="utf-8"))
    check_report(report, [server.repo])


def test_explicit_stdout(server, run_ghreport):
    result = run_ghreport(server, "--repos", server.repo, "--limit", "2", "--out", "-")
    assert result.code == 0, result.describe()
    check_report(json.loads(result.stdout), [server.repo])
