# База данных

Документ описывает PostgreSQL-слой Go-сервиса: подключение, миграции, схему, форматы JSONB, владение данными и обслуживание. Требования и модель данных верхнего уровня — в [корневом README](../../README.md); сценарии авторизации — в [auth.md](auth.md).

## 1. Роль и границы

PostgreSQL хранит всё, что принадлежит пользователю: аккаунты, refresh-токены, историю подборов и личные списки. Python RAG к этой базе не обращается; его состояние (индекс, Qdrant, настройки) живёт отдельно. Go не хранит фильмы: карточки приходят из RAG и сохраняются только как снимки в JSONB.

Доступ к базе идёт через `pgxpool` и репозитории в [internal/infrastructure/persistence/postgres](../internal/infrastructure/persistence/postgres/). SQL не покидает этот пакет: сервисы видят только интерфейсы [ports](../internal/domain/ports/repositories.go).

## 2. Подключение и пул

| Параметр | Значение |
|---|---|
| Переменная | `DATABASE_URL`, обязательна |
| Формат | `postgres://user:password@host:port/db?sslmode=disable` |
| Пул | `PG_MAX_CONNS`, 1–100, по умолчанию 10 |
| Минимум соединений | 1 |
| Время жизни соединения | 30 минут |
| Простой | 5 минут |
| Проверка здоровья пула | раз в минуту |
| Ping при старте | 5 секунд, без повторов |

`NewPool` разбирает URL, настраивает пул и делает `Ping` с таймаутом. Если база не отвечает, сервис завершает старт: полуготовый процесс не должен принимать трафик. В Docker Compose это компенсируется healthcheck'ом PostgreSQL и зависимостью `api` от healthy-состояния; у самого `api` включён `restart: unless-stopped`.

Конфигурация описана в [config.go](../internal/config/config.go); переменные и значения по умолчанию перечислены в корневом README, раздел «Интерфейс и эксплуатация».

## 3. Миграции

SQL встроен в бинарник через `embed` ([migrations.go](../internal/infrastructure/persistence/postgres/migrations.go)). Файлы лежат в [migrations/](../internal/infrastructure/persistence/postgres/migrations/), имя — `NNNN_описание.up.sql` / `.down.sql`. Применяет их `golang-migrate`, версия и флаг `dirty` хранятся в таблице `schema_migrations`.

```bash
go run ./cmd/migrate up        # применить всё
go run ./cmd/migrate version   # версия и dirty
go run ./cmd/migrate down 1    # откатить один шаг
```

Правила:

- API-процесс миграции не применяет. В Compose для этого есть одноразовый сервис `migrate`, который завершается до старта `api`; локально команду запускают вручную.
- `/ready` проверяет, что схема не `dirty` и её версия совпадает с последней миграцией, встроенной в бинарник. Иначе 503 с `failed_checks: ["postgres"]`.
- `down` — операция для разработки: откат может удалить данные. В рабочей среде лучше новая миграция вперёд, а не откат.
- Миграции идемпотентны на уровне версии: повторный `up` при актуальной схеме возвращает `ErrNoChange` и не считается ошибкой.

## 4. Схема

Схема задана [первой миграцией](../internal/infrastructure/persistence/postgres/migrations/0001_init.up.sql) и [миграцией чатов](../internal/infrastructure/persistence/postgres/migrations/0002_chats.up.sql). Поля `user_id` и `chat_id` каждой подборки вместе ссылаются на чат того же владельца. Старые записи получают отдельный чат с тем же UUID.

### `users`

| Колонка | Тип | Ограничения |
|---|---|---|
| `id` | `uuid` | PK, `gen_random_uuid()` |
| `email` | `text` | NOT NULL |
| `password_hash` | `text` | NOT NULL, PHC-строка Argon2id |
| `created_at` | `timestamptz` | NOT NULL, `now()` |

Уникальность — функциональный индекс `users_email_lower_key ON users (lower(email))`. Приложение нормализует email (`trim` + `lower`) до записи, но индекс защищает и от прямых вставок. Нарушение уникальности превращается в `ErrEmailTaken` → 409 `EMAIL_TAKEN`.

### `refresh_tokens`

