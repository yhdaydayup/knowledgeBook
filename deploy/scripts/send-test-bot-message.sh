#!/usr/bin/env bash
set -euo pipefail

APP_ID="${TEST_BOT_APP_ID:-cli_a9475ca6cd7b9cee}"
APP_SECRET="${TEST_BOT_APP_SECRET:-IR65raQwZ2u9xWAVd5KfgfxZAQxICawD}"
CHAT_ID="${TEST_BOT_CHAT_ID:-oc_ea14e9086e4dfb843b9f228efd8c49d5}"
MESSAGE="${1:-@knowledgeBook 帮我记一下，测试机器人触发了一条真实群聊测试消息。}"

TOKEN_JSON=$(curl -s -X POST "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d "{\"app_id\":\"${APP_ID}\",\"app_secret\":\"${APP_SECRET}\"}")
TOKEN=$(python3 - <<'PY' "$TOKEN_JSON"
import json,sys
print(json.loads(sys.argv[1]).get('tenant_access_token',''))
PY
)

if [ -z "$TOKEN" ]; then
  echo "failed to get tenant_access_token"
  echo "$TOKEN_JSON"
  exit 1
fi

CONTENT=$(python3 - <<'PY' "$MESSAGE"
import json,sys
print(json.dumps({"text": sys.argv[1]}, ensure_ascii=False))
PY
)

curl -s -X POST "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id" \
  -H "Content-Type: application/json; charset=utf-8" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{\"receive_id\":\"${CHAT_ID}\",\"msg_type\":\"text\",\"content\":$(python3 - <<'PY' "$CONTENT"
import json,sys
print(json.dumps(sys.argv[1], ensure_ascii=False))
PY
)}"