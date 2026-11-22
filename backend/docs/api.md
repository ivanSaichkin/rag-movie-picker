# HTTP API

Документ описывает HTTP-контракт Go-сервиса: общие свойства, формат ошибок, ручки, пагинацию, идемпотентность и ограничения. Сценарии авторизации — в [auth.md](auth.md), хранение — в [database.md](database.md), кеш — в [cache.md](cache.md), лимиты — в [rate-limiting.md](rate-limiting.md). Требования верхнего уровня — в [корневом README](../../README.md).

## 1. Общие свойства

| Свойство | Значение |
|---|---|
| Базовый префикс | `/api/v1`; `/health` и `/ready` вне версии |
| Формат | JSON UTF-8, поля `snake_case` |
| Контракт | Закрытый: неизвестные поля в теле → 422 |
| Content-Type | `application/json` для тел запросов |
| Слэш в конце | Не редиректится: `/catalog/` → 404 |
| Неизвестный метод | 405 `METHOD_NOT_ALLOWED` |
| Версионирование | Только в пути; заголовков версии нет |
| OpenAPI | Нет; источник контракта — этот документ, DTO и тесты |

Редиректы отключены намеренно: опечатка в пути должна давать явную ошибку, а не 301/307.

### Аутентификация

Все ручки `/api/v1` требуют access-токен в `Authorization: Bearer <jwt>`, кроме:

- `POST /auth/register`, `POST /auth/login` — без токена;
- `POST /auth/refresh` — по refresh-cookie;
- `POST /auth/logout` — по refresh-cookie, без access-токена.

Refresh-токен передаётся только в cookie:

| Атрибут | Значение |
|---|---|
| Имя | `refresh_token` |
| Path | `/api/v1/auth` |
| HttpOnly | да |
| SameSite | `Strict` |
| Secure | `COOKIE_SECURE`, по умолчанию `false` для локального HTTP |

Ошибки токена: 401 `UNAUTHENTICATED` (нет/неверный токен) и 401 `TOKEN_EXPIRED` (истёк access).

### Заголовки

| Заголовок | Направление | Смысл |
|---|---|---|
| `Authorization` | запрос | `Bearer <access>` |
| `Content-Type` | запрос | `application/json` при наличии тела |
| `Origin` | запрос | Проверяется на POST/PUT/DELETE; при наличии должен совпасть с `ALLOWED_ORIGIN` |
| `X-Request-ID` | запрос/ответ | Корреляция; допустимы `[A-Za-z0-9._:-]{1,64}`, иначе генерируется новый |
| `Set-Cookie` | ответ | Refresh-токен только на auth-ручках |
| `Retry-After` | ответ | Только при 429: секунды до следующего окна |

`X-Request-ID` всегда возвращается и попадает в структурные логи. Запросы без `Origin` (curl, мобильные клиенты) разрешены: access-токен не отправляется автоматически, поэтому CSRF к ним не применяется. Запрос с чужим `Origin` получает 403 `FORBIDDEN`.

CORS-заголовков сервис не отдаёт. Сайт должен работать через один публичный origin (прокси фронтенда) либо потребуется отдельно добавить CORS и `SameSite=None; Secure` — см. [auth.md](auth.md).

### Таймауты сервера

| Параметр | Значение |
|---|---|
| Заголовки | 5 секунд |
| Чтение | 10 секунд |
| Запись | 45 секунд |
| Idle | 60 секунд |
| Максимальный размер заголовков | 1 МиБ |
| Graceful shutdown | 10 секунд |

Таймаут RAG ограничен `RAG_TIMEOUT_SECONDS` (1–120, по умолчанию 30) и должен быть меньше таймаута записи. Явного ограничения размера тела запроса нет.

## 2. Ошибки

Единственный формат ошибки:

```json
{"error":{"code":"VALIDATION_ERROR","message":"Некорректный формат запроса."}}
```

`code` стабилен и предназначен для машинной обработки; `message` — человекочитаемый текст на русском. Код всегда соответствует одному HTTP-статусу.

