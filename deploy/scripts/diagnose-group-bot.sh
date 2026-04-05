#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MESSAGE="${1:-@knowledgeBook 帮我记一下，测试机器人发起真实群聊诊断。}"

echo "== compose status =="
(cd "$ROOT" && docker compose ps)

echo
echo "== healthz =="
curl -s http://127.0.0.1:8080/healthz || true

echo
echo "== readyz =="
curl -s http://127.0.0.1:8080/readyz || true

echo
echo "== send test message =="
bash "$ROOT/scripts/send-test-bot-message.sh" "$MESSAGE"

echo
echo "== wait 5s =="
sleep 5

echo
echo "== recent group messages =="
bash "$ROOT/scripts/check-test-group-messages.sh"

echo
echo "== app-server recent logs =="
(cd "$ROOT" && docker compose logs app-server --tail=200)