#!/usr/bin/env bash
set -euo pipefail
REPLY_VALUE=""
MODE="interactive"

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
DEPLOY_DIR="$ROOT_DIR/deploy"
ENV_FILE="$DEPLOY_DIR/.env"
HEALTHCHECK_SCRIPT="$DEPLOY_DIR/scripts/healthcheck.sh"

require_command() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[ERROR] 缺少命令: $cmd"
    exit 1
  fi
}

usage() {
  cat <<EOF
用法:
  bash deploy/scripts/bootstrap.sh
  bash deploy/scripts/bootstrap.sh --from-env

模式说明:
  默认模式           交互式收集部署参数
  --from-env         直接使用当前 shell 环境变量生成 deploy/.env 并启动服务

--from-env 模式至少建议提供:
  POSTGRES_PASSWORD

可选环境变量:
  POSTGRES_DB
  POSTGRES_USER
  REDIS_ADDR
  APP_PORT
  APP_ENV
  APP_LOG_LEVEL
  AUTO_CLASSIFY_TOP1_THRESHOLD
  AUTO_CLASSIFY_GAP_THRESHOLD
  SOFT_DELETE_RETENTION_DAYS
  AUTO_MIGRATE
  FEISHU_DOC_BASE_URL
  ENABLE_FEISHU
  FEISHU_APP_ID
  FEISHU_APP_SECRET
  FEISHU_VERIFICATION_TOKEN   # 事件订阅校验 token，不是 tenant_access_token
  FEISHU_ENCRYPT_KEY
  PUBLIC_BASE_URL
EOF
}

prompt_default() {
  local prompt="$1"
  local default_value="$2"
  local value
  if ! read -r -p "$prompt [$default_value]: " value; then
    value=""
  fi
  if [[ -z "$value" ]]; then
    value="$default_value"
  fi
  REPLY_VALUE="$value"
}

prompt_required() {
  local prompt="$1"
  local value
  while true; do
    if ! read -r -p "$prompt: " value; then
      value=""
    fi
    if [[ -n "$value" ]]; then
      REPLY_VALUE="$value"
      return
    fi
    echo "该项不能为空，请重新输入。"
  done
}

prompt_secret_required() {
  local prompt="$1"
  local value
  while true; do
    if ! read -r -s -p "$prompt: " value; then
      value=""
    fi
    echo
    if [[ -n "$value" ]]; then
      REPLY_VALUE="$value"
      return
    fi
    echo "该项不能为空，请重新输入。"
  done
}

prompt_yes_no() {
  local prompt="$1"
  local default_value="$2"
  local input
  local hint="y/N"
  if [[ "$default_value" == "y" ]]; then
    hint="Y/n"
  fi
  while true; do
    if ! read -r -p "$prompt [$hint]: " input; then
      input=""
    fi
    input="${input:-$default_value}"
    input=$(printf '%s' "$input" | tr '[:upper:]' '[:lower:]')
    case "$input" in
      y|yes) REPLY_VALUE='y'; return ;;
      n|no) REPLY_VALUE='n'; return ;;
      *) echo "请输入 y 或 n。" ;;
    esac
  done
}

check_runtime() {
  require_command docker
  if ! docker info >/dev/null 2>&1; then
    echo "[ERROR] Docker daemon 不可用，请先启动 Docker / Colima。"
    exit 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    echo "[ERROR] docker compose 不可用，请先安装 Docker Compose。"
    exit 1
  fi
  if ! docker buildx version >/dev/null 2>&1; then
    echo "[ERROR] docker buildx 不可用，请先安装 buildx 插件。"
    exit 1
  fi
}

backup_existing_env() {
  if [[ -f "$ENV_FILE" ]]; then
    local backup_file="$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
    cp "$ENV_FILE" "$backup_file"
    echo "[INFO] 已备份现有 .env -> $backup_file"
  fi
}

write_env() {
  cat > "$ENV_FILE" <<EOF
POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DSN=postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@postgres:5432/$POSTGRES_DB?sslmode=disable
REDIS_ADDR=$REDIS_ADDR
APP_PORT=$APP_PORT
APP_ENV=$APP_ENV
APP_LOG_LEVEL=$APP_LOG_LEVEL
FEISHU_APP_ID=$FEISHU_APP_ID
FEISHU_APP_SECRET=$FEISHU_APP_SECRET
FEISHU_VERIFICATION_TOKEN=$FEISHU_VERIFICATION_TOKEN
FEISHU_ENCRYPT_KEY=$FEISHU_ENCRYPT_KEY
FEISHU_DOC_BASE_URL=$FEISHU_DOC_BASE_URL
AUTO_CLASSIFY_TOP1_THRESHOLD=$AUTO_CLASSIFY_TOP1_THRESHOLD
AUTO_CLASSIFY_GAP_THRESHOLD=$AUTO_CLASSIFY_GAP_THRESHOLD
SOFT_DELETE_RETENTION_DAYS=$SOFT_DELETE_RETENTION_DAYS
AUTO_MIGRATE=$AUTO_MIGRATE
EOF
}

