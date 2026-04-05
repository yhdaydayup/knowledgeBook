#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
DEPLOY_DIR="$ROOT_DIR/deploy"
ENV_FILE="$DEPLOY_DIR/.env"
APP_PORT="8080"
HAS_ERROR=0
HAS_WARN=0

ok() {
  echo "[OK] $1"
}

warn() {
  echo "[WARN] $1"
  HAS_WARN=1
}

err() {
  echo "[ERROR] $1"
  HAS_ERROR=1
}

require_or_warn() {
  local cmd="$1"
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "命令可用: $cmd"
    return 0
  fi
  err "缺少命令: $cmd"
  return 1
}

read_env_value() {
  local key="$1"
  if [[ ! -f "$ENV_FILE" ]]; then
    return 0
  fi
  awk -F= -v target="$key" '$1 == target {sub($1 FS, ""); print; exit}' "$ENV_FILE"
}

check_runtime() {
  echo "== 运行时检查 =="
  require_or_warn docker

  if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
      ok "Docker daemon 可用"
    else
      err "Docker daemon 不可用，请先启动 Docker / Colima"
    fi

    if docker compose version >/dev/null 2>&1; then
      ok "docker compose 可用"
    else
      err "docker compose 不可用"
    fi

    if docker buildx version >/dev/null 2>&1; then
      ok "docker buildx 可用"
    else
      err "docker buildx 不可用"
    fi
  fi
  echo
}

check_env_file() {
  echo "== 配置文件检查 =="
  if [[ -f "$ENV_FILE" ]]; then
    ok "已找到环境文件: $ENV_FILE"
  else
    err "未找到环境文件: $ENV_FILE"
    echo
    return
  fi

  local keys="POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DSN REDIS_ADDR APP_PORT FEISHU_APP_ID FEISHU_APP_SECRET FEISHU_VERIFICATION_TOKEN"
  for key in $keys; do
    local value
    value="$(read_env_value "$key")"
    if [[ -n "$value" ]]; then
      ok "$key 已配置"
    else
      err "$key 未配置"
    fi
  done

  local feishu_app_id feishu_app_secret verification_token
  feishu_app_id="$(read_env_value "FEISHU_APP_ID")"
  feishu_app_secret="$(read_env_value "FEISHU_APP_SECRET")"
  verification_token="$(read_env_value "FEISHU_VERIFICATION_TOKEN")"
  APP_PORT="$(read_env_value "APP_PORT")"
  APP_PORT="${APP_PORT:-8080}"

  if [[ -z "$feishu_app_id" || "$feishu_app_id" == "cli_xxx" ]]; then
    warn "FEISHU_APP_ID 为空或仍为示例值，文档同步将退化为 SIMULATED"
  fi
  if [[ -z "$feishu_app_secret" || "$feishu_app_secret" == "xxx" ]]; then
    warn "FEISHU_APP_SECRET 为空或仍为示例值，文档同步将退化为 SIMULATED"
  fi
  if [[ -z "$verification_token" || "$verification_token" == "xxx" ]]; then
    warn "FEISHU_VERIFICATION_TOKEN 为空或仍为示例值，飞书事件验签将不可用于真实环境"
  fi
  echo
}

check_compose_status() {
  echo "== 服务状态检查 =="
  if [[ ! -d "$DEPLOY_DIR" ]]; then
    err "deploy 目录不存在: $DEPLOY_DIR"
    echo
    return
  fi

  if ! command -v docker >/dev/null 2>&1; then
    echo
    return
  fi

  local ps_output
  if ps_output="$(cd "$DEPLOY_DIR" && docker compose ps 2>/dev/null)"; then
    echo "$ps_output"
    if echo "$ps_output" | grep -q "knowledgebook-app-server"; then
      ok "Compose 服务已创建"
    else
      warn "Compose 服务尚未启动，建议执行: bash deploy/scripts/bootstrap.sh"
    fi
  else
    warn "无法获取 docker compose ps 输出"
  fi
  echo
}

check_health() {
  echo "== 健康检查 =="
  local healthz_url="http://127.0.0.1:$APP_PORT/healthz"
  local readyz_url="http://127.0.0.1:$APP_PORT/readyz"

  if curl -fsS "$healthz_url" >/dev/null 2>&1; then
    ok "healthz 通过: $healthz_url"
  else
    err "healthz 失败: $healthz_url"
  fi

  if curl -fsS "$readyz_url" >/dev/null 2>&1; then
    ok "readyz 通过: $readyz_url"
  else
    err "readyz 失败: $readyz_url"
  fi
  echo
}

print_summary() {
  echo "== 诊断结论 =="
  if [[ "$HAS_ERROR" -eq 0 && "$HAS_WARN" -eq 0 ]]; then
    echo "整体状态：可直接使用。"
    return
  fi
  if [[ "$HAS_ERROR" -eq 0 ]]; then
    echo "整体状态：可运行，但仍有配置风险。"
  else
    echo "整体状态：存在阻塞问题，需先修复 ERROR 项。"
  fi

  echo "建议动作："
  echo "- 查看部署文档: docs/DEPLOYMENT.md"
  echo "- 一键部署: bash deploy/scripts/bootstrap.sh"
  echo "- 查看 app-server 日志: cd deploy && docker compose logs --tail=200 app-server"
  echo "- 查看 app-worker 日志: cd deploy && docker compose logs --tail=200 app-worker"
}

check_runtime
check_env_file
check_compose_status
check_health
print_summary

if [[ "$HAS_ERROR" -ne 0 ]]; then
  exit 1
fi
