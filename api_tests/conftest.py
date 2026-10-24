"""Общая инфраструктура API-тестов ghreport.

Переменные окружения:
  GHREPORT_BIN         абсолютный путь к собранной службе (обязательно)
  GITHUB_API_BASE_URL  контрактный сервер; если задан, тесты с данными идут и против него
  GITHUB_TOKEN         токен для этого сервера (необязательно)
  GHREPORT_TEST_REPO   репозиторий с данными на этом сервере (по умолчанию cli/cli)

Тесты с данными параметризованы фикстурой server: «mock» — встроенный мок-сервер,
«external» — GITHUB_API_BASE_URL. Сценарии, которые нельзя гарантировать на чужом
сервере (429, 401, обрывы, 5xx), всегда идут против мок-сервера (фикстура mock).
"""

from __future__ import annotations

import itertools
import json
import os
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import pytest

from checks import check_report, check_trace
from mock_server import MockServer
from server_client import ServerClient, Walk

EXTERNAL_URL = os.environ.get("GITHUB_API_BASE_URL", "").strip()
EXTERNAL_TOKEN = os.environ.get("GITHUB_TOKEN", "").strip() or None
EXTERNAL_REPO = os.environ.get("GHREPORT_TEST_REPO", "").strip() or "cli/cli"
MOCK_REPO = "mock/main"
RUN_TIMEOUT = 120  # секунд на один запуск службы

SERVER_KINDS = ["mock"] + (["external"] if EXTERNAL_URL else [])


def pytest_configure(config):
    binary = os.environ.get("GHREPORT_BIN", "")
    if not binary:
        raise pytest.UsageError("задайте GHREPORT_BIN — абсолютный путь к собранной службе ghreport")
    if not os.path.isabs(binary) or not os.access(binary, os.X_OK):
        raise pytest.UsageError(f"GHREPORT_BIN={binary!r}: нужен абсолютный путь к исполняемому файлу")


def pytest_report_header(config):
    lines = [f"ghreport: {os.environ.get('GHREPORT_BIN')}"]
    if EXTERNAL_URL:
        lines.append(f"external server: {EXTERNAL_URL} (repo {EXTERNAL_REPO}, "
                     f"token {'задан' if EXTERNAL_TOKEN else 'не задан'})")
    else:
        lines.append("external server: не задан (GITHUB_API_BASE_URL), только мок-сервер")
    return lines


@dataclass
class Server:
    """Сервер, против которого запускается служба."""

    kind: str  # "mock" или "external"
    base_url: str
    repo: str  # репозиторий с данными
    token: Optional[str]
    mock: Optional[MockServer] = None
    _walks: dict = field(default_factory=dict)

    @property
    def client(self) -> ServerClient:
        return ServerClient(self.base_url, self.token)

    def walk(self, repo: Optional[str] = None, per_page: int = 100) -> Walk:
        """Обход сервера, общий для тестов сессии: страницы запрашиваются один раз."""
        key = (repo or self.repo, per_page)
        if key not in self._walks:
            self._walks[key] = Walk(self.client, key[0], per_page)
        return self._walks[key]

    def missing_repo(self) -> str:
        return f"ghreport-api-tests/missing-{uuid.uuid4().hex[:12]}"


def _mock_server(mock: MockServer) -> Server:
    return Server("mock", mock.url, MOCK_REPO, f"api-tests-token-{uuid.uuid4().hex}", mock)


@pytest.fixture(scope="session")
def session_mock():
    with MockServer() as mock:
        yield mock


@pytest.fixture(scope="session", params=SERVER_KINDS)
def server(request, session_mock) -> Server:
    """Сервер с данными: встроенный мок и, если задан, GITHUB_API_BASE_URL."""
    if request.param == "mock":
        return _mock_server(session_mock)
    return Server("external", EXTERNAL_URL.rstrip("/"), EXTERNAL_REPO, EXTERNAL_TOKEN)


@pytest.fixture
def mock() -> Server:
    """Свой мок-сервер на тест: для сценариев ошибок и проверки запросов службы."""
    with MockServer() as instance:
        yield _mock_server(instance)


@dataclass
class Result:
    args: list
    code: int
    stdout: str
    stderr: str
    duration: float
    trace_text: str
    cache_dir: Path

    @property
    def trace(self) -> list:
        lines = []
        for i, raw in enumerate(self.trace_text.splitlines(), 1):
            try:
                lines.append(json.loads(raw))
            except ValueError:
                raise AssertionError(f"строка журнала {i} не JSON: {raw!r}") from None
        return lines

    def describe(self) -> str:
        return (f"ghreport {' '.join(self.args)}\n  код {self.code}, {self.duration:.1f} с\n"
                f"  stdout: {self.stdout[:2000]}\n  stderr: {self.stderr[:2000]}")


_USE_SERVER_TOKEN = object()


@pytest.fixture
def run_ghreport(tmp_path):
    """Запуск службы процессом: run_ghreport(server, *args, ...) -> Result.

    По умолчанию у каждого запуска свой каталог кэша внутри tmp_path теста и
    журнал --trace; общий кэш передаётся через cache_dir.
    """
    counter = itertools.count(1)
    binary = os.environ["GHREPORT_BIN"]

    def run(server: Server, *args: str, token=_USE_SERVER_TOKEN, cache_dir: Optional[Path] = None,
            trace: bool = True, base_url: Optional[str] = None, timeout: float = RUN_TIMEOUT) -> Result:
        n = next(counter)
        cache_dir = cache_dir or tmp_path / f"cache-{n}"
        trace_path = tmp_path / f"trace-{n}.jsonl"
        cmd = [binary, *args, "--cache-dir", str(cache_dir)]
        if trace:
            cmd += ["--trace", str(trace_path)]

        env = {k: v for k, v in os.environ.items() if not k.startswith("GITHUB_")}
        env["GITHUB_API_BASE_URL"] = base_url if base_url is not None else server.base_url
        token = server.token if token is _USE_SERVER_TOKEN else token
        if token:
            env["GITHUB_TOKEN"] = token
        env["NO_PROXY"] = env["no_proxy"] = "127.0.0.1,localhost"

        started = time.monotonic()
        proc = subprocess.run(cmd, env=env, cwd=tmp_path, capture_output=True, timeout=timeout)
        duration = time.monotonic() - started
        trace_text = trace_path.read_text(encoding="utf-8") if trace and trace_path.exists() else ""
        return Result([str(a) for a in cmd[1:]], proc.returncode, proc.stdout.decode("utf-8", "replace"),
                      proc.stderr.decode("utf-8", "replace"), duration, trace_text, cache_dir)

    return run


@pytest.fixture
def report_of(run_ghreport):
    """Запустить службу, проверить код возврата, схему отчёта и журнала.

    report_of(server, [repos...], *args, expect_code=0) -> (report, result)
    """

    def report_of(server: Server, repos: list, *args: str, expect_code: int = 0, **kwargs):
        result = run_ghreport(server, "--repos", ",".join(repos), *args, **kwargs)
        assert result.code == expect_code, result.describe()
        try:
            report = json.loads(result.stdout)
        except ValueError:
            raise AssertionError(f"stdout не JSON\n{result.describe()}") from None
        unique = list(dict.fromkeys(repos))
        check_report(report, unique)
        if kwargs.get("trace", True):
            check_trace(result.trace)
        return report, result

    return report_of
