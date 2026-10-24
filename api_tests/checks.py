"""Проверки формата отчёта и журнала по контракту службы."""

from __future__ import annotations

from datetime import timedelta
from typing import Optional

from server_client import parse_time, sort_key

REPORT_KEYS = {"generated_at", "repos"}
REPO_KEYS = {"repo", "error", "count", "issues"}
ISSUE_KEYS = {"number", "title", "author", "labels", "created_at", "url"}
TRACE_KEYS = {"ts", "method", "url", "status", "attempt", "from_cache", "error"}
REPO_ERRORS = {"not_found", "rate_limited", "request_failed", "invalid_response"}


def is_int(value) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def check_utc_time(value, what: str) -> None:
    assert isinstance(value, str), f"{what}: ожидалась строка, получено {value!r}"
    try:
        parsed = parse_time(value)
    except ValueError as exc:
        raise AssertionError(f"{what}: {exc}") from None
    assert parsed.utcoffset() == timedelta(0), f"{what}: время не в UTC: {value}"


def check_issue(issue, where: str) -> None:
    assert isinstance(issue, dict), f"{where}: issue не объект: {issue!r}"
    assert set(issue) == ISSUE_KEYS, f"{where}: поля issue {sorted(issue)}, ожидались {sorted(ISSUE_KEYS)}"
    assert is_int(issue["number"]), f"{where}: number не целое: {issue['number']!r}"
    for key in ("title", "author", "url"):
        assert isinstance(issue[key], str), f"{where}: {key} не строка: {issue[key]!r}"
    assert isinstance(issue["labels"], list) and all(isinstance(x, str) for x in issue["labels"]), \
        f"{where}: labels не список строк: {issue['labels']!r}"
    try:
        parse_time(issue["created_at"])
    except ValueError as exc:
        raise AssertionError(f"{where}: created_at: {exc}") from None


def check_repo_entry(entry) -> None:
    assert isinstance(entry, dict), f"элемент repos не объект: {entry!r}"
    assert set(entry) == REPO_KEYS, f"поля репозитория {sorted(entry)}, ожидались {sorted(REPO_KEYS)}"
    name = entry["repo"]
    assert isinstance(name, str) and name.count("/") == 1, f"repo: {name!r}"
    assert entry["error"] is None or entry["error"] in REPO_ERRORS, f"{name}: неизвестный error {entry['error']!r}"
    assert is_int(entry["count"]), f"{name}: count не целое"
    assert isinstance(entry["issues"], list), f"{name}: issues не список"
    assert entry["count"] == len(entry["issues"]), f"{name}: count={entry['count']}, issues={len(entry['issues'])}"
    if entry["error"] is not None:
        assert entry["issues"] == [], f"{name}: при error={entry['error']} список issues не пуст"
    for issue in entry["issues"]:
        check_issue(issue, name)
    numbers = [issue["number"] for issue in entry["issues"]]
    assert len(numbers) == len(set(numbers)), f"{name}: повторяются number: {numbers}"
    keys = [sort_key(issue) for issue in entry["issues"]]
    assert keys == sorted(keys), f"{name}: issues не отсортированы по created_at, затем number: {numbers}"


def check_report(report, repos: Optional[list] = None) -> None:
    """Обязательные поля, типы, сортировка; repos — ожидаемый порядок репозиториев."""
    assert isinstance(report, dict), f"отчёт не объект: {report!r:.200}"
    assert set(report) == REPORT_KEYS, f"поля отчёта {sorted(report)}, ожидались {sorted(REPORT_KEYS)}"
    check_utc_time(report["generated_at"], "generated_at")
    assert isinstance(report["repos"], list), "repos не список"
    for entry in report["repos"]:
        check_repo_entry(entry)
    if repos is not None:
        assert [entry["repo"] for entry in report["repos"]] == repos, \
            f"порядок репозиториев {[e['repo'] for e in report['repos']]}, ожидался {repos}"


def check_trace(lines: list) -> None:
    assert lines, "журнал пуст"
    for i, line in enumerate(lines, 1):
        where = f"строка журнала {i}"
        assert isinstance(line, dict), f"{where}: не объект"
        assert set(line) == TRACE_KEYS, f"{where}: поля {sorted(line)}, ожидались {sorted(TRACE_KEYS)}"
        check_utc_time(line["ts"], f"{where}: ts")
        assert line["method"] == "GET", f"{where}: method={line['method']!r}"
        assert isinstance(line["url"], str) and line["url"].startswith(("http://", "https://")), \
            f"{where}: url={line['url']!r}"
        assert is_int(line["status"]) and (line["status"] == 0 or 100 <= line["status"] <= 599), \
            f"{where}: status={line['status']!r}"
        assert is_int(line["attempt"]) and line["attempt"] >= 1, f"{where}: attempt={line['attempt']!r}"
        assert isinstance(line["from_cache"], bool), f"{where}: from_cache={line['from_cache']!r}"
        assert line["error"] is None or (isinstance(line["error"], str) and line["error"]), \
            f"{where}: error={line['error']!r}"
        if line["status"] == 0:
            assert line["error"] is not None, f"{where}: status 0 без error"
