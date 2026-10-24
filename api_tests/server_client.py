"""Прямые запросы к серверу GITHUB_API_BASE_URL (только GET).

Тесты не доверяют ни службе, ни серверу: они сами обходят сервер по контракту,
проверяют его ответы и сверяют с ними отчёт ghreport.
"""

from __future__ import annotations

import json
import re
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional
from urllib.parse import quote, urljoin, urlsplit

API_VERSION = "2026-03-10"
USER_AGENT = "ghreport-api-tests/1.0"
MAX_PAGES = 50


class ContractError(AssertionError):
    """Сервер нарушил контракт."""


def parse_time(value: str) -> datetime:
    """RFC 3339 с «Z» (datetime.fromisoformat понимает «Z» только с Python 3.11)."""
    if not isinstance(value, str) or not re.fullmatch(
            r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})", value):
        raise ValueError(f"не время RFC 3339: {value!r}")
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    return parsed


def next_link(header_values: list, base: str) -> Optional[str]:
    """Адрес из Link с rel="next" или None."""
    for value in header_values:
        for match in re.finditer(r"<([^>]*)>((?:\s*;\s*[^;,]+=(?:\"[^\"]*\"|[^;,]*))*)", value):
            target, params = match.group(1), match.group(2)
            for rel in re.findall(r";\s*rel\s*=\s*(?:\"([^\"]*)\"|([^;,\s]*))", params, flags=re.I):
                if "next" in (rel[0] or rel[1]).lower().split():
                    return urljoin(base, target)
    return None


@dataclass
class Response:
    url: str
    status: int
    headers: object  # email.message.Message: get(), get_all()
    body: bytes

    def header(self, name: str) -> Optional[str]:
        return self.headers.get(name)

    def header_all(self, name: str) -> list:
        return self.headers.get_all(name) or []

    def json(self):
        try:
            return json.loads(self.body.decode("utf-8"))
        except (UnicodeDecodeError, ValueError) as exc:
            raise ContractError(f"{self.url}: тело ответа {self.status} не JSON: {exc}") from None


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