| Колонка | Тип | Ограничения |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | NOT NULL, FK → `users(id)` ON DELETE CASCADE |
| `token_hash` | `bytea` | NOT NULL, UNIQUE |
| `family_id` | `uuid` | NOT NULL |
| `expires_at` | `timestamptz` | NOT NULL |
| `revoked_at` | `timestamptz` | NULL |
| `created_at` | `timestamptz` | NOT NULL, `now()` |

Индексы: `refresh_tokens_user_id_idx`, `refresh_tokens_expires_at_idx`. В базе лежит только `sha256(refresh_token)`; сам токен существует только у клиента в cookie. `family_id` связывает цепочку ротаций: повторное использование старого токена отзывает всю семью. Подробности — в [auth.md](auth.md).

### `search_history`

| Колонка | Тип | Ограничения |
|---|---|---|
| `id` | `uuid` | PK |
| `user_id` | `uuid` | NOT NULL, FK → `users(id)` ON DELETE CASCADE |
| `query` | `text` | NOT NULL, `length(btrim(query)) > 0` |
| `filters` | `jsonb` | NOT NULL, объект |
| `requested_limit` | `integer` | NOT NULL, `> 0` |
| `response` | `jsonb` | NOT NULL, объект |
| `created_at` | `timestamptz` | NOT NULL, `now()` |

Индекс `search_history_user_created_idx ON (user_id, created_at DESC, id DESC)` обслуживает и подсчёт, и постраничную выдачу. `response` — полный снимок ответа RAG: повторное открытие истории не вызывает RAG. Запись создаётся только через `POST /recommendations`; обновления нет, есть только удаление владельцем.

### `user_movies`

| Колонка | Тип | Ограничения |
|---|---|---|
| `user_id` | `uuid` | FK → `users(id)` ON DELETE CASCADE |
| `movie_id` | `bigint` | NOT NULL |
| `status` | `text` | NOT NULL, `want_to_watch` или `watched` |
| `liked` | `boolean` | NULL, только при `watched` |
| `movie_snapshot` | `jsonb` | NOT NULL, объект |
| `updated_at` | `timestamptz` | NOT NULL, `now()` |

Первичный ключ — `(user_id, movie_id)`: один фильм в списке пользователя встречается один раз. Индекс `user_movies_user_status_updated_idx ON (user_id, status, updated_at DESC, movie_id)` обслуживает выборку по статусу.

Ограничения БД дублируют доменные правила:

- `status IN ('want_to_watch', 'watched')`;
- `liked IS NULL OR status = 'watched'` — оценка без просмотра невозможна;
- `movie_snapshot` — JSON-объект.

Снимок карточки записывается при первом `PUT` и дальше не обновляется: `ON CONFLICT ... DO UPDATE` меняет только `status`, `liked` и `updated_at`.

## 5. Форматы JSONB

JSONB — самостоятельный контракт хранения, а не HTTP DTO и не доменная модель. Домен не содержит JSON-тегов; преобразование выполняют [history_codec.go](../internal/infrastructure/persistence/postgres/history_codec.go) и [movie_codec.go](../internal/infrastructure/persistence/postgres/movie_codec.go). Поэтому HTTP-контракт можно менять, не переписывая строки, но изменение формата хранения требует отдельной миграции.

### Фильтры истории

```json
{
  "genre_ids": [878, 28],
  "year_from": 1990,
  "year_to": 2010,
  "min_rating": 7.0
}
```

`genre_ids` всегда массив; пустой список хранится как `[]`, а не `null`. Необязательные границы при отсутствии не пишутся (`omitempty`).

### Снимок ответа истории

```json
{
  "mode": "generated",
  "strategy_used": "hybrid",
  "items": [
    {
      "movie": {
        "movie_id": 603,
        "title": "The Matrix",
        "year": 1999,
        "genres": [{"id": 28, "name": "Action"}],
        "rating": 8.2,
        "vote_count": 24589,
        "overview": "A hacker discovers the truth."
      },
      "reason": "Подходит под запрос"
    }
  ],
  "warnings": [{"code": "FEWER_RESULTS", "message": "Найдено меньше фильмов, чем запрошено."}]
}
```

