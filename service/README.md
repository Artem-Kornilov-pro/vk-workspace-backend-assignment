# ghreport

Часть 2: сводка по открытым issue репозиториев GitHub. Go ≥ 1.22, только стандартная библиотека (без клиентских SDK GitHub).

## Сборка и запуск

```sh
cd service
go build -o ghreport ./cmd/ghreport
GITHUB_TOKEN=... ./ghreport --repos cli/cli,golang/go --limit 20 --since 2026-01-01
```

Docker:

```sh
docker build -t ghreport service/
docker run --rm -e GITHUB_TOKEN=... ghreport --repos cli/cli --limit 20 --out -
docker run --rm -e GITHUB_API_BASE_URL=https://ghe.example.com/api/v3 ghreport --repos team/repo
```

Образ собирается в два этапа: `golang:1.26-alpine` собирает статический бинарник (`CGO_ENABLED=0`), итоговый образ — `gcr.io/distroless/static-debian12:nonroot` (≈ 3 МБ, только бинарник и CA-сертификаты), запуск от непривилегированного пользователя `nonroot` (uid 65532). Секретов в образе нет, токен и адрес сервера передаются только через `-e`. Чтобы получить файлы `--out`, `--trace` или сохранить кэш между запусками, смонтируйте каталог: `-v "$PWD/data:/home/nonroot/data" ... --out data/report.json --cache-dir data/cache`.

Если сервер запущен на хосте, `localhost` внутри контейнера его не увидит: используйте `--network host` (Linux) или адрес `http://host.docker.internal:<порт>` (Docker Desktop).

## Параметры

```
ghreport --repos <owner/repo[,owner/repo...]>
         [--limit <N>] [--since <YYYY-MM-DD>]
         [--out <file|->] [--trace <file>] [--cache-dir <dir>]
```

| Параметр | По умолчанию | Описание |
|---|---|---|
| `--repos` | обязателен | репозитории через запятую; пробелы вокруг имён допускаются, дубликаты удаляются с сохранением первого вхождения (сравнение точное, с учётом регистра) |
| `--limit` | `100` | максимум issue на репозиторий, целое ≥ 1 |
| `--since` | — | только issue, созданные не раньше 00:00:00 UTC этой даты |
| `--out` | `-` | файл отчёта; `-` — stdout |
| `--trace` | — | журнал HTTP-запросов (JSON Lines); файл перезаписывается при каждом запуске |
| `--cache-dir` | `./.ghreport-cache` | каталог кэша, создаётся автоматически |

Параметры принимаются в формах `--name value`, `--name=value` и `-name value`. `-h`/`--help` печатает справку.

## Переменные окружения

| Переменная | Описание |
|---|---|
| `GITHUB_TOKEN` | необязательный токен; отправляется как `Authorization: Bearer <token>` и только на хост из `GITHUB_API_BASE_URL` |
| `GITHUB_API_BASE_URL` | адрес API, по умолчанию `https://api.github.com`; завершающий слеш допускается, путь сохраняется (`https://ghe.example.com/api/v3`) |

## Коды возврата

| Код | Ситуация | stdout |
|---|---|---|
| 0 | все репозитории получены | отчёт |
| 1 | часть репозиториев с ошибкой | отчёт, у проблемных заполнено `error` |
| 2 | некорректные аргументы или окружение | пусто; сообщение в stderr |
| 3 | ответ 401 или ошибка по всем репозиториям | `{"error":"unauthorized"}` или `{"error":"all_failed"}` |

stdout содержит только отчёт или фатальный JSON, вся диагностика (ошибки репозиториев, ожидание лимита, редиректы) — в stderr. При коде 3 отчёт не пишется ни в stdout, ни в `--out`.

Коды ошибок репозитория: `not_found` (404), `rate_limited` (лимит не восстановился), `request_failed` (сеть, таймаут или 5xx после повторов, другие коды 4xx и 3xx), `invalid_response` (тело не JSON-массив issue, битые поля, 304 без закэшированного ответа, некорректный адрес в `Link`/`Location`).

## Пример вывода

```
$ ghreport --repos cli/cli,example/missing --limit 1
ghreport: example/missing: not_found: https://api.github.com/repos/example/missing/issues?state=open&sort=created&direction=asc&per_page=100: 404 Not Found
{
  "generated_at": "2026-10-24T11:47:06Z",
  "repos": [
    {
      "repo": "cli/cli",
      "error": null,
      "count": 1,
      "issues": [
        {
          "number": 136,
          "title": "Add better messaging to address archived repos",
          "author": "tierninho",
          "labels": [
            "enhancement",
            "core",
            "gh-pr"
          ],
          "created_at": "2019-12-05T02:03:22Z",
          "url": "https://github.com/cli/cli/issues/136"
        }
      ]
    },
    {
      "repo": "example/missing",
      "error": "not_found",
      "count": 0,
      "issues": []
    }
  ]
}
$ echo $?
1
```

Журнал `--trace` того же запуска с кэшем, оставшимся от предыдущего:

```
{"ts":"2026-10-24T11:47:05Z","method":"GET","url":"https://api.github.com/repos/cli/cli/issues?state=open&sort=created&direction=asc&per_page=100","status":304,"attempt":1,"from_cache":true,"error":null}
{"ts":"2026-10-24T11:47:06Z","method":"GET","url":"https://api.github.com/repos/example/missing/issues?state=open&sort=created&direction=asc&per_page=100","status":404,"attempt":1,"from_cache":false,"error":null}
```

## Как это работает

### Запросы

