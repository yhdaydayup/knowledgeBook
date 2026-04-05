# knowledgeBook 部署手册

本文档面向第一次接触 knowledgeBook 的用户，目标是在一台全新机器上完成：
- 启动 PostgreSQL / Redis / app-server / app-worker
- 配置飞书自建应用与事件订阅
- 让用户通过飞书机器人用 `/kb` 命令接入 MVP
- 完成健康检查与最小链路验证

---

## 1. MVP 能力边界

当前交付版本支持：
- 单用户、私有知识助手
- 飞书机器人私聊命令入口
- 知识 draft 创建、approve、search
- knowledge update / soft delete / restore / sync-from-doc API
- PostgreSQL FTS 搜索
- worker 异步消费 `DOC_SYNC_KNOWLEDGE`
- 飞书文档同步：
  - 无真实凭据时走 `SIMULATED`
  - 有真实凭据时走 Feishu OpenAPI 最小写入链路

当前仍是 MVP，不包含：
- 多用户协作
- Web 控制台
- 复杂权限模型
- 完整 block 级 patch/update 回写闭环
- 生产级监控告警体系

---

## 2. 部署架构

推荐部署方式：Docker Compose。

默认启动 4 个服务：
- `postgres`
- `redis`
- `app-server`
- `app-worker`

其中：
- `app-server` 暴露 HTTP API 与飞书回调入口
- `app-worker` 负责异步任务，如文档同步

---

## 3. 环境要求

### 3.1 通用要求
- Docker
- Docker Compose
- buildx 插件
- 可访问公网的 HTTPS 回调地址（供飞书事件订阅使用）

### 3.2 本地开发编译要求
如需在宿主机本地编译，使用 Go `1.24.x`。

### 3.3 macOS(Homebrew) 参考
```bash
brew install docker docker-compose colima docker-buildx go@1.24
colima start --cpu 2 --memory 4 --disk 20
export PATH="/opt/homebrew/opt/go@1.24/bin:/opt/homebrew/bin:$PATH"
go build ./...
```

---

## 4. 获取代码与准备配置

### 4.1 进入项目目录
```bash
cd knowU
```

### 4.2 统一入口命令（推荐）
项目根目录提供了 `Makefile`，可直接使用：

```bash
make install-local      # 交互式一键部署
make deploy-from-env    # 非交互部署
make diagnose           # 环境与部署诊断
make start              # 启动服务
make stop               # 停止服务
make healthcheck        # 健康检查
make build              # 本地编译
```

### 4.3 推荐：使用交互式一键部署向导
如果你希望由脚本自动收集部署信息、生成 `.env`、启动服务并执行健康检查，优先使用：

```bash
bash deploy/scripts/bootstrap.sh
```

如果你希望在 CI、远程自动化或批量部署中直接使用已有环境变量，也可以使用非交互模式：

```bash
POSTGRES_PASSWORD=change_me \
ENABLE_FEISHU=n \
bash deploy/scripts/bootstrap.sh --from-env
```

推荐做法是基于示例文件准备一份安装配置：

```bash
cp deploy/install.example.env deploy/install.env
vi deploy/install.env
set -a
source deploy/install.env
set +a
bash deploy/scripts/bootstrap.sh --from-env
```

该脚本会依次询问：
- PostgreSQL 用户名/密码
- 应用端口与运行环境
- 自动分类与软删除参数
- 是否启用飞书接入
- 飞书 App ID / App Secret / Verification Token
- 公网 HTTPS 回调基础地址
该脚本会自动完成：
- 检查 Docker / Compose / buildx
- 备份现有 `deploy/.env`
- 生成新的 `deploy/.env`
- 启动 `docker compose up -d --build`
- 执行健康检查
- 输出飞书后台需要填写的回调地址与后续步骤

### 4.4 部署诊断脚本
如果你需要在目标机器上快速判断“环境是否完整、服务是否启动、飞书配置是否仍是示例值”，可以执行：

