# Авторизация и хранение данных

Документ описывает реализацию авторизации в Go-сервисе: сценарии, формат токенов, where-хранение и принятые ограничения. Требования, HTTP API и модель данных — в [корневом README](../../README.md); этот файл их не переопределяет.

## 1. Обзор

| Endpoint | Доступ | Успех | Тело ответа |
|---|---|---|---|
| `POST /api/v1/auth/register` | без токена | 201 | `user`, `access_token`, `expires_in` |
| `POST /api/v1/auth/login` | без токена | 200 | то же |
| `POST /api/v1/auth/refresh` | refresh-cookie | 200 | `access_token`, `expires_in` |
| `POST /api/v1/auth/logout` | refresh-cookie | 204 | — |
| `GET /api/v1/auth/me` | access-токен | 200 | `id`, `email`, `created_at` |

Состав:

- **access-токен** — JWT HS256, живёт `ACCESS_TOKEN_TTL` (по умолчанию 15 минут), передаётся заголовком `Authorization: Bearer`;
- **refresh-токен** — 32 случайных байта в `HttpOnly`-cookie, живёт `REFRESH_TOKEN_TTL` (по умолчанию 30 дней), ротируется при каждом использовании;
- **пароли** — Argon2id в PHC-формате;
- **лимиты** — фиксированное окно на IP через Redis или память.

## 2. Сценарии

### 2.1. Регистрация

Запрос проходит цепочку `RequestID → Logging → Recovery → CheckOrigin → лимитер register` и попадает в [AuthHandler.Register](../internal/interfaces/http/handlers/auth.go), затем в [AuthenticationService.Register](../internal/infrastructure/services/auth/service.go).

1. JSON декодируется с запретом неизвестных полей (`binding.EnableDecoderDisallowUnknownFields` в [router.go](../internal/infrastructure/adapters/router/router.go)); мусор или лишнее поле → 422 `VALIDATION_ERROR`.
2. Email нормализуется (`trim` + `lower`) и проверяется: один `@`, непустые части, точка в домене, не длиннее 254 символов, без пробелов и крайних точек ([validation.go](../internal/domain/rules/validation.go)). Пароль — 8–128 символов.
3. Пароль хешируется Argon2id ([argon2id.go](../internal/infrastructure/adapters/password/argon2id.go)): соль 16 байт из `crypto/rand`, ключ 32 байта, параметры из конфига; результат — `$argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>`.
4. `INSERT INTO users ... RETURNING id, created_at` ([users.go](../internal/infrastructure/persistence/postgres/users.go)). Нарушение уникального индекса по `lower(email)` (SQLSTATE 23505) превращается в `ErrEmailTaken` → **409 `EMAIL_TAKEN`**.
5. Создаётся сессия ([newSession](../internal/infrastructure/services/auth/service.go)):
   - refresh-токен — 32 случайных байта, `base64.RawURLEncoding`;
   - в БД уходит только `sha256(token)` (32 байта `bytea`), `family_id = uuid.New()`, `expires_at = now + REFRESH_TOKEN_TTL`;
   - access-токен — JWT HS256 с `sub` (UUID пользователя), `iss: rag-movie-picker`, `typ: access`, `iat`, `exp = now + ACCESS_TOKEN_TTL`.
6. Ответ 201: access-токен в теле, refresh — в `Set-Cookie`.

### 2.2. Вход

`POST /login` (лимит 10 в минуту на IP):

1. Email нормализуется; пустые email или пароль → 422. Полная проверка формата email выполняется только при регистрации.
2. `users.FindByEmail` ищет по `lower(email)`.
3. Если пользователя нет — всё равно вызывается `Verify(password, dummyHash)`. `dummyHash` считается один раз при старте сервиса, чтобы время ответа не выдавало существование аккаунта. Наружу — **401 `INVALID_CREDENTIALS`** с одинаковым текстом.
4. Если пользователь есть — Argon2id-проверка с `subtle.ConstantTimeCompare`; параметры берутся из самой PHC-строки, поэтому смена `ARGON2_*` не ломает старые хеши и влияет только на новые.
5. Успех — новая сессия: новая семья refresh-токенов, новый access.

### 2.3. Использование access-токена

Защищённые ручки требуют `Authorization: Bearer <jwt>`. [middleware.Authenticate](../internal/interfaces/http/server/middleware/auth.go):

