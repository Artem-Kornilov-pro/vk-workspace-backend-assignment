"""Покрытие 1: пагинация и --limit."""

import pytest

from server_client import expected_issues, select_issues, tie_safe_limit, unique_numbers

PAGE_SIZE = 100  # per_page в запросе службы


def candidates(server, count):
    walk = server.walk()
    walk.ensure_issues(count)
    return walk, select_issues(walk.items, count)


def test_limit_spans_several_pages(server, report_of):
    walk, issues = candidates(server, 250)
    if len(issues) <= PAGE_SIZE:
        pytest.skip(f"в {server.repo} не больше {PAGE_SIZE} issue: несколько страниц не получить")
    limit = tie_safe_limit(issues, 150)
    assert limit > PAGE_SIZE // 2, "слишком длинная группа с одинаковым created_at"

    report, result = report_of(server, [server.repo], "--limit", str(limit))
    got = report["repos"][0]["issues"]
    assert len(got) == limit, result.describe()
    assert got == expected_issues(walk, limit)
    pages = list(dict.fromkeys(line["url"] for line in result.trace))
    assert len(pages) >= 2, f"ожидалось несколько страниц, запрошено: {pages}"


@pytest.mark.parametrize("desired", [1, 7, 99, 100, 101])
def test_exact_limit(server, report_of, desired):
    walk, issues = candidates(server, desired + 1)
    limit = tie_safe_limit(issues, desired)
    if limit == 0:
        pytest.skip("на сервере нет подходящих issue")
    report, result = report_of(server, [server.repo], "--limit", str(limit))
    got = report["repos"][0]["issues"]
    assert [i["number"] for i in got] == [i["number"] for i in expected_issues(walk, limit)], \
        result.describe()


def test_no_duplicates_across_pages(server, report_of):
    walk, issues = candidates(server, 250)
    limit = tie_safe_limit(issues, 250)
    report, _ = report_of(server, [server.repo], "--limit", str(limit))
    numbers = [i["number"] for i in report["repos"][0]["issues"]]
    assert len(numbers) == len(set(numbers)) == limit
    if server.kind == "mock":
        raw = [it["number"] for it in walk.items]
        assert len(raw) > len(set(raw)), "мок-сервер должен повторять элементы на границе страниц"


def test_next_pages_come_from_link(server, report_of):
    walk, issues = candidates(server, 250)
    limit = tie_safe_limit(issues, 250)
    _, result = report_of(server, [server.repo], "--limit", str(limit))
    requested = list(dict.fromkeys(line["url"] for line in result.trace))
    assert requested == [page.url for page in walk.pages[:len(requested)]], \
        "адреса страниц должны браться из Link rel=\"next\""


def test_stops_when_limit_reached(server, report_of):
    walk = server.walk()
    walk.ensure_items(1)
    first_page = select_issues(walk.pages[0].items, PAGE_SIZE)
    if not first_page or walk.pages[0].next_url is None:
        pytest.skip("нужна первая страница с issue и ссылкой на следующую")
    limit = tie_safe_limit(first_page, 5) or 1
    _, result = report_of(server, [server.repo], "--limit", str(limit))
    urls = [line["url"] for line in result.trace]
    assert len(set(urls)) == 1, f"после набора limit запрошены лишние страницы: {urls}"


def test_limit_larger_than_repository(mock, report_of):
    walk = mock.walk("mock/second")
    walk.ensure_all()
    total = len(select_issues(walk.items, 10**6))
    report, result = report_of(mock, ["mock/second"], "--limit", "1000")
    got = report["repos"][0]["issues"]
    assert len(got) == total and got == expected_issues(walk, 1000)
    assert len(unique_numbers(walk.items)) >= total
    assert len(set(line["url"] for line in result.trace)) == len(walk.pages)