class ServerClient:
    def __init__(self, base_url: str, token: Optional[str] = None, timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self._origin = urlsplit(self.base_url)[:2]
        self._opener = urllib.request.build_opener(_NoRedirect)

    def issues_url(self, repo: str, per_page: int = 100) -> str:
        owner, name = repo.split("/", 1)
        return (f"{self.base_url}/repos/{quote(owner, safe='')}/{quote(name, safe='')}/issues"
                f"?state=open&sort=created&direction=asc&per_page={per_page}")

    def get(self, url: str, headers: Optional[dict] = None) -> Response:
        all_headers = {
            "Accept": "application/vnd.github+json",
            "User-Agent": USER_AGENT,
            "X-GitHub-Api-Version": API_VERSION,
        }
        if self.token and urlsplit(url)[:2] == self._origin:
            all_headers["Authorization"] = f"Bearer {self.token}"
        all_headers.update(headers or {})
        request = urllib.request.Request(url, headers=all_headers, method="GET")
        try:
            with self._opener.open(request, timeout=self.timeout) as resp:
                return Response(url, resp.status, resp.headers, resp.read())
        except urllib.error.HTTPError as err:
            return Response(url, err.code, err.headers, err.read())


@dataclass
class Page:
    url: str
    response: Response
    items: list
    next_url: Optional[str]


@dataclass
class Walk:
    """Ленивый обход страниц репозитория по Link rel="next" с проверкой контракта."""

    client: ServerClient
    repo: str
    per_page: int = 100
    pages: list = field(default_factory=list)
    _next: Optional[str] = None
    _started: bool = False

    @property
    def done(self) -> bool:
        return self._started and self._next is None

    @property
    def items(self) -> list:
        return [it for page in self.pages for it in page.items]

    def fetch_next(self) -> Optional[Page]:
        if not self._started:
            self._started = True
            self._next = self.client.issues_url(self.repo, self.per_page)
        if self._next is None:
            return None
        if len(self.pages) >= MAX_PAGES:
            raise ContractError(f"{self.repo}: больше {MAX_PAGES} страниц — обход прерван")
        url = self._next
        if any(page.url == url for page in self.pages):
            raise ContractError(f"{self.repo}: Link rel=\"next\" зациклен на {url}")
        resp = self.client.get(url)
        if resp.status != 200:
            raise ContractError(f"{url}: ожидался 200, получен {resp.status}: {resp.body[:200]!r}")
        items = resp.json()
        if not isinstance(items, list) or not all(isinstance(it, dict) for it in items):
            raise ContractError(f"{url}: ожидался JSON-массив объектов")
        for it in items:
            check_item(it, url)
        page = Page(url, resp, items, next_link(resp.header_all("Link"), url))
        self.pages.append(page)
        self._next = page.next_url
        return page

    def ensure_items(self, count: int) -> None:
        """Дочитать страницы, пока не наберётся count элементов (с повторами и PR)."""
        while len(self.items) < count and self.fetch_next() is not None:
            pass

    def ensure_issues(self, count: int, since: Optional[datetime] = None) -> None:
        """Дочитать страницы, пока не наберётся count подходящих issue."""
        while len(select_issues(self.items, count, since)) < count and self.fetch_next() is not None:
            pass

    def ensure_all(self) -> None:
        while self.fetch_next() is not None:
            pass


def check_item(item: dict, url: str) -> None:
    number = item.get("number")
    if not isinstance(number, int) or isinstance(number, bool):
        raise ContractError(f"{url}: у элемента нет целого number: {item!r:.200}")
    try:
        parse_time(item.get("created_at"))
    except ValueError as exc:
        raise ContractError(f"{url}: элемент {number}: {exc}") from None
    html_url = item.get("html_url")
    if isinstance(html_url, str) and "/pull/" in html_url and "pull_request" not in item:
        raise ContractError(f"{url}: элемент {number} — PR ({html_url}), но без ключа pull_request")


def is_pull_request(item: dict) -> bool:
    return "pull_request" in item


def select_issues(items: list, limit: int, since: Optional[datetime] = None) -> list:
    """Первые limit подходящих issue в порядке сервера: без PR, без повторов
    number, не раньше since — так, как это описано в контракте службы."""
    selected, seen = [], set()
    for it in items:
        if is_pull_request(it) or it["number"] in seen:
            continue
        if since is not None and parse_time(it["created_at"]) < since:
            continue
        seen.add(it["number"])
        selected.append(it)
        if len(selected) == limit:
            break
    return selected


def report_issue(item: dict) -> dict:
    """Ожидаемое представление элемента сервера в отчёте."""
    user = item.get("user") or {}
    return {
        "number": item["number"],
        "title": item.get("title"),
        "author": user.get("login"),
        "labels": [label["name"] if isinstance(label, dict) else label for label in item.get("labels") or []],
        "created_at": item["created_at"],
        "url": item.get("html_url"),
    }


def sort_key(issue: dict):
    return parse_time(issue["created_at"]), issue["number"]


def expected_issues(walk: Walk, limit: int, since: Optional[datetime] = None) -> list:
    walk.ensure_issues(limit, since)
    return sorted((report_issue(it) for it in select_issues(walk.items, limit, since)), key=sort_key)


def tie_safe_limit(issues: list, desired: int) -> int:
    """Наибольший limit ≤ desired, который не разрезает группу issue с одинаковым
    created_at: при разрезе контракт допускает разный выбор элементов внутри группы."""
    def splits(k):
        return 0 < k < len(issues) and issues[k - 1]["created_at"] == issues[k]["created_at"]

    limit = min(desired, len(issues))
    while splits(limit):
        limit -= 1
    if limit == 0 and issues:  # вся первая группа длиннее desired — берём её целиком
        limit = 1
        while splits(limit):
            limit += 1
    return limit


def unique_numbers(items: list) -> list:
    seen, out = set(), []
    for it in items:
        if it["number"] not in seen:
            seen.add(it["number"])
            out.append(it["number"])
    return out