start_services() {
  echo "[INFO] 启动 Docker Compose 服务。"
  (
    cd "$DEPLOY_DIR"
    docker compose up -d --build
  )
}

run_healthcheck() {
  echo "[INFO] 执行健康检查。"
  local attempts=0
  local max_attempts=10
  until bash "$HEALTHCHECK_SCRIPT" >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [[ "$attempts" -ge "$max_attempts" ]]; then
      echo "[WARN] 健康检查失败，请检查日志："
      echo "  cd $DEPLOY_DIR && docker compose logs --tail=200 app-server"
      echo "  cd $DEPLOY_DIR && docker compose logs --tail=200 app-worker"
      exit 1
    fi
    sleep 2
  done
  echo "[OK] 服务健康检查通过。"
}

print_summary() {
  echo
  echo "================ 部署完成 ================"
  echo "模式: $MODE"
  echo "项目目录: $ROOT_DIR"
  echo "环境文件: $ENV_FILE"
  echo "服务地址: http://127.0.0.1:$APP_PORT"
  echo "健康检查: http://127.0.0.1:$APP_PORT/healthz"
  echo "就绪检查: http://127.0.0.1:$APP_PORT/readyz"
  echo
  if [[ "$ENABLE_FEISHU" == "y" ]]; then
    echo "飞书接入信息："
    echo "- 事件回调地址: $FEISHU_CALLBACK_URL"
    echo "- Verification Token: $FEISHU_VERIFICATION_TOKEN"
    if [[ -n "$FEISHU_ENCRYPT_KEY" ]]; then
      echo "- Encrypt Key: 已配置"
    else
      echo "- Encrypt Key: 未配置（当前版本可留空）"
    fi
    echo
    echo "建议在飞书中完成："
    echo "1. 创建企业自建应用并启用机器人。"
    echo "2. 配置事件订阅回调地址为: $FEISHU_CALLBACK_URL"
    echo "3. 至少开启文本消息接收事件。"
    echo "4. 给机器人发送测试命令："
    echo "   /kb help"
    echo "   /kb add 登录接口 | 登录接口需支持短信验证码"
  else
    echo "当前未启用飞书接入，仅完成本地 API / worker 部署。"
    echo "如需后续接入飞书，请修改 $ENV_FILE 后重启服务。"
  fi
  echo
  echo "常用命令："
  echo "- 启动: bash deploy/scripts/start.sh"
  echo "- 停止: bash deploy/scripts/stop.sh"
  echo "- 健康检查: bash deploy/scripts/healthcheck.sh"
  echo "- 诊断: bash deploy/scripts/doctor.sh"
  echo "- 查看状态: cd deploy && docker compose ps"
  echo "- 查看日志: cd deploy && docker compose logs --tail=200 app-server"
  echo "========================================="
}

load_from_env() {
  POSTGRES_DB="${POSTGRES_DB:-knowledgebook}"
  POSTGRES_USER="${POSTGRES_USER:-knowledgebook}"
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"
  REDIS_ADDR="${REDIS_ADDR:-redis:6379}"
  APP_PORT="${APP_PORT:-8080}"
  APP_ENV="${APP_ENV:-production}"
  APP_LOG_LEVEL="${APP_LOG_LEVEL:-info}"
  AUTO_CLASSIFY_TOP1_THRESHOLD="${AUTO_CLASSIFY_TOP1_THRESHOLD:-0.85}"
  AUTO_CLASSIFY_GAP_THRESHOLD="${AUTO_CLASSIFY_GAP_THRESHOLD:-0.15}"
  SOFT_DELETE_RETENTION_DAYS="${SOFT_DELETE_RETENTION_DAYS:-30}"
  AUTO_MIGRATE="${AUTO_MIGRATE:-true}"
  FEISHU_DOC_BASE_URL="${FEISHU_DOC_BASE_URL:-https://www.feishu.cn/docx}"
  ENABLE_FEISHU="${ENABLE_FEISHU:-n}"
  FEISHU_APP_ID="${FEISHU_APP_ID:-}"
  FEISHU_APP_SECRET="${FEISHU_APP_SECRET:-}"
  FEISHU_VERIFICATION_TOKEN="${FEISHU_VERIFICATION_TOKEN:-}"
  FEISHU_ENCRYPT_KEY="${FEISHU_ENCRYPT_KEY:-}"
  PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
  FEISHU_CALLBACK_URL=""

  if [[ -z "$POSTGRES_PASSWORD" ]]; then
    echo "[ERROR] --from-env 模式下必须提供 POSTGRES_PASSWORD"
    exit 1
  fi

  if [[ "$ENABLE_FEISHU" == "y" ]]; then
    if [[ -z "$PUBLIC_BASE_URL" || -z "$FEISHU_APP_ID" || -z "$FEISHU_APP_SECRET" || -z "$FEISHU_VERIFICATION_TOKEN" ]]; then
      echo "[ERROR] --from-env 模式启用飞书时，必须提供 PUBLIC_BASE_URL、FEISHU_APP_ID、FEISHU_APP_SECRET、FEISHU_VERIFICATION_TOKEN"
      exit 1
    fi
    FEISHU_CALLBACK_URL="${PUBLIC_BASE_URL%/}/api/v1/feishu/events"
  fi
}

