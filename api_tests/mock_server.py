"""Мок-сервер, совместимый с контрактом части 2: GET /repos/{owner}/{repo}/issues.

Используется фикстурами conftest.py, а также запускается отдельно — например,
чтобы проверить, что набор тестов падает на сломанном сервере:

    python api_tests/mock_server.py --port 8000 --fault sort_desc

Поведение повторяет GitHub в той части, на которую опирается контракт:
- первая страница — только по адресу с state=open&sort=created&direction=asc&per_page=N,
  неизвестные параметры (например, since или page) отвергаются с 400;
- следующие страницы — по непрозрачным адресам из Link rel="next", которые нельзя
  построить вручную;
- соседние страницы перекрываются на один элемент (повторы number между страницами);
- элементы с одинаковым created_at отдаются в порядке убывания number;
- ETag и 304 на совпавший If-None-Match;
- сценарии ошибок (429, 401, 404, 5xx, обрыв соединения, битый JSON) задаются из тестов.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import math
import threading
import time
from collections import defaultdict
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from random import Random
from typing import Optional, Union
from urllib.parse import parse_qs

MAX_PER_PAGE = 100

# Запросы самих тестов (server_client.py) — «наблюдатели»: они получают обычные
# данные (с включёнными поломками), но не попадают в журнал запросов и не
# расходуют сценарии ошибок, предназначенные для службы.
OBSERVER_AGENT_PREFIX = "ghreport-api-tests/"

# Поломки сервера для самопроверки набора тестов.
FAULTS = {
    "skip_item": "на границе страниц пропускается элемент",
    "unmarked_pr": "PR отдаются без ключа pull_request",
    "always_304": "повторный запрос того же адреса получает 304 даже без If-None-Match",
    "broken_json": "вторая и следующие страницы — битый JSON",
    "sort_desc": "элементы отдаются в порядке убывания created_at",
    "no_link": "нет заголовка Link, хотя следующая страница есть",
}

TITLES = [
    "Ошибка при разборе «кавычек»",
    'Quote "double" & <tag>',
    "Эмодзи 💅 в заголовке",
    "Plain title",
    "Back\\slash and tab\tinside",
    "Длинный заголовок про пагинацию и кэш",
]
LABELS = ["bug", "enhancement", "help wanted", "приоритет: высокий", "good first issue"]


@dataclass
class Reply:
    status: int
    headers: dict = field(default_factory=dict)
    body: Union[bytes, str] = b""
    drop: bool = False  # закрыть соединение, ничего не ответив


@dataclass
class Request:
    method: str
    path: str
    raw_query: str
    query: dict
    headers: dict  # имена в нижнем регистре
    url: str
    time: float  # time.monotonic()


def json_reply(status: int, payload, headers: Optional[dict] = None) -> Reply:
    return Reply(status, dict(headers or {}), json.dumps(payload, ensure_ascii=False))


def make_item(repo: str, number: int, created: datetime, rnd: Random, pr: bool) -> dict:
    kind = "pull" if pr else "issues"
    item = {
        "url": f"https://api.example.test/repos/{repo}/issues/{number}",
        "html_url": f"https://github.example.test/{repo}/{kind}/{number}",
        "number": number,
        "state": "open",
        "title": f"{TITLES[number % len(TITLES)]} #{number}",
        "user": {"login": f"user-{number % 7}", "id": 1000 + number % 7, "type": "User"},
        "labels": [{"id": i, "name": name, "color": "ededed"}
                   for i, name in enumerate(rnd.sample(LABELS, rnd.randint(0, 3)))],
        "comments": number % 5,
        "created_at": created.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "updated_at": (created + timedelta(days=3)).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    if pr:
        item["pull_request"] = {"url": f"https://api.example.test/repos/{repo}/pulls/{number}",
                                "html_url": item["html_url"]}
    return item


def make_items(repo: str, count: int, seed: int, pr_ratio: float = 0.2, midnight: bool = False) -> list:
    """Элементы в порядке created_at по возрастанию. Иногда несколько элементов
    создаются в одну секунду — их сервер отдаёт по убыванию number. При midnight
    в середине есть issue, созданный ровно в полночь UTC (граница --since)."""
    rnd = Random(seed)
    created = datetime(2025, 3, 1, 8, 0, 0, tzinfo=timezone.utc)
    items: list = []
    number = 0
    while len(items) < count:
        if midnight and len(items) >= count // 2:
            midnight = False
            created = (created + timedelta(days=1)).replace(hour=0, minute=0, second=0)
            number += 1
            items.append(make_item(repo, number, created, rnd, pr=False))
            continue
        created += timedelta(hours=rnd.randint(1, 30), minutes=rnd.randint(0, 59), seconds=rnd.randint(1, 59))
        group = []
        for _ in range(1 if not items or rnd.random() < 0.85 else rnd.randint(2, 3)):
            number += 1
            group.append(make_item(repo, number, created, rnd, rnd.random() < pr_ratio))
        items.extend(reversed(group))
    return items[:count]


def default_repos() -> dict:
    return {
        "mock/main": make_items("mock/main", 260, seed=7, midnight=True),
        "mock/small": make_items("mock/small", 7, seed=11, pr_ratio=0.3),
        "mock/second": make_items("mock/second", 40, seed=13),
        "mock/empty": [],
        "mock/only-prs": make_items("mock/only-prs", 4, seed=17, pr_ratio=1.0),
    }


def _encode_cursor(repo: str, offset: int, per_page: int) -> str:
    raw = json.dumps([repo, offset, per_page]).encode()
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _decode_cursor(token: str):
    try:
        raw = base64.urlsafe_b64decode(token + "=" * (-len(token) % 4))
        repo, offset, per_page = json.loads(raw)
        if isinstance(repo, str) and isinstance(offset, int) and isinstance(per_page, int):
            return repo, offset, per_page
    except (ValueError, TypeError):
        pass
    return None


class _Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "ghreport-mock/1.0"

    def log_message(self, format, *args):  # noqa: A002 — сигнатура базового класса
        pass

    def do_GET(self):
        self.server.mock.handle(self)

    do_HEAD = do_POST = do_PUT = do_PATCH = do_DELETE = do_GET


class MockServer:
    """Контрактный сервер в отдельном потоке.

    Состояние для тестов:
      requests      — все запросы службы по порядку (без запросов самих тестов);
      max_inflight  — наибольшее число одновременно обрабатываемых запросов;
      issued_links  — выданные адреса rel="next" в порядке выдачи.
    """

    def __init__(self, repos: Optional[dict] = None, faults=(), require_token: Optional[str] = None,
                 host: str = "127.0.0.1", port: int = 0):
        unknown = set(faults) - set(FAULTS)
        if unknown:
            raise ValueError(f"неизвестные поломки: {sorted(unknown)}")
        self.repos = default_repos() if repos is None else repos
        self.faults = set(faults)
        self.require_token = require_token
        self.requests: list = []
        self.issued_links: list = []
        self.max_inflight = 0
        self._inflight = 0
        self._lock = threading.Lock()
        self._scripts: dict = defaultdict(dict)  # repo -> {номер запроса к репо: Reply}
        self._persistent: dict = {}  # repo -> Reply на каждый запрос
        self._repo_requests: dict = defaultdict(int)
        self._not_before: dict = {}  # repo -> (момент, Reply) пока лимит не восстановлен
        self._served: set = set()  # адреса, уже получившие 200 (для always_304)
        self._httpd = ThreadingHTTPServer((host, port), _Handler)
        self._httpd.daemon_threads = True
        self._httpd.mock = self
        self._thread: Optional[threading.Thread] = None

    # --- жизненный цикл -------------------------------------------------

    @property
    def url(self) -> str:
        host, port = self._httpd.server_address[:2]
        return f"http://{host}:{port}"

    def start(self) -> "MockServer":
        self._thread = threading.Thread(target=self._httpd.serve_forever, daemon=True)
        self._thread.start()
        return self

    def stop(self) -> None:
        self._httpd.shutdown()
        self._httpd.server_close()

    def __enter__(self):
        return self.start()

    def __exit__(self, *exc):
        self.stop()

    # --- настройка сценариев ----------------------------------------------

    def script(self, repo: str, replies: dict) -> None:
        """Подменить ответы: {i: Reply} — ответ на i-й (с 0) запрос к репозиторию."""
        with self._lock:
            self._scripts[repo].update(replies)

    def always(self, repo: str, reply: Reply) -> None:
        """Отвечать reply на любой запрос к репозиторию."""
        with self._lock:
            self._persistent[repo] = reply

    def requests_for(self, repo: str) -> list:
        with self._lock:
            return [r for r in self.requests if self._repo_of(r) == repo]

    # --- обработка запросов -----------------------------------------------

    def handle(self, h: BaseHTTPRequestHandler) -> None:
        # h.path уже нормализован (ведущие // схлопываются), поэтому адрес берётся
        # из исходной строки запроса: двойной слеш — признак ненормализованного base URL.
        target = h.raw_requestline.decode("latin-1").split(" ")[1]
        path, _, raw_query = target.partition("?")
        req = Request(
            method=h.command,
            path=path,
            raw_query=raw_query,
            query=parse_qs(raw_query, keep_blank_values=True),
            headers={k.lower(): v for k, v in h.headers.items()},
            url=f"http://{h.headers.get('Host', '')}{target}",
            time=time.monotonic(),
        )
        observer = req.headers.get("user-agent", "").startswith(OBSERVER_AGENT_PREFIX)
        with self._lock:
            if not observer:
                self.requests.append(req)
            self._inflight += 1
            self.max_inflight = max(self.max_inflight, self._inflight)
        try:
            reply = self._route(req, f"http://{h.headers.get('Host', '')}", observer)
            self._send(h, reply)
        finally:
            with self._lock:
                self._inflight -= 1

    def _route(self, req: Request, origin: str, observer: bool = False) -> Reply:
        if req.method != "GET":
            return json_reply(405, {"message": "Method Not Allowed"})
        if self.require_token is not None and \
                req.headers.get("authorization") != f"Bearer {self.require_token}":
            return json_reply(401, {"message": "Bad credentials"})

        target = self._target(req)
        if isinstance(target, Reply):
            return target
        repo, offset, per_page = target
        if observer:
            if repo not in self.repos:
                return json_reply(404, {"message": "Not Found"})
            return self._page(req, origin, repo, offset, per_page, observer)

        with self._lock:
            index = self._repo_requests[repo]
            self._repo_requests[repo] += 1
            scripted = self._scripts[repo].pop(index, None)
            persistent = self._persistent.get(repo)
            limited = self._not_before.get(repo)
        if limited is not None:
            until, reply = limited
            remaining = until - time.monotonic()
            if remaining > 0:
                # Лимит ещё не восстановился: служба не выждала паузу.
                headers = dict(reply.headers)
                if "Retry-After" in headers:
                    headers["Retry-After"] = str(math.ceil(remaining))
                return Reply(reply.status, headers, reply.body)
        reply = scripted or persistent
        if reply is not None:
            self._remember_rate_limit(repo, reply)
            return reply

        if repo not in self.repos:
            return json_reply(404, {"message": "Not Found"})
        return self._page(req, origin, repo, offset, per_page)

    def _target(self, req: Request):
        """(repo, offset, per_page) или Reply с ошибкой."""
        segments = req.path.split("/")
        if len(segments) == 5 and segments[0] == "" and segments[1] == "repos" and segments[4] == "issues" \
                and segments[2] and segments[3]:
            expected = {"state": "open", "sort": "created", "direction": "asc"}
            for key, value in expected.items():
                if req.query.get(key) != [value]:
                    return json_reply(400, {"message": f"ожидался параметр {key}={value}"})
            per_page = req.query.get("per_page", [""])
            if len(per_page) != 1 or not per_page[0].isdigit() or int(per_page[0]) < 1:
                return json_reply(400, {"message": "ожидался параметр per_page"})
            extra = set(req.query) - set(expected) - {"per_page"}
            if extra:
                return json_reply(400, {"message": f"неожиданные параметры: {sorted(extra)}"})
            return f"{segments[2]}/{segments[3]}", 0, min(int(per_page[0]), MAX_PER_PAGE)
        if len(segments) == 3 and segments[1] == "_cursor" and not req.raw_query:
            cursor = _decode_cursor(segments[2])
            if cursor is not None:
                return cursor
        return json_reply(404, {"message": "Not Found"})

    def _repo_of(self, req: Request) -> Optional[str]:
        target = self._target(req)
        return None if isinstance(target, Reply) else target[0]

    def _remember_rate_limit(self, repo: str, reply: Reply) -> None:
        if reply.status not in (403, 429):
            return
        wait = None
        if "Retry-After" in reply.headers:
            wait = float(reply.headers["Retry-After"])
        elif "X-RateLimit-Reset" in reply.headers:
            wait = float(reply.headers["X-RateLimit-Reset"]) - time.time()
        if wait is not None and wait > 0:
            with self._lock:
                self._not_before[repo] = (time.monotonic() + wait, reply)

    def _page(self, req: Request, origin: str, repo: str, offset: int, per_page: int,
              observer: bool = False) -> Reply:
        items = self.repos[repo]
        if "sort_desc" in self.faults:
            items = list(reversed(items))
        if offset < 0 or (offset >= len(items) and offset != 0):
            return json_reply(404, {"message": "Not Found"})

        page = items[offset:offset + per_page]
        if "unmarked_pr" in self.faults:
            page = [{k: v for k, v in it.items() if k != "pull_request"} for it in page]
        body = json.dumps(page, ensure_ascii=False)
        if "broken_json" in self.faults and offset > 0:
            body = body[: len(body) // 2]

        headers = {"Content-Type": "application/json; charset=utf-8"}
        step = per_page + 1 if "skip_item" in self.faults else per_page - 1  # перекрытие на 1 элемент
        if offset + per_page < len(items) and "no_link" not in self.faults:
            next_url = f"{origin}/_cursor/{_encode_cursor(repo, offset + max(step, 1), per_page)}"
            links = [f'<{next_url}>; rel="next"']
            if offset > 0:
                first = f"{origin}/_cursor/{_encode_cursor(repo, 0, per_page)}"
                links.append(f'<{first}>; rel="first"')
            headers["Link"] = ", ".join(links)
            if not observer:
                with self._lock:
                    self.issued_links.append(next_url)

        etag = 'W/"' + hashlib.sha1((body + headers.get("Link", "")).encode()).hexdigest()[:24] + '"'
        headers["ETag"] = etag
        if_none_match = req.headers.get("if-none-match", "")
        with self._lock:
            served_before = req.url in self._served
            self._served.add(req.url)
        if etag in [v.strip() for v in if_none_match.split(",")] or \
                ("always_304" in self.faults and served_before):
            return Reply(304, headers)
        return Reply(200, headers, body)

    @staticmethod
    def _send(h: BaseHTTPRequestHandler, reply: Reply) -> None:
        if reply.drop:
            h.close_connection = True
            return
        body = reply.body.encode() if isinstance(reply.body, str) else reply.body
        if reply.status == 304:
            body = b""
        h.send_response(reply.status)
        headers = dict(reply.headers)
        if body and "Content-Type" not in headers:
            headers["Content-Type"] = "application/json; charset=utf-8"
        for name, value in headers.items():
            h.send_header(name, value)
        h.send_header("Content-Length", str(len(body)))
        h.end_headers()
        if body and h.command != "HEAD":
            h.wfile.write(body)


def main() -> None:
    parser = argparse.ArgumentParser(description="Мок-сервер контракта ghreport")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--token", help="требовать Authorization: Bearer <token>, иначе 401")
    parser.add_argument("--fault", action="append", default=[], choices=sorted(FAULTS),
                        help="включить поломку (можно несколько раз)")
    args = parser.parse_args()

    server = MockServer(faults=args.fault, require_token=args.token, host=args.host, port=args.port)
    print(f"mock server: {server.url}", flush=True)
    print(f"repos: {', '.join(sorted(server.repos))}", flush=True)
    for name in args.fault:
        print(f"fault: {name} — {FAULTS[name]}", flush=True)
    try:
        server._httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server._httpd.server_close()


if __name__ == "__main__":
    main()
