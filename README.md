# vk-workspace-backend-assignment

Тестовое задание VK Tech: «Развитие платформы для ИИ-агентов в корпоративном мессенджере VK WorkSpace».

| Каталог | Часть | Содержимое |
|---|---|---|
| [`msgfmt/`](msgfmt/) | 1 | форматтер сообщений на C++17: подмножество Markdown → текст и диапазоны стилей в кодовых единицах UTF-16 |
| [`service/`](service/) | 2 | служба `ghreport` на Go: сводка по открытым issue репозиториев GitHub, Docker-образ |
| [`api_tests/`](api_tests/) | 3 | API-тесты службы на pytest по принципу чёрного ящика |

Подробности — в README каждой части.

## Окружение

Linux, GCC или Clang с C++17, CMake ≥ 3.20, Go ≥ 1.22, Python ≥ 3.10 и pytest ≥ 7, Docker ≥ 24. Во всех частях только стандартные библиотеки (кроме pytest): без клиентских SDK GitHub и готовых парсеров Markdown.

## Запуск

Все команды — из корня репозитория.

**Часть 1 — msgfmt**

```sh
cmake -S msgfmt -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
ctest --test-dir build
printf '**жирный _и_ курсив**' | build/msgfmt
printf '2 * 3' | build/msgfmt escape
```

Проверка под санитайзерами: добавить `-DMSGFMT_SANITIZE=ON` (и `-DMSGFMT_WERROR=ON`, чтобы предупреждения считались ошибками).

**Часть 2 — ghreport**

```sh
cd service && gofmt -l . && go vet ./... && go test ./... && cd ..
docker build -t ghreport service/
docker run --rm -e GITHUB_TOKEN=... ghreport --repos cli/cli --limit 20 --out -
```

**Часть 3 — api_tests**

```sh
(cd service && go build -o /tmp/ghreport ./cmd/ghreport)
python3 -m venv .venv && .venv/bin/pip install -r api_tests/requirements.txt
GHREPORT_BIN=/tmp/ghreport .venv/bin/pytest api_tests -q
# вместе с контрактным сервером:
GHREPORT_BIN=/tmp/ghreport GITHUB_API_BASE_URL=https://api.github.com GITHUB_TOKEN=... \
  .venv/bin/pytest api_tests -q
```

## Основные решения

**msgfmt** ([подробно](msgfmt/README.md#принятые-решения))
- Рекурсивный спуск за один проход: на открывающем маркере ищется ближайший подходящий закрывающий внутри текущего сегмента, содержимое разбирается в его границах — частичных пересечений диапазонов не бывает по построению.
- Результаты поиска закрывающих маркеров запоминаются, поэтому разбор линеен даже на входах вида `_a _a _a …`.
- «Буква или цифра» — ASCII и основные алфавиты Unicode (кириллица, латиница с диакритикой, греческий, CJK и др.), «пробельный» — все символы Unicode `White_Space`.
- Свой мини-каркас тестов: 15 примеров из ТЗ, краевые случаи, property-тест на 20 000 случайных входов, тест линейности; проверено GCC 13 и Clang 18 в Linux, в том числе с `-fsanitize=address,undefined`.

**ghreport** ([подробно](service/README.md#как-это-работает))
- Редиректы обрабатываются вручную: каждая попытка попадает в журнал, `Authorization` отправляется только на хост `GITHUB_API_BASE_URL`.
- Повторы при сетевых ошибках и 5xx — 1, 2, 4 с; при исчерпании лимита — пауза по `retry-after` → `x-ratelimit-reset` → 60 с, не больше 3 ожиданий и не дольше 15 минут (в ТЗ не задано), затем `rate_limited`.
- 401 в любой момент сразу завершает работу с `{"error":"unauthorized"}` и кодом 3.
- Кэш: файл `<sha256(url)>.json` с `url`, `etag`, `link`, `body`. `Link` хранится, потому что GitHub отвечает 304 без него — без этого повторный запуск останавливался на первой странице (найдено при проверке на реальном API).
- Образ: `distroless/static:nonroot`, ≈ 3 МБ, без секретов.

**api_tests** ([подробно](api_tests/README.md#как-устроен-набор))
- Тесты не доверяют ни серверу, ни службе: сами обходят `GITHUB_API_BASE_URL` по `Link`, проверяют контракт сервера и сверяют отчёт с его данными. Сценарии 429, 401, 5xx, обрывы соединения — на своём мок-сервере.
- Самопроверка: набор падает на каждой из 6 поломок сервера и 21 внесённой ошибки службы (таблица в README части).

## Затраченное время

2 часа.

## ИИ-инструменты

- Claude Code (Anthropic).