Первая страница: `GET {base}/repos/{owner}/{repo}/issues?state=open&sort=created&direction=asc&per_page=100` с заголовками `Accept: application/vnd.github+json`, `User-Agent: ghreport/1.0 (vk-workspace-backend-assignment)` (имя и версия программы и проект, к которому она относится, — по рекомендации GitHub указывать в User-Agent приложение), `X-GitHub-Api-Version: 2026-03-10` и `Authorization: Bearer` при наличии токена. Все запросы последовательные, таймаут одного запроса — 30 с.

Следующая страница берётся только из `Link: <...>; rel="next"` (адрес не строится вручную; относительный адрес разрешается относительно текущей страницы). Обход прекращается, когда набрано `--limit` подходящих issue или `next` нет; если `next` указывает на уже запрошенный адрес, обход тоже прекращается (защита от цикла).

### Отбор

Элементы страницы обрабатываются по порядку: PR (есть ключ `pull_request`, даже со значением `null`) пропускаются, issue с `created_at` раньше `--since` пропускаются (параметр API `since` не используется — он фильтрует по `updated_at`), повторный `number` пропускается. Итоговый список сортируется по `created_at` (как время, с учётом смещения), затем по `number`. Поля отчёта: `number`, `title`, `user.login` → `author`, `labels[].name` в порядке ответа, `created_at` как в ответе, `html_url` → `url`. Отсутствующие или `null` `title`, `user`, `labels` дают пустые значения; без `number` или с `created_at` не в RFC 3339 ответ считается некорректным.

### Редиректы

Обрабатываются вручную, по документации GitHub: 301 и 308 — постоянный перенос, запрос повторяется по `Location`, а замена запоминается до конца запуска; 302 и 307 — временный, по `Location` только этот запрос. Не больше 5 переходов подряд. Каждый переход — отдельная строка журнала. `Authorization` отправляется только на схему и хост из `GITHUB_API_BASE_URL`, так что токен не уходит на чужой сервер.

### Повторы и лимиты

| Ситуация | Поведение |
|---|---|
| сетевая ошибка, таймаут, 5xx | до 3 повторов с паузами 1, 2, 4 с, затем `request_failed` |
| 403 или 429 и (`rate limit` в теле, или есть `retry-after`, или `x-ratelimit-remaining: 0`) | пауза: `retry-after` (секунды или HTTP-дата) → иначе до `x-ratelimit-reset` + 1 с запаса → иначе 60 с; затем повтор |
| лимит не восстановился после 3 ожиданий | `rate_limited` |
| сервер просит ждать дольше 15 минут | сразу `rate_limited`, без ожидания |
| 403/429 без признаков лимита, другие 4xx | `request_failed` без повторов |
| 401 | работа прекращается: `{"error":"unauthorized"}`, код 3 |

Число ожиданий лимита и максимальная пауза в ТЗ не заданы; ограничения выбраны, чтобы программа не зависала на часы, если лимит исчерпан надолго. Счётчики повторов и ожиданий ведутся для каждого адреса отдельно.

### Кэш

Для каждого адреса, вернувшего 200 с `ETag`, в `--cache-dir` хранится файл `<sha256(url) в hex>.json`:

```json
{"url":"https://api.github.com/repos/cli/cli/issues?state=open&sort=created&direction=asc&per_page=100","etag":"W/\"4fdde5ff28d6ae3bd782e019eb6d8b882d7d85f7a2615a4b1fa5e1379f9fc8ed\"","link":"<https://api.github.com/repositories/212613049/issues?state=open&sort=created&direction=asc&per_page=100&after=...>; rel=\"next\"","body":"[{\"url\":\"https://api.github.com/repos/cli/cli/issues/136\", ...}]"}
```

- повторный запрос того же адреса отправляется с `If-None-Match: <etag>`;
- 304 → тело берётся из кэша, в журнале `from_cache: true`;
- 200 → запись перезаписывается (через временный файл и переименование), ответ без `ETag` удаляет устаревшую запись;
- `link` хранится потому, что GitHub отвечает 304 без заголовка `Link`, а без него нельзя перейти к следующей странице; если 304 пришёл с `Link`, используется он;
- 304 на запрос без `If-None-Match` — нарушение контракта, `invalid_response`;
- токен и заголовки запроса в кэш не пишутся; повреждённые файлы игнорируются.

### Журнал

`--trace` — одна строка JSON на каждую попытку HTTP-запроса, в порядке выполнения: `ts` (RFC 3339, UTC, начало попытки), `method`, `url` (полный, с параметрами), `status` (0, если ответа нет), `attempt` (с 1, отдельно для каждого адреса), `from_cache`, `error` (`null`, `timeout` или `network`). Заголовков и токена в журнале нет.

## Устройство

```
cmd/ghreport        точка входа: сигналы, os.Exit
internal/app        сценарий запуска: аргументы → обход репозиториев → отчёт и код возврата
internal/config     разбор флагов и переменных окружения
internal/github     HTTP-клиент (повторы, лимиты, редиректы, кэш, журнал), пагинация, разбор issue
internal/cache      файловый кэш ETag и тел ответов
internal/trace      журнал JSON Lines
internal/report     формат отчёта и сериализация
```

## Тесты

```sh
cd service
gofmt -l . && go vet ./... && go test -race ./...
```

Юнит-тесты работают на `httptest.Server` и не ходят в сеть. Паузы и текущее время подменяются, поэтому тесты повторов и лимитов не ждут реальных секунд. Покрыто: разбор аргументов, формат отчёта, заголовки запроса, токен только на базовый хост, повторы 1/2/4 с, таймауты и сетевые ошибки, все признаки rate limit и способы вычисления паузы, редиректы и их циклы, пагинация по `Link` (в том числе относительные адреса и циклы), `--limit`, `--since`, дедупликация, сортировка, кэш между запусками и 304 без `Link`, коды возврата, отсутствие токена в журнале, кэше и сообщениях.