```bash
bash deploy/scripts/doctor.sh
```

该脚本会检查：
- Docker daemon / compose / buildx
- `deploy/.env` 是否存在及关键字段是否已填写
- 飞书配置是否仍为示例值
- `docker compose ps` 服务状态
- `healthz` / `readyz` 是否通过

### 4.5 手动模式：复制环境变量模板
```bash
cp deploy/.env.example deploy/.env
```

### 4.6 必填环境变量
编辑 `deploy/.env`，至少确认以下配置：

```env
POSTGRES_DB=knowledgebook
POSTGRES_USER=knowledgebook
POSTGRES_PASSWORD=change_me
POSTGRES_DSN=postgres://knowledgebook:change_me@postgres:5432/knowledgebook?sslmode=disable
REDIS_ADDR=redis:6379
APP_PORT=8080
APP_ENV=production
APP_LOG_LEVEL=info
FEISHU_APP_ID=你的真实 App ID
FEISHU_APP_SECRET=你的真实 App Secret
FEISHU_VERIFICATION_TOKEN=你的事件订阅 token
FEISHU_ENCRYPT_KEY=
FEISHU_DOC_BASE_URL=https://www.feishu.cn/docx
LLM_ENABLED=false
LLM_BASE_URL=https://your-llm-provider.example.com/v1
LLM_API_KEY=你的模型服务密钥
LLM_MODEL=你选择的模型名
LLM_TIMEOUT_MS=8000
LLM_MAX_TOKENS=1200
LLM_FALLBACK_ENABLED=true
AUTO_CLASSIFY_TOP1_THRESHOLD=0.85
AUTO_CLASSIFY_GAP_THRESHOLD=0.15
SOFT_DELETE_RETENTION_DAYS=30
AUTO_MIGRATE=true
```

### 4.4 环境变量说明
- `POSTGRES_DSN`：服务连接 PostgreSQL 的内部地址，Compose 默认用 `postgres:5432`
- `REDIS_ADDR`：服务连接 Redis 的内部地址，Compose 默认用 `redis:6379`
- `FEISHU_APP_ID` / `FEISHU_APP_SECRET`：飞书自建应用凭据，服务会用它们自动换取 `tenant_access_token`
- `FEISHU_VERIFICATION_TOKEN`：飞书事件订阅校验 token，不是 `tenant_access_token`
- `FEISHU_ENCRYPT_KEY`：当前可留空；如果未来启用加密事件，再补充
- `FEISHU_DOC_BASE_URL`：飞书 doc 链接前缀，默认即可
- `LLM_ENABLED`：是否启用外部 LLM 能力；关闭时系统只走本地 fallback
- `LLM_BASE_URL`：外部模型服务地址，默认按 OpenAI-compatible 的 `/chat/completions` 协议拼接
- `LLM_API_KEY`：外部模型服务密钥
- `LLM_MODEL`：要调用的模型名
- `LLM_TIMEOUT_MS`：单次 LLM 请求超时
- `LLM_MAX_TOKENS`：单次 LLM 返回 token 上限
- `LLM_FALLBACK_ENABLED`：LLM 出错时是否退回规则逻辑
- `AUTO_MIGRATE=true`：服务启动时自动执行 `migrations/*.sql`

---

## 5. 启动服务

### 5.1 直接使用 Docker Compose
```bash
cd deploy
docker compose up -d --build
```

### 5.2 或使用脚本
```bash
bash deploy/scripts/start.sh
```

### 5.3 查看运行状态
```bash
cd deploy
docker compose ps
```

正常情况下你会看到：
- `knowledgebook-postgres`
- `knowledgebook-redis`
- `knowledgebook-app-server`
- `knowledgebook-app-worker`

均处于 `Up` 状态。

---

## 6. 健康检查

