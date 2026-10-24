"""Покрытие 6: коды возврата при 401, 404 и некорректных аргументах."""

import json

import pytest

from conftest import Server
from mock_server import MockServer, Reply
from server_client import expected_issues, select_issues, tie_safe_limit


def fatal_error(result) -> str:
    assert result.code == 3, result.describe()
    try:
        payload = json.loads(result.stdout)
    except ValueError:
        raise AssertionError(f"stdout не JSON\n{result.describe()}") from None
    assert isinstance(payload, dict) and set(payload) == {"error"}, result.describe()
    return payload["error"]


@pytest.mark.parametrize("token", ["wrong-token", None], ids=["wrong-token", "no-token"])
def test_unauthorized_exits_with_code_3(run_ghreport, token):
    with MockServer(require_token="right-token") as instance:
        server = Server("mock", instance.url, "mock/small", token, instance)
        result = run_ghreport(server, "--repos", "mock/small")
    assert fatal_error(result) == "unauthorized"


def test_unauthorized_after_successful_repo(mock, run_ghreport):
    mock.mock.always("mock/second", Reply(401, body=json.dumps({"message": "Bad credentials"})))
    result = run_ghreport(mock, "--repos", "mock/small,mock/second")
    assert fatal_error(result) == "unauthorized"


def test_one_missing_repo_gives_code_1(server, report_of):
    walk = server.walk()
    walk.ensure_issues(11)
    limit = tie_safe_limit(select_issues(walk.items, 11), 10) or 1
    missing = server.missing_repo()
    report, _ = report_of(server, [server.repo, missing], "--limit", str(limit), expect_code=1)
    found, failed = report["repos"]
    assert found["error"] is None and found["issues"] == expected_issues(walk, limit)
    assert failed == {"repo": missing, "error": "not_found", "count": 0, "issues": []}


def test_single_missing_repo_is_all_failed(server, run_ghreport):
    result = run_ghreport(server, "--repos", server.missing_repo())
    assert fatal_error(result) == "all_failed"


def test_all_repos_missing(server, run_ghreport):
    result = run_ghreport(server, "--repos", f"{server.missing_repo()},{server.missing_repo()}")
    assert fatal_error(result) == "all_failed"


def test_broken_json_is_invalid_response(mock, report_of):
    mock.mock.always("mock/second", Reply(200, {"Content-Type": "application/json"}, '[{"number": 1,'))
    report, _ = report_of(mock, ["mock/small", "mock/second"], expect_code=1)
    assert report["repos"][1]["error"] == "invalid_response"


def test_not_json_array_is_invalid_response(mock, report_of):
    mock.mock.always("mock/second", Reply(200, {}, json.dumps({"message": "not a list"})))
    report, _ = report_of(mock, ["mock/small", "mock/second"], expect_code=1)
    assert report["repos"][1]["error"] == "invalid_response"


@pytest.mark.parametrize("args", [
    [],
    ["--repos"],
    ["--repos", ""],
    ["--repos", "no-slash"],
    ["--repos", "a/b/c"],
    ["--repos", "mock/small", "--limit", "0"],
    ["--repos", "mock/small", "--limit", "-5"],
    ["--repos", "mock/small", "--limit", "ten"],
    ["--repos", "mock/small", "--limit", "1.5"],
    ["--repos", "mock/small", "--since", "2026-13-40"],
    ["--repos", "mock/small", "--since", "yesterday"],
    ["--repos", "mock/small", "--no-such-flag"],
], ids=lambda args: " ".join(args) or "no-args")
def test_invalid_arguments(mock, run_ghreport, args):
    result = run_ghreport(mock, *args)
    assert result.code == 2, result.describe()
    assert result.stdout == "", "при ошибке аргументов stdout должен быть пуст"
    assert result.stderr.strip(), "сообщение об ошибке должно быть в stderr"
    assert mock.mock.requests == [], "при ошибке аргументов запросов быть не должно"
