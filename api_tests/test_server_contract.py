"""Контракт самого сервера GITHUB_API_BASE_URL.

Отчёт службы сверяется с данными сервера, поэтому ошибки сервера (пропуск элемента
при пагинации, PR без pull_request, битый JSON, неверная сортировка, нет Link,
304 без If-None-Match) должны ловиться отдельно — прямыми запросами.
"""

import pytest

from server_client import parse_time, unique_numbers

SAMPLE = 200  # сколько элементов сравнивать


def test_pages_are_json_arrays_with_valid_items(server):
    walk = server.walk()
    walk.ensure_items(SAMPLE)  # Walk проверяет JSON, number, created_at и пометку PR
    assert walk.pages, "сервер не вернул ни одной страницы"


def test_items_are_sorted_by_created_at_ascending(server):
    walk = server.walk()
    walk.ensure_items(SAMPLE)
    times = [parse_time(it["created_at"]) for it in walk.items]
    broken = [(walk.items[i]["number"], walk.items[i + 1]["number"])
              for i in range(len(times) - 1) if times[i] > times[i + 1]]
    assert not broken, f"нарушен порядок created_at по возрастанию: {broken[:5]}"


def test_page_size_does_not_change_the_items(server):
    """Обход страницами по 100 и по 30 даёт одну и ту же последовательность.

    Так ловятся пропуски элементов на границах страниц и отсутствие Link при
    наличии следующей страницы: границы страниц при другом размере другие.
    """
    by_100 = server.walk(per_page=100)
    by_30 = server.walk(per_page=30)
    by_100.ensure_items(SAMPLE)
    by_30.ensure_items(SAMPLE)
    a, b = unique_numbers(by_100.items), unique_numbers(by_30.items)
    n = min(len(a), len(b))
    assert a[:n] == b[:n], "при разном размере страницы элементы отличаются: пропуск на границе страниц?"
    for walk, mine, other in ((by_100, a, b), (by_30, b, a)):
        if walk.done and len(mine) < len(other):
            pytest.fail(f"обход по {walk.per_page} закончился на {len(mine)} элементах, а при другом "
                        f"размере страницы их не меньше {len(other)}: пропущен Link rel=\"next\"?")


def test_not_modified_only_with_matching_if_none_match(server):
    client = server.client
    url = client.issues_url(server.repo)
    first = client.get(url)
    assert first.status == 200, f"{url}: {first.status}"
    etag = first.header("ETag")
    assert etag, "сервер не отдаёт ETag — кэширование невозможно"

    plain = client.get(url)
    assert plain.status == 200, f"на запрос без If-None-Match сервер ответил {plain.status}"
    plain.json()

    bogus = client.get(url, {"If-None-Match": '"ghreport-api-tests-no-such-etag"'})
    assert bogus.status == 200, f"на чужой ETag сервер ответил {bogus.status}"

    conditional = client.get(url, {"If-None-Match": etag})
    assert conditional.status in (200, 304), f"на свой ETag сервер ответил {conditional.status}"
    if conditional.status == 304:
        assert not conditional.body.strip(), "у ответа 304 не должно быть тела"

