"""Минимальная проверка: служба запускается и строит отчёт по серверу с данными."""


def test_report_for_repo_with_data(server, report_of):
    report, result = report_of(server, [server.repo], "--limit", "5")
    entry = report["repos"][0]
    assert entry["error"] is None and entry["count"] == 5, result.describe()