`items` и `warnings` всегда массивы; `reason`, `year`, `rating` опускаются, если значения нет. Тот же формат карточки используется в `movie_snapshot` личных списков.

## 6. Владение и изоляция

Все запросы к пользовательским данным содержат `user_id` из access-токена:

- история читается и удаляется только по паре `(id, user_id)`;
- списки — по `(user_id, movie_id)`;
- чужой или отсутствующий объект неотличимы: оба дают `ErrNotFound` → 404;
- каскадное удаление пользователя удаляет его токены, историю и списки; отдельного API удаления аккаунта пока нет.

Репозитории не принимают фильтры, собранные из тела запроса: идентификатор пользователя приходит только из проверенного токена. Это осознанно исключает доступ к чужим строкам через подмену параметра.

## 7. Порядок и пагинация

| Выборка | Сортировка |
|---|---|
| История | `created_at DESC, id DESC` |
| Личные списки | `updated_at DESC, movie_id` |

Обе выдачи считают `count(*)` по тому же фильтру и отдают страницу через `LIMIT`/`OFFSET`. Ограничения страницы (1–100, по умолчанию 20) задаёт HTTP-слой; репозиторий получает уже проверенные `limit` и `offset`. Смещение считается как `(page - 1) * page_size`.

## 8. Транзакции и конкурентность

Явных транзакций (`BEGIN`/`COMMIT`) в текущем коде нет: каждый метод репозитория выполняет один SQL-запрос, а сервисы комбинируют вызовы. Практические следствия:

- вставка истории и списка атомарна на уровне строки; `user_movies` использует `INSERT ... ON CONFLICT`, поэтому параллельные `PUT` не создают дубль;
- ротация refresh-токена — последовательность «условно отозвать старый → создать новый». Условный `UPDATE ... WHERE revoked_at IS NULL` не даёт двум запросам ротировать один токен; проигравший отзывает семью. Если процесс упадёт между отзывом и созданием нового токена, сессия потеряется — пользователь войдёт заново;
- `RevokeFamily` и `DeleteExpired` — отдельные запросы; частичного отката нет.

Для сценариев, где нужна атомарность нескольких записей, транзакцию придётся добавить явно; сейчас такие сценарии отсутствуют.

## 9. Очистка refresh-токенов

Фоновая задача [cleanup/refresh_tokens.go](../internal/infrastructure/services/cleanup/refresh_tokens.go) запускается при старте и затем раз в 15 минут. Она удаляет строки, у которых `expires_at < now - 7 дней` **или** `revoked_at < now - 7 дней`.

Семидневный запас нужен для обнаружения повторного использования: если удалить отозванный токен сразу, повторная попытка выглядела бы как «неизвестный токен», а не как кража семьи. История и личные списки автоматически не чистятся.

## 10. Эксплуатация

- **Готовность.** `/ready` проверяет Ping и версию схемы; RAG в проверку не входит. Пока миграции не применены, `/ready` отвечает 503.
- **Хранилище.** В Compose данные лежат в volume `pgdata`; порт PostgreSQL наружу не публикуется. Удаление volume (`docker compose down -v`) стирает аккаунты и историю.
- **Резервные копии.** Штатного механизма в сервисе нет: нужен `pg_dump`/снапшоты средствами PostgreSQL. При восстановлении проверяйте `schema_migrations`, иначе `/ready` останется 503.
- **Индексы.** Отдельных индексов по `filters`/`response` нет; JSONB используется как снимок, а не как поисковый индекс.
- **Масштабирование.** Несколько экземпляров API могут работать с одной базой; блокировок на уровне приложения нет. Реплики чтения не настроены.

## 11. Ограничения

- Нет транзакций, объединяющих несколько операций сервиса; см. раздел 8.
- История и личные списки растут без TTL и без архивирования.
- Снимки карточек и истории неизменяемы: чтобы получить свежие данные, запись нужно удалить и создать заново.
- Нет API удаления аккаунта, экспорта данных и мягкого удаления.
- Нет полнотекстового поиска по `query`; индекс истории оптимизирован под хронологию.
- Откат миграций destructive и предназначен для разработки.