### 6.1 直接检查
```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

期望结果：
```json
{"code":0,"message":"ok"}
{"code":0,"message":"ready"}
```

### 6.2 使用脚本
```bash
bash deploy/scripts/healthcheck.sh
```

---

## 7. 飞书自建应用配置

### 7.1 创建应用
在飞书开放平台创建**企业自建应用**，记录：
- App ID
- App Secret

并填入 `deploy/.env`。

### 7.2 配置机器人能力
为应用启用机器人，并保证用户可以给机器人发私聊消息。

### 7.3 事件订阅
在飞书开放平台中开启事件订阅，请求地址填写：
```text
https://<你的域名>/api/v1/feishu/events
```

同时配置：
- Verification Token：写入 `FEISHU_VERIFICATION_TOKEN`
- Encrypt Key：当前版本可不启用，留空即可

### 7.4 推荐订阅事件
至少订阅文本消息接收相关事件，用于机器人命令入口。当前 MVP 主要处理文本消息事件并解析 `/kb` 命令。

### 7.5 HTTPS 要求
飞书事件回调必须是公网可访问的 HTTPS 地址。
如果本地开发，可通过反向代理或隧道工具暴露 `8080`。

---

## 8. 飞书接入后的命令使用

当前 MVP 已支持的最小命令：

### 8.1 查看帮助
```text
/kb help
```

### 8.2 创建草稿
```text
/kb add 登录接口 | 登录接口需支持短信验证码
```

返回结果会包含：
- draft ID
- 推荐分类
- 下一步 approve 提示

### 8.3 确认草稿
```text
/kb approve 12
```
或指定分类：
```text
/kb approve 12 工作/项目A/接口设计
```

### 8.4 搜索知识
```text
/kb search 登录接口
```

---

## 9. API 入口速查

服务默认监听：`http://127.0.0.1:8080`

关键路由：
- `POST /api/v1/feishu/events`
- `GET /api/v1/bot/command`
- `POST /api/v1/knowledge`
- `GET /api/v1/knowledge/search`
- `GET /api/v1/knowledge/:id`
- `PUT /api/v1/knowledge/:id`
- `DELETE /api/v1/knowledge/:id`
- `POST /api/v1/knowledge/:id/restore`
- `POST /api/v1/knowledge/:id/sync-from-doc`
- `POST /api/v1/drafts/:id/approve`
- `POST /api/v1/doc-sync/knowledge/:id`

代码位置：`internal/api/router.go:8`

---

## 10. 最小自测流程

推荐部署完成后按以下顺序验证：

### 10.1 服务健康
```bash
bash deploy/scripts/healthcheck.sh
```

### 10.2 机器人命令链路
向机器人私聊发送：
```text
/kb add 登录接口 | 登录接口需支持短信验证码
```
然后：
```text
/kb approve <草稿ID>
/kb search 登录接口
```

### 10.3 API 链路
如需直接验证 API：
- create draft
- approve draft
- search
- update
- soft delete
- restore
- sync-from-doc

### 10.4 worker 链路
触发知识创建或更新后，检查 worker 是否消费 `DOC_SYNC_KNOWLEDGE`：
```bash
cd deploy
docker compose logs --tail=100 app-worker
```

---

## 11. 文档同步说明

### 11.1 模拟模式
当 `FEISHU_APP_ID` / `FEISHU_APP_SECRET`：
- 未配置
- 或仍为示例值（如 `cli_xxx` / `xxx`）

当前版本会退化为安全的模拟同步：
- 仍生成 `docLink` / `docAnchorLink`
- 仍写入 `doc_sync_mappings`
- `sync_status = SIMULATED`

这适合本地开发和无真实飞书凭据的环境。

### 11.2 真实同步模式
配置真实 `FEISHU_APP_ID` / `FEISHU_APP_SECRET` 后，服务会尝试：
- 调用 `tenant_access_token/internal` 获取租户访问凭证
- 按分类复用目标文档 ID（当前规则：`kb-u{userID}-c{categoryID}`）
- 调用 `POST /open-apis/docx/v1/documents/{document_id}/blocks/{block_id}/children` 写入知识标题与正文
- 为每条知识生成稳定的 `external_key` / `target_block_id`