- проверяет подпись **только HS256**, `iss`, срок `exp`, claim `typ=access`, `sub` парсится как UUID;
- истёкший токен → **401 `TOKEN_EXPIRED`** (фронт по этому коду идёт в `/refresh`);
- любой другой сбой → **401 `UNAUTHENTICATED`**;
- кладёт UUID пользователя в gin-контекст; хендлеры читают его через `middleware.UserIDFrom`.

JWT stateless: ролей, `jti` и `aud` нет; сервер access-токены не хранит.

### 2.4. Refresh и ротация

`POST /refresh` читает cookie `refresh_token`; без cookie → 401 `UNAUTHENTICATED`.

1. `sha256(cookie)` → `refresh_tokens.FindByHash`.
2. Не найден → 401, причина не раскрывается.
3. `revoked_at IS NOT NULL` → **reuse detection**: токен украли, `RevokeFamily(family_id)` отзывает все токены семьи, ответ 401.
4. Истёк (`expires_at <= now`) → 401, ротации нет.
5. Атомарный отзыв: `UPDATE ... SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`. Если обновлено 0 строк (параллельный refresh тем же токеном) → `RevokeFamily` и 401.
6. Пользователь ещё существует? Если нет → 401. Иначе новый refresh выпускается **в той же семье**, новый access — в теле, новый refresh — в cookie.

Итог: refresh-токен одноразовый. Повторное использование старого отзывает всю ветку.

### 2.5. Выход

`POST /logout` идемпотентен: cookie не обязательна, найденный токен отзывается, cookie удаляется (`Max-Age=-1`), ответ 204. Access-токен не отзывается и доживает до `exp`. Денайлиста доступа нет осознанно: при коротком TTL это дешевле, чем хранить состояние каждого токена.

### 2.6. `/auth/me`

Требует access-токен: UUID берётся из контекста, возвращается пользователь. Если пользователь удалён → 401, cookie сессии очищается.

### 2.7. Cookie

Одна cookie, [cookie.go](../internal/interfaces/http/handlers/cookie.go):

| Параметр | Значение |
|---|---|
| имя | `refresh_token` |
| `Path` | `/api/v1/auth` — на остальные ручки не отправляется |
| `HttpOnly` | всегда |
| `SameSite` | `Strict` |
| `Secure` | из `COOKIE_SECURE`; локально по HTTP — `false` |
| `Max-Age` / `Expires` | остаток до `RefreshTokenExpiresAt` |

Access-токен в cookie не кладётся: клиент держит его в памяти и отправляет заголовком.

### 2.8. Защита от CSRF

Отдельного CSRF-токена нет: access-токен не отправляется автоматически, а refresh-cookie прикрыта `SameSite=Strict` и проверкой `Origin` в [middleware.CheckOrigin](../internal/interfaces/http/server/middleware/origin.go). Для методов кроме GET/HEAD/OPTIONS при наличии заголовка `Origin` он должен совпадать с `ALLOWED_ORIGIN`, иначе 403 `FORBIDDEN`. Запросы без `Origin` (curl, мобильные клиенты) пропускаются.

### 2.9. Ограничение частоты

[RateLimit](../internal/interfaces/http/server/middleware/ratelimit.go) — фиксированное окно на IP:

| Ручка | Ключ | Лимит | Окно |
|---|---|---|---|
| login | `ratelimit:login:<ip>:<unix начала окна>` | `RATE_LIMIT_AUTH_PER_MINUTE` (10) | минута |
| register | `ratelimit:register:<ip>:<unix>` | `RATE_LIMIT_REGISTER_PER_HOUR` (20) | час |
| refresh | `ratelimit:refresh:<ip>:<unix>` | `RATE_LIMIT_REFRESH_PER_MINUTE` (60) | минута |

Счётчик инкрементируется через `Cache.Increment` с TTL окна. Превышение → 429 `RATE_LIMITED` + `Retry-After` (секунды до конца окна + 1). Если хранилище сломалось — fail-open: запрос пропускается, в лог пишется warning.

## 3. Где что хранится

### PostgreSQL

Миграция [0001_init.up.sql](../internal/infrastructure/persistence/postgres/migrations/0001_init.up.sql).

**`users`**

| Колонка | Тип | Смысл |
|---|---|---|
| `id` | `uuid PK DEFAULT gen_random_uuid()` | идентификатор |
| `email` | `text NOT NULL` | нормализованный (lower + trim) |
| `password_hash` | `text NOT NULL` | PHC-строка Argon2id, не пароль |
| `created_at` | `timestamptz DEFAULT now()` | |