| Код | Статус | Когда |
|---|---|---|
| `VALIDATION_ERROR` | 422 | Неверный JSON, неизвестное поле, пустой/длинный запрос, неверные фильтры, ID, статус или пагинация |
| `UNAUTHENTICATED` | 401 | Нет или неверен access-токен; нет сессии для refresh |
| `TOKEN_EXPIRED` | 401 | Истёк access-токен |
| `INVALID_CREDENTIALS` | 401 | Неверная пара email/пароль при входе |
| `EMAIL_TAKEN` | 409 | Email уже зарегистрирован |
| `FORBIDDEN` | 403 | `Origin` не совпал с `ALLOWED_ORIGIN` |
| `NOT_FOUND` | 404 | Нет фильма, записи истории или фильма в личном списке |
| `METHOD_NOT_ALLOWED` | 405 | Путь есть, метод не поддерживается |
| `RATE_LIMITED` | 429 | Превышен лимит авторизации; есть `Retry-After` |
| `DEPENDENCY_UNAVAILABLE` | 503 | RAG недоступен (и каталога нет в кеше) |
| `DEADLINE_EXCEEDED` | 504 | RAG не ответил вовремя |
| `INTERNAL` | 500 | Непредвиденная ошибка; детали только в логе с `request_id` |

Код `CONFLICT` объявлен в DTO, но текущие обработчики его не возвращают: конфликты email отдаются как `EMAIL_TAKEN`.

## 3. Служебные ручки

### `GET /health`

Без авторизации. Проверяет только живость процесса.

```json
{"status":"ok"}
```

Ответ всегда 200, пока процесс отвечает.

### `GET /ready`

Без авторизации. Проверяет PostgreSQL и актуальность схемы; RAG не проверяется.

```json
{"status":"ok"}
```

Если база недоступна, миграции не применены или схема `dirty`:

```json
{"status":"unavailable","failed_checks":["postgres"]}
```

Статус — 503. Детали ошибки пишутся в лог, но не отдаются клиенту.

## 4. Авторизация

Подробные сценарии, формат JWT и хранение — в [auth.md](auth.md). Кратко:

| Ручка | Доступ | Успех | Тело |
|---|---|---|---|
| `POST /api/v1/auth/register` | без токена | 201 | `user`, `access_token`, `expires_in` |
| `POST /api/v1/auth/login` | без токена | 200 | то же |
| `POST /api/v1/auth/refresh` | refresh-cookie | 200 | `access_token`, `expires_in` |
| `POST /api/v1/auth/logout` | refresh-cookie | 204 | — |
| `GET /api/v1/auth/me` | access | 200 | `id`, `email`, `created_at` |

Тело регистрации и входа:

```json
{"email":"user@example.com","password":"password123"}
```

Правила: email нормализуется (`trim` + `lower`), один `@`, точка в домене, до 254 символов; пароль 8–128 символов. Ошибки — 422, 409 `EMAIL_TAKEN`, 401 `INVALID_CREDENTIALS`, 429.

`refresh` без cookie → 401 `UNAUTHENTICATED` «Сессия не найдена». `logout` идемпотентен: без cookie и с неизвестным токеном всё равно 204.

## 5. Каталог и карточки

### `GET /api/v1/catalog`

Требует access-токен. Отдаёт метаданные поиска; кешируется на 5 минут ([cache.md](cache.md)).

```json
{
  "genres": [{"id": 28, "name": "Action"}],
  "movie_count": 4803,
  "year_from": 1902,
  "year_to": 2017,
  "default_results": 10,
  "max_results": 50,
  "available_strategies": ["bm25", "transformer", "hybrid"],
  "default_strategy": "hybrid"
}
```

`year_from`/`year_to` опускаются, если RAG их не сообщил. `max_candidates` из нового контракта RAG не отдаётся: бэкенд его не использует.

Ошибки: 401, 503 `DEPENDENCY_UNAVAILABLE` (RAG недоступен и кеш пуст), 504.

### `GET /api/v1/movies/{movie_id}`

`movie_id` — положительное целое. Иное значение → 422 `VALIDATION_ERROR`.

```json
{
  "movie_id": 603,
  "title": "The Matrix",
  "year": 1999,
  "genres": [{"id": 28, "name": "Action"}],
  "rating": 8.2,
  "vote_count": 24589,
  "overview": "A hacker discovers the truth."
}
```