若全部成功：
- `doc_sync_mappings.sync_status = SUCCESS`

若失败：
- worker 任务会进入重试或失败状态

### 11.3 当前限制
当前仍是 MVP：
- 已支持分类级目标文档复用
- 已支持稳定 block identity 落库
- 但尚未实现完整的 block 级 patch/update 回写

也就是说，目前已经为幂等更新准备好了数据结构，但真实飞书文档侧仍主要走最小写入链路。

---

## 12. 常见运维命令

### 12.1 查看服务状态
```bash
cd deploy
docker compose ps
```

### 12.2 查看 server 日志
```bash
cd deploy
docker compose logs --tail=200 app-server
```

### 12.3 查看 worker 日志
```bash
cd deploy
docker compose logs --tail=200 app-worker
```

### 12.4 查看数据库
```bash
docker exec -it knowledgebook-postgres psql -U knowledgebook -d knowledgebook
```

### 12.5 停止服务
```bash
bash deploy/scripts/stop.sh
```

### 12.6 彻底清理（谨慎）
```bash
cd deploy
docker compose down -v
```
这会删除容器与数据卷。

---

## 13. 升级与回滚

### 13.1 升级
在新机器或已有环境升级时：
```bash
git pull
cd deploy
docker compose up -d --build
```

若 `AUTO_MIGRATE=true`，新 migration 会在启动时自动执行。

### 13.2 回滚
若新版本异常：
1. 回退代码到上一个稳定版本
2. 重新执行：
```bash
cd deploy
docker compose up -d --build
```

注意：
- 若数据库 migration 已发生不可逆变更，需提前做好备份
- 当前新增 migration 主要为增量字段/索引，风险相对可控，但生产环境仍建议先备份数据库

---

## 14. 数据备份与恢复

### 14.1 备份 PostgreSQL
```bash
docker exec knowledgebook-postgres pg_dump -U knowledgebook knowledgebook > knowledgebook_backup.sql
```

### 14.2 恢复 PostgreSQL
```bash
cat knowledgebook_backup.sql | docker exec -i knowledgebook-postgres psql -U knowledgebook -d knowledgebook
```

### 14.3 备份建议
建议至少在以下场景前备份：
- 升级版本前
- 修改 migration 前
- 切换真实飞书凭据前

---

## 15. 常见问题

### 15.1 `readyz` 失败
检查 PostgreSQL 与 Redis 是否成功启动。

### 15.2 `docker compose` 提示 buildx 缺失
安装 `docker-buildx`，并确保 Docker 可识别 CLI 插件。

### 15.3 飞书回调失败
确认：
- 回调地址是公网 HTTPS
- 地址正确：`https://<你的域名>/api/v1/feishu/events`
- `FEISHU_VERIFICATION_TOKEN` 与飞书后台一致

### 15.4 文档手改无法回收
当前只有显式 `sync-from-doc` 才会从文档回收内容。
默认用户手改不会自动回写 DB。

### 15.5 文档同步一直是 `SIMULATED`
检查：
- `FEISHU_APP_ID` 是否仍是 `cli_xxx`
- `FEISHU_APP_SECRET` 是否仍是 `xxx`
- 是否真正重启了服务容器

### 15.6 worker 不消费任务
检查：
```bash
cd deploy
docker compose logs --tail=200 app-worker
```
并确认 Redis / PostgreSQL 正常、worker 容器处于 `Up` 状态。

---

## 16. 对外部署建议

如果要给其他用户快速部署，建议最小交付包包含：
- 当前代码仓库
- `docs/DEPLOYMENT.md`
- `deploy/.env.example`
- 一套已经申请好的飞书应用配置说明
- 一个公网 HTTPS 域名或反向代理方案

这样其他用户只需要：
1. 准备 Docker 环境
2. 填写 `.env`
3. 启动 Compose
4. 在飞书后台配置事件订阅
5. 给机器人发 `/kb` 命令

即可完成 MVP 接入。
