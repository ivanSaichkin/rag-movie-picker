#!/usr/bin/env bash
# Stage 1 deployment: replace only the API container in the existing stack.
# PostgreSQL, Redis, Qdrant, Caddy and the Python recommender are not touched.
set -euo pipefail

image="${1:?usage: deploy.sh <image> <commit>}"
commit="${2:?usage: deploy.sh <image> <commit>}"

base="${APP_DIR:-/opt/movie-picker}"
compose="$base/compose.json"

cd "$base"

if [[ ! -f "$compose" ]]; then
  echo "compose file not found: $compose" >&2
  exit 1
fi

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup="$base/compose.backup-${stamp}.json"
current_image="$(python3 - "$compose" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    data = json.load(source)
print(data["services"]["api"]["image"])
PY
)"

cp "$compose" "$backup"
if docker image inspect "$current_image" >/dev/null 2>&1; then
  docker tag "$current_image" "movie-picker-api:backup-${stamp}"
fi

rollback() {
  echo "deployment failed; restoring ${backup}" >&2
  cp "$backup" "$compose" || true
  if [[ -n "$current_image" ]] && docker image inspect "$current_image" >/dev/null 2>&1; then
    docker compose -f "$compose" up -d --no-deps api >/dev/null 2>&1 || true
  fi
}
trap 'code=$?; if [[ $code -ne 0 ]]; then rollback; fi' EXIT

python3 - "$compose" "$image" <<'PY'
import json
import os
import sys

path, image = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as source:
    data = json.load(source)

for name in ("api", "migrate"):
    if name not in data.get("services", {}):
        raise SystemExit(f"service {name!r} is missing from {path}")
    data["services"][name]["image"] = image

temporary = path + ".tmp"
with open(temporary, "w", encoding="utf-8") as target:
    json.dump(data, target, indent=2, ensure_ascii=False)
    target.write("\n")
os.replace(temporary, path)
PY

echo "deploying ${image} (commit ${commit})"
# Compose may skip the pull when the VPS uses pull_policy: never, so pull the
# image explicitly and let compose use the local copy afterwards.
docker pull "$image"
docker compose -f "$compose" pull api migrate
# This script is piped over SSH; run must not consume its stdin.
docker compose -f "$compose" run --rm migrate < /dev/null
# Recreate explicitly: the older container may be reported healthy while still
# running the previous image if compose considers it up to date.
docker compose -f "$compose" up -d --no-deps --force-recreate api

port="$(python3 - "$compose" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    data = json.load(source)
env = data["services"]["api"].get("environment", {})
if isinstance(env, list):
    env = dict(item.split("=", 1) for item in env if "=" in item)
address = env.get("HTTP_ADDR", "127.0.0.1:8080")
print(address.rsplit(":", 1)[-1] or "8080")
PY
)"

ready=0
for _ in $(seq 1 45); do
  if curl -fsS "http://127.0.0.1:${port}/ready" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "$ready" -ne 1 ]]; then
  echo "API did not become ready" >&2
  docker compose -f "$compose" logs --tail=100 api >&2 || true
  exit 1
fi

status="$(curl -sS -o /tmp/movie-picker-registration.json -w '%{http_code}' \
  -X POST "http://127.0.0.1:${port}/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"invalid","password":"invalid"}' || true)"
if [[ "$status" != "404" && "$status" != "403" ]]; then
  echo "registration is not disabled (status ${status:-none})" >&2
  exit 1
fi

echo "deployment complete: ${image} (commit ${commit})"
docker compose -f "$compose" ps -a || true