`year` и `rating` опускаются, если неизвестны. Карточки не кешируются и всегда идут в RAG.

Ошибки: 401, 404 (фильма нет), 422, 503, 504.

## 6. Подбор

### `POST /api/v1/recommendations`

Требует access-токен. Тело:

| Поле | Тип | Обязательность | Ограничения |
|---|---|---|---|
| `query` | string | да | После `trim` 1–1000 символов |
| `limit` | integer | нет | 1..`max_results` каталога (если максимум не сообщён — 50); по умолчанию `default_results` или 10 |
| `filters` | object | нет | См. ниже |
| `strategy` | string | нет | Одна из `available_strategies`; иначе 422 |
| `generate_explanations` | boolean | нет | Если не передан, решает конфигурация RAG |

Фильтры:

| Поле | Тип | Ограничения |
|---|---|---|
| `genre_ids` | array of integer | ID жанров из каталога; пустой/отсутствующий — без фильтра |
| `year_from` | integer | 1800–3000 |
| `year_to` | integer | 1800–3000, не меньше `year_from` |
| `min_rating` | number | 0–10 |

Пример:

```json
{
  "query": "космос",
  "limit": 5,
  "filters": {"genre_ids": [878], "year_from": 1990},
  "strategy": "hybrid"
}
```

Успех — 200:

```json
{
  "history_id": "5b0c4f1e-6d8a-4f3b-9c2d-1e2f3a4b5c6d",
  "mode": "generated",
  "strategy_used": "hybrid",
  "items": [
    {
      "movie_id": 603,
      "title": "The Matrix",
      "year": 1999,
      "genres": [{"id": 28, "name": "Action"}],
      "rating": 8.2,
      "vote_count": 24589,
      "overview": "A hacker discovers the truth.",
      "reason": "Подходит под запрос"
    }
  ],
  "warnings": []
}
```

- `history_id` — `null`, если снимок не удалось сохранить; тогда в `warnings` появляется `HISTORY_NOT_SAVED`, а результат всё равно отдаётся.
- `mode` и `strategy_used` приходят от RAG (`generated`, `retrieval_only` и т. п.), бэкенд их не переписывает.
- `warnings` — массив объектов `{code, message}`; коды RAG пробрасываются как есть (например, `FEWER_RESULTS`), плюс backend-код `HISTORY_NOT_SAVED`.
- `reason` опускается, если объяснения нет; `items` и `warnings` — всегда массивы.

Каждый успешный вызов создаёт новую запись истории. `POST` не идемпотентен: повтор с тем же телом — новый `history_id`.

Ошибки: 401, 422 (запрос, фильтры, стратегия, лимит), 503 (RAG недоступен или каталог не получен), 504 (таймаут RAG). Ошибка сохранения истории не превращает ответ в ошибку.

## 7. История

### `GET /api/v1/history?page=1&page_size=20`

Одна страница истории пользователя, новые сверху (`created_at DESC, id DESC`).

```json
{
  "items": [
    {
      "history_id": "5b0c4f1e-6d8a-4f3b-9c2d-1e2f3a4b5c6d",
      "query": "космос",
      "filters": {"genre_ids": [878]},
      "requested_limit": 5,
      "created_at": "2026-01-02T03:04:05Z"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 1
}
```

`filters` — тот же формат, что в запросе подбора; `genre_ids` всегда массив.

### `GET /api/v1/history/{history_id}`

`history_id` — UUID. Возвращает полный снимок; RAG не вызывается.

```json
{
  "history_id": "5b0c4f1e-6d8a-4f3b-9c2d-1e2f3a4b5c6d",
  "query": "космос",
  "filters": {"genre_ids": [878]},
  "requested_limit": 5,
  "created_at": "2026-01-02T03:04:05Z",
  "response": {
    "mode": "generated",
    "strategy_used": "hybrid",
    "items": [{"movie_id": 603, "title": "The Matrix"}],
    "warnings": []
  }
}
```

Пример сокращён: в реальном ответе `items` содержат полную карточку с `reason`, как в ответе подбора. Неверный UUID → 422; чужая или отсутствующая запись → 404.

