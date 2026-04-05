#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

docker compose --env-file .env -f docker-compose.prod.yml pull
docker compose --env-file .env -f docker-compose.prod.yml up -d
docker compose --env-file .env -f docker-compose.prod.yml ps

if command -v curl >/dev/null 2>&1; then
  printf '\n[healthz]\n'
  curl -fsS "http://127.0.0.1:${APP_HOST_PORT:-8080}/healthz" || true
  printf '\n[readyz]\n'
  curl -fsS "http://127.0.0.1:${APP_HOST_PORT:-8080}/readyz" || true
  printf '\n'
fi
