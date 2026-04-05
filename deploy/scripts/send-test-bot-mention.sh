#!/usr/bin/env bash
set -euo pipefail

TEST_APP_ID="${TEST_BOT_APP_ID:-cli_a9475ca6cd7b9cee}"
TEST_APP_SECRET="${TEST_BOT_APP_SECRET:-IR65raQwZ2u9xWAVd5KfgfxZAQxICawD}"
TEST_CHAT_ID="${TEST_BOT_CHAT_ID:-oc_ea14e9086e4dfb843b9f228efd8c49d5}"
KB_APP_ID="${KB_BOT_APP_ID:-cli_a9420533cc38dccb}"
KB_APP_SECRET="${KB_BOT_APP_SECRET:-d3KxDy3f9y4mWiCe1TYonfxPfLtFmkUy}"
MESSAGE_SUFFIX="${1:-帮我记一下，测试机器人已打通真实 mention 链路，请回复这条消息。}"

KB_TOKEN_JSON=$(curl -s -X POST "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d "{\"app_id\":\"${KB_APP_ID}\",\"app_secret\":\"${KB_APP_SECRET}\"}")
KB_TOKEN=$(python3 - <<'PY' "$KB_TOKEN_JSON"
import json,sys
print(json.loads(sys.argv[1]).get('tenant_access_token',''))
PY
)

KB_BOT_INFO=$(curl -s "https://open.feishu.cn/open-apis/bot/v3/info" -H "Authorization: Bearer ${KB_TOKEN}")
KB_OPEN_ID=$(python3 - <<'PY' "$KB_BOT_INFO"
import json,sys
print(json.loads(sys.argv[1]).get('bot',{}).get('open_id',''))
PY
)

TEST_TOKEN_JSON=$(curl -s -X POST "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d "{\"app_id\":\"${TEST_APP_ID}\",\"app_secret\":\"${TEST_APP_SECRET}\"}")
TEST_TOKEN=$(python3 - <<'PY' "$TEST_TOKEN_JSON"
import json,sys
print(json.loads(sys.argv[1]).get('tenant_access_token',''))
PY
)

python3 - <<'PY' "$TEST_TOKEN" "$TEST_CHAT_ID" "$KB_OPEN_ID" "$MESSAGE_SUFFIX"
import json,sys,urllib.request
TOKEN, CHAT_ID, KB_OPEN_ID, MESSAGE_SUFFIX = sys.argv[1:5]
content = {
  "zh_cn": {
    "title": "真实 @ knowledgeBook 测试",
    "content": [[
      {"tag": "at", "user_id": KB_OPEN_ID, "user_name": "knowledgeBook"},
      {"tag": "text", "text": " " + MESSAGE_SUFFIX}
    ]]
  }
}
body = json.dumps({
  "receive_id": CHAT_ID,
  "msg_type": "post",
  "content": json.dumps(content, ensure_ascii=False)
}, ensure_ascii=False).encode('utf-8')
req = urllib.request.Request(
  'https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id',
  data=body,
  headers={'Content-Type':'application/json; charset=utf-8','Authorization':'Bearer '+TOKEN},
)
print(urllib.request.urlopen(req, timeout=20).read().decode())
PY