Уникальность — индекс `users_email_lower_key` по `lower(email)`: дубль ловится и при гонке.

**`refresh_tokens`**

| Колонка | Тип | Смысл |
|---|---|---|
| `id` | `uuid PK` | |
| `user_id` | `uuid FK → users ON DELETE CASCADE` | владелец; удаление пользователя чистит токены |
| `token_hash` | `bytea NOT NULL UNIQUE` | `sha256` значения из cookie; сам токен не хранится |
| `family_id` | `uuid` | цепочка ротаций; отзыв семьи |
| `expires_at`, `created_at` | `timestamptz` | срок и время создания |
| `revoked_at` | `timestamptz NULL` | момент отзыва; идемпотентность через `IS NULL` |

Индексы: `user_id`, `expires_at`.

**Готовы, но пока без репозиториев:** `search_history` (JSONB `filters`/`response`, CHECK на объекты, непустой запрос, лимит > 0) и `user_movies` (PK `user_id + movie_id`, CHECK статуса, `liked` только для `watched`, JSONB-снимок). Доступ появится в следующих частях.

**Служебная** `schema_migrations` — версия миграций golang-migrate; её читает `/ready` и при расхождении с версией в бинарнике или флаге `dirty` отдаёт 503.

### Redis

- Необязателен: пустой `REDIS_ADDR` → сразу in-memory.
- Один `DB` (по умолчанию 0); ключи `ratelimit:login|register|refresh:<ip>:<windowStartUnix>`, TTL = окно.
- Инкремент атомарный: Lua-скрипт делает `INCR` и `PEXPIRE` только при создании ключа, окно не «плывёт» ([redis.go](../internal/infrastructure/adapters/cache/redis.go)).
- Персистентность не нужна: при потере ключей счётчики начнутся заново.
- При ошибке Redis [FallbackCache](../internal/infrastructure/adapters/cache/fallback.go) уходит в память и логирует warning не чаще раза в минуту.

### Память процесса

- `InMemoryCache` — тот же `ports.Cache`, если Redis не настроен или упал: `map` + TTL + периодическая чистка.
- `dummyHash` — один PHC-хеш, посчитанный при старте для выравнивания времени входа.
- Байты `JWT_SECRET` и параметры конфига — только из окружения.

### Cookie и клиент

- refresh-токен — единственное значение в cookie; у клиента оригинал, в БД — только хеш;
- access-токен — в теле ответа; клиент хранит его в памяти, на сервере его нет.

### Окружение

| Переменная | По умолчанию | Роль |
|---|---|---|
| `JWT_SECRET` | обязательна, ≥32 символов | ключ HS256 |
| `ACCESS_TOKEN_TTL` | 900 с | жизнь access |
| `REFRESH_TOKEN_TTL` | 2592000 с (30 дней) | жизнь refresh |
| `COOKIE_SECURE` | `false` | флаг `Secure` у cookie |
| `ALLOWED_ORIGIN` | `http://127.0.0.1:8080` | проверка Origin |
| `ARGON2_ITERATIONS` / `ARGON2_MEMORY_MIB` / `ARGON2_PARALLELISM` | 3 / 64 / 2 | стоимость хеша |
| `RATE_LIMIT_AUTH_PER_MINUTE` / `RATE_LIMIT_REGISTER_PER_HOUR` / `RATE_LIMIT_REFRESH_PER_MINUTE` | 10 / 20 / 60 | лимиты |
| `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB` / `REDIS_TIMEOUT_SECONDS` | пусто / — / 0 / 1 | необязательный кеш |

## 4. Код по слоям

