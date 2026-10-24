# vk-workspace-backend-assignment

Тестовое задание VK Tech: «Развитие платформы для ИИ-агентов в корпоративном мессенджере VK WorkSpace».

## Структура

| Каталог | Часть | Содержимое |
|---|---|---|
| [`msgfmt/`](msgfmt/) | 1 | форматтер сообщений на C++17: Markdown-подмножество → текст и диапазоны стилей |
| [`service/`](service/) | 2 | служба `ghreport` на Go: сводка по открытым issue репозиториев GitHub, Docker-образ |
| [`api_tests/`](api_tests/) | 3 | API-тесты службы на pytest |

## Окружение

- Linux, GCC или Clang с поддержкой C++17, CMake ≥ 3.20
- Go ≥ 1.22
- Python ≥ 3.10, pytest ≥ 7
- Docker ≥ 24
