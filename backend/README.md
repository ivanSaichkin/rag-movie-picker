# Go-бэкенд

HTTP-сервис RAG Movie Picker: аккаунты, история поиска и личные списки в PostgreSQL, вызовы Python RAG по gRPC. Требования, архитектура, HTTP API и модель данных описаны в [корневом README](../README.md) — он остаётся единственным источником истины. Этот файл описывает только разработку сервиса.

## Текущее состояние

Реализованы HTTP-каркас и слой PostgreSQL:

- конфигурация из окружения, JSON-логи, `request_id`, единый конверт ошибок, `/health` и `/ready`;
- graceful shutdown, миграции `users`, `refresh_tokens`, `search_history`, `user_movies`;
- репозитории аккаунтов и refresh-токенов, фоновая чистка просроченных токенов;
- проверка версии схемы и подключения к БД в `/ready`.

Авторизация и вызовы RAG ещё не подключены: HTTP API ограничен `/health` и `/ready`.

## Структура

```text
backend/
├── cmd/
│   ├── api/                    # HTTP-сервер
│   └── migrate/                # up | down <steps> | version
├── internal/
│   ├── app/                    # точка сборки зависимостей и жизненный цикл
│   ├── config/                 # env → Config, валидация
│   ├── domain/
│   │   ├── models/             # User, RefreshToken
│   │   ├── rules/              # доменные ошибки
│   │   └── ports/              # контракты репозиториев и readiness
│   ├── infrastructure/
│   │   ├── adapters/           # logger, router
│   │   ├── persistence/postgres/  # пул, миграции, репозитории, readiness
│   │   └── services/           # cleanup
│   └── interfaces/http/
│       ├── dto/                # JSON-контракты ошибок и здоровья
│       ├── handlers/           # health
│       └── server/             # http.Server и middleware
├── pkg/testutils/              # httptest-хелперы для тестов
├── Makefile
└── .golangci.yml
```

Модуль — `github.com/ivanSaichkin/rag-movie-picker/backend`.

## Запуск

```bash
cd backend
export DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/rag_movie_picker?sslmode=disable'

go run ./cmd/migrate up        # применить миграции, SQL встроен в бинарник
go run ./cmd/migrate version   # версия схемы и флаг dirty
go run ./cmd/migrate down 1    # откатить одну миграцию
make run                       # go run ./cmd/api

curl -i 127.0.0.1:8080/health
curl -i 127.0.0.1:8080/ready
```

Обязательная переменная — `DATABASE_URL`. Полный список настроек и их значения по умолчанию приведены в корневом README, раздел «Интерфейс и эксплуатация». `/ready` возвращает 503, если база недоступна, миграции не применены или схема помечена как dirty.

## Проверки

```bash
make check              # go fmt, go vet, golangci-lint, тесты с -race
make test-integration   # PostgreSQL в testcontainers, нужен Docker
```

`make check` использует `golangci-lint` из `PATH`; нужна версия 2.15 или новее, собранная с Go 1.26+, иначе линтер откажется работать с `go 1.26.0` в `go.mod`. Версию можно передать явно: `make check GOLANGCI_LINT=/путь/к/golangci-lint`.

Интеграционные тесты помечены тегом `integration` и не входят в обычный `go test ./...`; им нужен Docker.