| Слой | Файлы |
|---|---|
| контракты | [ports/services.go](../internal/domain/ports/services.go), [ports/repositories.go](../internal/domain/ports/repositories.go) |
| правила и ошибки | [rules/validation.go](../internal/domain/rules/validation.go), [rules/errors.go](../internal/domain/rules/errors.go) |
| хеширование | [password/argon2id.go](../internal/infrastructure/adapters/password/argon2id.go) |
| JWT | [token/jwt.go](../internal/infrastructure/adapters/token/jwt.go) |
| кеш | [cache/memory.go](../internal/infrastructure/adapters/cache/memory.go), [cache/redis.go](../internal/infrastructure/adapters/cache/redis.go), [cache/fallback.go](../internal/infrastructure/adapters/cache/fallback.go) |
| сценарии | [services/auth/service.go](../internal/infrastructure/services/auth/service.go) |
| SQL | [persistence/postgres/users.go](../internal/infrastructure/persistence/postgres/users.go), [refresh_tokens.go](../internal/infrastructure/persistence/postgres/refresh_tokens.go) |
| HTTP | [handlers/auth.go](../internal/interfaces/http/handlers/auth.go), [handlers/cookie.go](../internal/interfaces/http/handlers/cookie.go), [handlers/request.go](../internal/interfaces/http/handlers/request.go) |
| middleware | [auth.go](../internal/interfaces/http/server/middleware/auth.go), [origin.go](../internal/interfaces/http/server/middleware/origin.go), [ratelimit.go](../internal/interfaces/http/server/middleware/ratelimit.go) |
| сборка | [app/](../internal/app) (фабрики ресурсов, сервисов и HTTP), [router.go](../internal/infrastructure/adapters/router/router.go), [config.go](../internal/config/config.go) |
| фоновая задача | [cleanup/refresh_tokens.go](../internal/infrastructure/services/cleanup/refresh_tokens.go) — каждые 15 минут удаляет токены, просроченные или отозванные более 7 дней назад |

## 5. Ошибки API

Тела: `{"error":{"code":"...","message":"..."}}`.

| Ситуация | HTTP | Код |
|---|---|---|
| нет или битый access-токен | 401 | `UNAUTHENTICATED` |
| access истёк | 401 | `TOKEN_EXPIRED` |
| неверная пара email/пароль | 401 | `INVALID_CREDENTIALS` |
| refresh недействителен (нет, отозван, истёк, reuse) | 401 | `UNAUTHENTICATED` + очистка cookie |
| email занят | 409 | `EMAIL_TAKEN` |
| невалидные поля, лишние JSON-поля | 422 | `VALIDATION_ERROR` |
| чужой Origin | 403 | `FORBIDDEN` |
| превышен лимит | 429 | `RATE_LIMITED` + `Retry-After` |
| прочие сбои | 500 | `INTERNAL` (детали только в логе) |

## 6. Пример потока

```text
POST /api/v1/auth/register   {"email":"User@Example.com","password":"password123"}
  → 201, body: user + access_token; Set-Cookie: refresh_token=...; HttpOnly; SameSite=Strict
GET  /api/v1/auth/me         Authorization: Bearer <access>
  → 200
POST /api/v1/auth/refresh    Cookie: refresh_token=<старый>
  → 200, новый access + Set-Cookie: refresh_token=<новый>
POST /api/v1/auth/refresh    Cookie: refresh_token=<старый повторно>
  → 401, вся семья отозвана
POST /api/v1/auth/logout     Cookie: refresh_token=<текущий>
  → 204, cookie очищена
```

## 7. Проверки

- Unit-тесты: правила email/пароля, Argon2id (соль, чужой пароль, битые PHC-строки), JWT (истёк, чужой секрет, мусор), кеш memory/redis (miniredis)/fallback с троттлингом логов, сервис на фейках (регистрация, вход с dummy-хешем, ротация, reuse, конкурентный refresh, logout, текущий пользователь), хендлеры (201/200/401/403/409/422/429, флаги cookie), middleware, роутер.
- Интеграционно (testcontainers): register → login → me → refresh → reuse → logout на реальном PostgreSQL.
- Смоук на живых PostgreSQL 16 и Redis 7: cookie-флаги, ротация, отзыв семьи, 429 с `Retry-After`, 403 на чужой Origin, ключи `ratelimit:*` в Redis, `/ready` 200 и graceful shutdown.

## 8. Ограничения

- Access-токен нельзя отозвать до `exp`: logout отзывает только refresh, поэтому TTL короткий.
- Нет подтверждения email, восстановления пароля и «выйти на всех устройствах».
- Один ключ HS256, без ротации ключей и `kid`.
- Лимиты считаются на инстанс, если Redis не настроен; при отказе кеша — fail-open.
- `gin.ClientIP()` по умолчанию доверяет прокси: за обратным прокси нужно настроить trusted proxies, иначе `X-Forwarded-For` можно подделать и обойти лимит.
- Отозванные refresh-токены хранятся ещё 7 дней — столько времени работает детект повторного использования.