load_interactive() {
  prompt_default 'PostgreSQL 数据库名' 'knowledgebook'
  POSTGRES_DB="$REPLY_VALUE"
  prompt_default 'PostgreSQL 用户名' 'knowledgebook'
  POSTGRES_USER="$REPLY_VALUE"
  prompt_secret_required 'PostgreSQL 密码'
  POSTGRES_PASSWORD="$REPLY_VALUE"
  prompt_default 'Redis 地址' 'redis:6379'
  REDIS_ADDR="$REPLY_VALUE"
  prompt_default '应用端口' '8080'
  APP_PORT="$REPLY_VALUE"
  prompt_default '运行环境' 'production'
  APP_ENV="$REPLY_VALUE"
  prompt_default '日志级别' 'info'
  APP_LOG_LEVEL="$REPLY_VALUE"
  prompt_default '自动分类 Top1 阈值' '0.85'
  AUTO_CLASSIFY_TOP1_THRESHOLD="$REPLY_VALUE"
  prompt_default '自动分类 gap 阈值' '0.15'
  AUTO_CLASSIFY_GAP_THRESHOLD="$REPLY_VALUE"
  prompt_default '软删除保留天数' '30'
  SOFT_DELETE_RETENTION_DAYS="$REPLY_VALUE"
  prompt_default '是否自动执行 migration(true/false)' 'true'
  AUTO_MIGRATE="$REPLY_VALUE"
  prompt_default '飞书文档基础 URL' 'https://www.feishu.cn/docx'
  FEISHU_DOC_BASE_URL="$REPLY_VALUE"

  prompt_yes_no '是否现在配置飞书接入' 'y'
  ENABLE_FEISHU="$REPLY_VALUE"
  FEISHU_APP_ID=""
  FEISHU_APP_SECRET=""
  FEISHU_VERIFICATION_TOKEN=""
  FEISHU_ENCRYPT_KEY=""
  FEISHU_CALLBACK_URL=""

  if [[ "$ENABLE_FEISHU" == "y" ]]; then
    prompt_required '请输入公网 HTTPS 基础地址（例如 https://kb.example.com）'
    PUBLIC_BASE_URL="$REPLY_VALUE"
    prompt_required '请输入 FEISHU_APP_ID'
    FEISHU_APP_ID="$REPLY_VALUE"
    prompt_secret_required '请输入 FEISHU_APP_SECRET'
    FEISHU_APP_SECRET="$REPLY_VALUE"
    prompt_required '请输入 FEISHU_VERIFICATION_TOKEN'
    FEISHU_VERIFICATION_TOKEN="$REPLY_VALUE"
    prompt_default '请输入 FEISHU_ENCRYPT_KEY（可留空）' ''
    FEISHU_ENCRYPT_KEY="$REPLY_VALUE"
    FEISHU_CALLBACK_URL="${PUBLIC_BASE_URL%/}/api/v1/feishu/events"
  fi
}

if [[ $# -gt 1 ]]; then
  usage
  exit 1
fi
if [[ $# -eq 1 ]]; then
  case "$1" in
    --from-env)
      MODE="from-env"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 1
      ;;
  esac
fi

echo "knowledgeBook MVP 部署向导 ($MODE)"
check_runtime

if [[ "$MODE" == "from-env" ]]; then
  load_from_env
else
  load_interactive
fi

backup_existing_env
write_env

echo "[INFO] 已生成部署配置: $ENV_FILE"
start_services
run_healthcheck
print_summary
