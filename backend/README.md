# Go-бэкенд

HTTP-сервис RAG Movie Picker: аккаунты, авторизация по JWT, история поиска и личные списки в PostgreSQL, вызовы Python RAG по gRPC. Требования, архитектура, HTTP API и модель данных описаны в [корневом README](../README.md) — он остаётся единственным источником истины. Этот файл описывает только разработку сервиса.

## Текущее состояние

Реализованы каркас сервиса, слой PostgreSQL и авторизация:

- конфигурация из окружения, JSON-логи, `request_id`, единый конверт ошибок, `/health` и `/ready`;
- graceful shutdown, миграции `users`, `refresh_tokens`, `search_history`, `user_movies` и репозитории аккаунтов и refresh-токенов;
- Argon2id, access/refresh JWT, ротация refresh-токенов с отзывом семьи, проверка `Origin`, ограничение частоты через Redis или память;
- gRPC-клиент RAG с дедлайном и `request_id` в metadata, кеш каталога на 5 минут, `GET /api/v1/catalog` и `GET /api/v1/movies/{movie_id}`;
- подбор `POST /api/v1/recommendations` с валидацией фильтров по каталогу и снимком результата в `search_history`, история `GET/DELETE /api/v1/history`;
- личные списки `GET/PUT/DELETE /api/v1/my/movies` со снимком карточки из RAG на первой записи.

Каталог, карточки, подбор и первые записи в личные списки обращаются к Python RAG: при недоступном сервисе такие ручки отвечают 503 (каталог может отдаться из кеша), а вход продолжит работать. Фронтенд к API ещё не подключён.

Как устроена авторизация, что и где хранится — в [docs/auth.md](docs/auth.md).

## Структура

```text
backend/
├── cmd/
│   ├── api/                    # HTTP-сервер
│   └── migrate/                # up | down <steps> | version
├── internal/
│   ├── app/                    # точка сборки зависимостей и жизненный цикл
│   ├── config/                 # env → Config, валидация
│   ├── platform/               # сквозные помощники (request id)
│   ├── domain/
│   │   ├── models/             # User, RefreshToken, Session
│   │   ├── rules/              # доменные ошибки и валидация
│   │   └── ports/              # контракты репозиториев, сервисов, кеша
│   ├── infrastructure/
│   │   ├── adapters/           # logger, router, password, token, cache
│   │   ├── clients/            # gRPC-клиент RAG
│   │   ├── persistence/postgres/  # пул, миграции, репозитории, readiness
│   │   └── services/           # auth, catalog, cleanup, history, recommendation, usermovies
│   └── interfaces/http/
│       ├── dto/                # JSON-контракты
│       ├── handlers/           # health, auth, catalog, recommendation, history, user_movie
│       └── server/             # http.Server и middleware
├── pkg/testutils/              # httptest-хелперы для тестов
├── docs/                       # детальная документация сервиса
├── Makefile
└── .golangci.yml
```

Модуль — `github.com/ivanSaichkin/rag-movie-picker/backend`. Контракт gRPC лежит отдельным модулем в `../pkg/ragapi` и подключается на этапе интеграции с RAG.

## Запуск

```bash
cd backend
export DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/rag_movie_picker?sslmode=disable'
export JWT_SECRET='dev-secret-that-is-at-least-32-chars'

go run ./cmd/migrate up        # применить миграции, SQL встроен в бинарник
go run ./cmd/migrate version   # версия схемы и флаг dirty
go run ./cmd/migrate down 1    # откатить одну миграцию
make run                       # go run ./cmd/api, без REDIS_ADDR кеш в памяти

curl -i 127.0.0.1:8080/health
curl -i 127.0.0.1:8080/ready
```

Обязательные переменные — `DATABASE_URL` и `JWT_SECRET` (не короче 32 символов). Полный список настроек и их значения по умолчанию приведены в корневом README, раздел «Интерфейс и эксплуатация». Redis необязателен: без `REDIS_ADDR` ограничения частоты считаются в памяти, при недоступном Redis сервис откатывается на память и пишет предупреждение в лог с троттлингом.

## Проверки

```bash
make check              # go fmt, go vet, golangci-lint, тесты с -race
make test-integration   # PostgreSQL в testcontainers, нужен Docker
```

`make check` использует `golangci-lint` из `PATH`; нужна версия 2.15 или новее, собранная с Go 1.26+, иначе линтер откажется работать с `go 1.26.0` в `go.mod`. Версию можно передать явно: `make check GOLANGCI_LINT=/путь/к/golangci-lint`.

Интеграционные тесты помечены тегом `integration` и не входят в обычный `go test ./...`; им нужен Docker. Обычные unit-тесты не требуют БД, сети и модели.

## Ограничения текущего этапа

- RAG-клиент, каталог и подбор фильмов не реализованы — `/api/v1` содержит только `/auth/*`.
- Отзыв access-токена при выходе не мгновенный: токен живёт до истечения `ACCESS_TOKEN_TTL` (15 минут по умолчанию), отзывается только refresh.
- Ограничение частоты считается на экземпляр приложения; общий счётчик включается только при настроенном Redis.