### `DELETE /api/v1/history/{history_id}`

Удаляет запись владельца. Успех — 204 без тела. Повторное удаление или чужая запись → 404. Неверный UUID → 422.

## 8. Личные списки

### `GET /api/v1/my/movies?status=want_to_watch&page=1&page_size=20`

`status` обязателен: `want_to_watch` или `watched`. Иное/отсутствующее значение → 422. Сортировка — `updated_at DESC, movie_id`.

```json
{
  "items": [
    {
      "movie_id": 603,
      "title": "The Matrix",
      "year": 1999,
      "genres": [{"id": 28, "name": "Action"}],
      "rating": 8.2,
      "vote_count": 24589,
      "overview": "A hacker discovers the truth.",
      "status": "watched",
      "liked": true,
      "updated_at": "2026-01-02T03:04:05Z"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 1
}
```

Карточка плоская: те же поля, что у фильма, плюс `status`, `liked`, `updated_at`. `liked` присутствует всегда и равен `null`, если оценки нет.

### `PUT /api/v1/my/movies/{movie_id}`

Тело:

```json
{"status": "watched", "liked": true}
```

| Поле | Обязательность | Правила |
|---|---|---|
| `status` | да | `want_to_watch` или `watched` |
| `liked` | нет | Только для `watched`; вместе с `want_to_watch` → 422 |

Поведение:

- первый `PUT` проверяет фильм через RAG и сохраняет снимок карточки; фильма нет → 404, RAG недоступен → 503/504;
- повторный `PUT` не вызывает RAG и не обновляет снимок;
- `watched → want_to_watch` сбрасывает `liked` в `null`;
- `want_to_watch → watched` ставит `liked` из тела;
- `watched` без `liked` означает «без оценки» и сбрасывает предыдущую оценку;
- успех — 200 с полной записью, как в списке.

### `DELETE /api/v1/my/movies/{movie_id}`

Удаляет запись владельца. 204; повторно или чужая — 404; неверный ID — 422.

## 9. Пагинация

Применима к истории и личным спискам.

| Параметр | По умолчанию | Допустимо |
|---|---|---|
| `page` | 1 | ≥ 1 |
| `page_size` | 20 | 1–100 |

Нечисловое значение, `page < 1`, `page_size < 1` или `> 100` → 422. Смещение — `(page - 1) * page_size`; курсоров и ссылок `next/prev` нет, клиент считает страницы сам. `total` — общее число записей по фильтру.

## 10. Идемпотентность

| Операция | Повтор |
|---|---|
| `PUT /my/movies/{id}` | Идемпотентен по состоянию: тот же статус/оценка не создают дубль (upsert); `updated_at` меняется |
| `DELETE` | Повтор даёт 404, а не 204: удобно для очистки, но не «идемпотентен по коду» |
| `POST /recommendations` | Не идемпотентен: каждый вызов создаёт запись истории |
| `POST /auth/register` | Повтор с тем же email → 409 |
| `POST /auth/login` | Каждый вход создаёт новую refresh-сессию |
| `POST /auth/refresh` | Ротирует токен; повтор старого токена отзывает семью |
| `GET` | Безопасны, без побочных эффектов |

Ключей идемпотентности (`Idempotency-Key`) нет.

## 11. Ограничения

- Нет OpenAPI/Swagger; контракт поддерживается вручную и тестами.
- Нет CORS: сервис рассчитан на один origin через прокси; прямой cross-origin потребует доработки.
- Нет ограничения размера тела запроса; есть только лимит заголовков 1 МиБ и таймауты.
- Нет заголовков состояния rate limit и метрик.
- Нет `PATCH`, массовых операций, сортировки и поиска по истории.
- `CONFLICT` объявлен, но не возвращается.
- `ClientIP()` доверяет прокси по умолчанию (gin 1.12); для публичного развёртывания нужно настроить доверенные прокси — см. [rate-limiting.md](rate-limiting.md).
- Тексты ошибок русскоязычные и не являются стабильным контрактом; стабилен только `code`.
- Версия API одна; поля могут добавляться, удаление и смена смысла потребуют новой версии.
