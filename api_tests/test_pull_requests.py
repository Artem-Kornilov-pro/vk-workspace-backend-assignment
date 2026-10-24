"""Покрытие 2: PR отсутствуют в отчёте."""

from server_client import expected_issues, is_pull_request, select_issues, tie_safe_limit


def test_report_has_no_pull_requests(server, report_of):
    walk = server.walk()
    walk.ensure_issues(150)
    limit = tie_safe_limit(select_issues(walk.items, 150), 150)
    pull_numbers = {it["number"] for it in walk.items if is_pull_request(it)}

    report, _ = report_of(server, [server.repo], "--limit", str(limit))
    issues = report["repos"][0]["issues"]
    leaked = [i["number"] for i in issues if i["number"] in pull_numbers or "/pull/" in i["url"]]
    assert not leaked, f"в отчёт попали PR: {leaked}"
    # PR не должны и «съедать» место: issue после них тоже попадают в отчёт.
    assert issues == expected_issues(walk, limit)


def test_repository_with_only_pull_requests(mock, report_of):
    report, _ = report_of(mock, ["mock/only-prs"])
    entry = report["repos"][0]
    assert entry["error"] is None and entry["count"] == 0 and entry["issues"] == []
