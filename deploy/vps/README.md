# Этап 1: автоматический деплой API на VPS

Этот каталог добавляет CD для сервера, где развёрнут Movie Picker, не меняя его архитектуру.

## Что обновляется

- API-образ: `ghcr.io/<owner>/rag-movie-picker/api:sha-<commit>`;
- сервис `api` и образ `migrate` в `/opt/movie-picker/compose.json`;
- миграции;
- контейнер `api`.

Не меняются:

- PostgreSQL и его том `pgdata`;
- Qdrant и его том;
- Redis;
- Caddy;
- Python-рекомендер;
- `/opt/movie-picker/.env`, `recommender.env`, `app`, `venv`, `state`, `data`.

## Как работает

1. Push в `main` запускает `Go backend CI`.
2. CI после тестов собирает интегрированный образ (Go + frontend) и пушит его в GHCR с тегом `sha-<commit>`.
3. `Deploy VPS` запускается только после успешного CI.
4. Workflow заходит по SSH и запускает `deploy.sh`.
5. Скрипт сохраняет `compose.json`, меняет образ у `api` и `migrate`, делает `pull`, `migrate`, `up -d api`, ждёт `/ready` и проверяет, что регистрация отвечает 404.
6. При ошибке возвращает старый `compose.json` и перезапускает старый api.

## Требования к серверу

- Docker и Compose plugin;
- каталог `/opt/movie-picker` с `compose.json`;
- доступ к GHCR (`docker login ghcr.io` с PAT `read:packages`, если пакет приватный);
- SSH-пользователь, который может читать `/opt/movie-picker/.env` и писать `compose.json`.

Сейчас `.env` и `recommender.env` принадлежат `root` и имеют права `600`. Поэтому для первого запуска нужен либо root-пользователь по отдельному ключу, либо отдельный `deploy` с группой/ACL и доступом к Docker. Это надо решить до включения workflow.

## Секреты GitHub

Environment `production`:

| Секрет | Значение |
|---|---|
| `SSH_HOST` | `<SERVER_IP>` |
| `SSH_PORT` | `<SSH_PORT>` |
| `SSH_USER` | пользователь с доступом |
| `SSH_PRIVATE_KEY` | приватный ключ |
| `SSH_KNOWN_HOSTS` | `ssh-keyscan -p <SSH_PORT> <SERVER_IP>` |

Ключ должен быть отдельным, не личным ключом разработчика.

## Первый запуск

1. Сделать бэкап: `pg_dump`, снапшот Qdrant, копию `.env`, `compose.json`, `Caddyfile`, текущий образ.
2. Добавить секреты в GitHub.
3. Смержить PR с CI/CD в `main`.
4. Дождаться green CI и появления образа в GHCR.
5. Проверить Actions: `Deploy VPS` должен быть зелёным.
6. Проверить на сервере:

```bash
cd /opt/movie-picker
docker compose -f compose.json ps -a
curl -i http://127.0.0.1:8080/health
curl -i http://127.0.0.1:8080/ready
curl -I https://<DOMAIN_OR_IP>/
```

7. Проверить вход тестовым аккаунтом, каталог, подбор, историю.
8. Проверить, что `POST /api/v1/auth/register` отвечает 404.

## Откат

Скрипт оставляет `compose.backup-<timestamp>.json` и тег `movie-picker-api:backup-<timestamp>`.

```bash
cp /opt/movie-picker/compose.backup-<timestamp>.json /opt/movie-picker/compose.json
docker compose -f /opt/movie-picker/compose.json up -d --no-deps api
```

Миграции откатываются только вручную и только если они совместимы; в рабочей среде лучше писать миграции вперёд.

## Ограничения этапа 1

- Короткий простой при пересоздании одного контейнера `api`.
- Caddy и Python-рекомендер обновляются отдельно.
- Нет zero-downtime.
- `compose.json` сервера пока не версионируется полностью; в репозитории лежит только Caddyfile как документация.

Полный compose с recommender, Qdrant и Caddy — этап 2, после стабилизации этого pipeline.
