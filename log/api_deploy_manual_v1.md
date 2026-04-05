<callout background-color="light-blue">
本稿基于《小帮手 后端模块拆分、数据库 Schema 与机器人指令协议》继续细化，补充三类可直接落地的内容：API 请求/响应 JSON 示例、Docker Compose 部署方案、用户部署手册初稿。
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | 小帮手 API 详细协议、Docker Compose 部署方案与用户部署手册 |
| 文档版本 | v1.0 |
| 更新时间 | 2026-03-29 |
| 适用范围 | MVP 落地与交付 |

---

## 二、API 详细协议

### 2.1 通用约定

#### 请求头
```http
Content-Type: application/json
X-Request-Id: <uuid>
Authorization: Bearer <token>
```

#### 通用响应结构
```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "requestId": "7f2e4d3c-xxxx"
}
```

#### 通用错误结构
```json
{
  "code": 4001,
  "message": "knowledge not found",
  "details": {
    "knowledgeId": 1001
  },
  "requestId": "7f2e4d3c-xxxx"
}
```

#### 建议错误码
| code | 含义 |
| --- | --- |
| 0 | 成功 |
| 4001 | 知识不存在 |
| 4002 | 分类不存在 |
| 4003 | 参数非法 |
| 4004 | 状态不允许当前操作 |
| 4005 | 已超过恢复窗口 |
| 5001 | 飞书文档同步失败 |
| 5002 | 文档回收未找到映射 |
| 5003 | 未检测到文档差异 |
| 5004 | AI 分类服务异常 |

---

### 2.2 飞书事件接入

#### `POST /api/v1/feishu/events`
用于接收飞书机器人事件。

#### challenge 校验响应
```json
{
  "challenge": "test-challenge"
}
```

#### 消息事件示例
```json
{
  "schema": "2.0",
  "header": {
    "event_id": "8d4c",
    "event_type": "im.message.receive_v1",
    "create_time": "1710000000",
    "token": "token"
  },
  "event": {
    "message": {
      "message_id": "om_xxx",
      "chat_id": "oc_xxx",
      "chat_type": "p2p",
      "message_type": "text",
      "content": "{\"text\":\"/kb search 登录流程\"}"
    },
    "sender": {
      "sender_id": {
        "open_id": "ou_xxx"
      }
    }
  }
}
```

---

### 2.3 新增知识

#### `POST /api/v1/knowledge`

#### 请求示例
```json
{
  "title": "登录接口设计",
  "content": "登录接口需要支持短信验证码校验。",
  "categoryPath": "工作/项目A/接口设计",
  "source": "BOT_MESSAGE"
}
```

#### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "draftId": 123,
    "recommendedCategoryPath": "工作/项目A/接口设计",
    "autoAcceptedCategory": true,
    "status": "PENDING_REVIEW"
  },
  "requestId": "req-001"
}
```

---

### 2.4 确认候选知识

#### `POST /api/v1/drafts/{id}/approve`

#### 请求示例
```json
{
  "categoryPath": "工作/项目A/接口设计",
  "acceptRecommendedCategory": true
}
```

#### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "knowledgeId": 1001,
    "status": "ACTIVE",
    "categoryPath": "工作/项目A/接口设计",
    "docAnchorLink": "https://xxx.feishu.cn/docx/abc#heading=h.def"
  },
  "requestId": "req-002"
}
```

---

### 2.5 忽略候选知识

#### `POST /api/v1/drafts/{id}/ignore`

#### 响应示例
```json
{
  "code": 0,
  "message": "ignored",
  "data": {
    "draftId": 123,
    "status": "IGNORED"
  },
  "requestId": "req-003"
}
```

---

### 2.6 稍后处理

#### `POST /api/v1/drafts/{id}/later`

#### 响应示例
```json
{
  "code": 0,
  "message": "moved to later list",
  "data": {
    "draftId": 123,
    "status": "LATER"
  },
  "requestId": "req-004"
}
```

#### 查看待办列表 `GET /api/v1/drafts/later`

#### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "draftId": 123,
        "title": "登录接口设计",
        "recommendedCategoryPath": "工作/项目A/接口设计",
        "updatedAt": "2026-03-29T00:00:00Z"
      }
    ]
  },
  "requestId": "req-005"
}
```

---

### 2.7 检索知识

#### `GET /api/v1/knowledge/search?query=登录流程&category=工作/项目A`

#### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "answer": "登录流程需要先校验验证码，再创建会话。",
    "items": [
      {
        "knowledgeId": 1001,
        "title": "登录接口设计",
        "categoryPath": "工作/项目A/接口设计",
        "updatedAt": "2026-03-29T00:00:00Z",
        "docAnchorLink": "https://xxx.feishu.cn/docx/abc#heading=h.def"
      }
    ]
  },
  "requestId": "req-006"
}
```

---

### 2.8 更新知识

#### `PUT /api/v1/knowledge/{id}`

#### 请求示例
```json
{
  "title": "登录接口设计 V2",
  "content": "登录接口需要支持短信验证码与设备指纹校验。",
  "categoryPath": "工作/项目A/接口设计",
  "changeReason": "补充设备指纹逻辑"
}
```

#### 响应示例
```json
{
  "code": 0,
  "message": "updated",
  "data": {
    "knowledgeId": 1001,
    "currentVersion": 2,
    "status": "ACTIVE",
    "docAnchorLink": "https://xxx.feishu.cn/docx/abc#heading=h.def"
  },
  "requestId": "req-007"
}
```

---

### 2.9 调整分类

#### `POST /api/v1/knowledge/{id}/move-category`

#### 请求示例
```json
{
  "categoryPath": "工作/项目A/API设计"
}
```

#### 响应示例
```json
{
  "code": 0,
  "message": "moved",
  "data": {
    "knowledgeId": 1001,
    "categoryPath": "工作/项目A/API设计",
    "docAnchorLink": "https://xxx.feishu.cn/docx/new#heading=h.xyz"
  },
  "requestId": "req-008"
}
```

---

### 2.10 软删除知识

#### `DELETE /api/v1/knowledge/{id}`

#### 响应示例
```json
{
  "code": 0,
  "message": "soft deleted",
  "data": {
    "knowledgeId": 1001,
    "status": "REMOVED_SOFT",
    "removedAt": "2026-03-29T00:00:00Z",
    "purgeAt": "2026-04-28T00:00:00Z"
  },
  "requestId": "req-009"
}
```

---

### 2.11 恢复知识

#### `POST /api/v1/knowledge/{id}/restore`

#### 响应示例
```json
{
  "code": 0,
  "message": "restored",
  "data": {
    "knowledgeId": 1001,
    "status": "ACTIVE",
    "currentVersion": 3,
    "docAnchorLink": "https://xxx.feishu.cn/docx/abc#heading=h.def"
  },
  "requestId": "req-010"
}
```

---

### 2.12 文档修改回收

#### `POST /api/v1/knowledge/{id}/sync-from-doc`

#### 请求示例
```json
{
  "syncMode": "anchor",
  "changeReason": "用户手动修改文档后回收"
}
```

#### 成功响应
```json
{
  "code": 0,
  "message": "synced from doc",
  "data": {
    "knowledgeId": 1001,
    "currentVersion": 4,
    "source": "DOC_SYNC_BACKFILL"
  },
  "requestId": "req-011"
}
```

#### 无差异响应
```json
{
  "code": 5003,
  "message": "no diff detected",
  "details": {
    "knowledgeId": 1001
  },
  "requestId": "req-012"
}
```

---

### 2.13 分类树查询

#### `GET /api/v1/categories/tree`

#### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 1,
        "name": "工作",
        "path": "工作",
        "children": [
          {
            "id": 2,
            "name": "项目A",
            "path": "工作/项目A",
            "children": []
          }
        ]
      }
    ]
  },
  "requestId": "req-013"
}
```

---

## 三、Docker Compose 部署方案

### 3.1 推荐部署拓扑
MVP 推荐 4 个核心服务：
- `postgres`
- `redis`
- `app-server`
- `app-worker`

### 3.2 示例目录结构
```plaintext
deploy/
├── docker-compose.yml
├── .env.example
├── init.sql
├── scripts/
│   ├── start.sh
│   ├── stop.sh
│   └── healthcheck.sh
└── README.md
```

### 3.3 Docker Compose 示例

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16
    container_name: knowu-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql:ro

  redis:
    image: redis:7
    container_name: knowu-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  app-server:
    image: knowu/app-server:latest
    container_name: knowu-app-server
    restart: unless-stopped
    depends_on:
      - postgres
      - redis
    env_file:
      - .env
    ports:
      - "8080:8080"

  app-worker:
    image: knowu/app-worker:latest
    container_name: knowu-app-worker
    restart: unless-stopped
    depends_on:
      - postgres
      - redis
    env_file:
      - .env

volumes:
  postgres_data:
  redis_data:
```

### 3.4 `.env.example` 建议
```env
POSTGRES_DB=knowu
POSTGRES_USER=knowu
POSTGRES_PASSWORD=change_me
POSTGRES_DSN=postgres://knowu:change_me@postgres:5432/knowu?sslmode=disable
REDIS_ADDR=redis:6379

APP_PORT=8080
APP_ENV=production
APP_LOG_LEVEL=info

FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=xxx
FEISHU_VERIFICATION_TOKEN=xxx
FEISHU_ENCRYPT_KEY=

FEISHU_DOC_FOLDER_TOKEN=
AUTO_CLASSIFY_TOP1_THRESHOLD=0.85
AUTO_CLASSIFY_GAP_THRESHOLD=0.15
SOFT_DELETE_RETENTION_DAYS=30
```

### 3.5 健康检查建议
- `GET /healthz`：进程存活检查
- `GET /readyz`：数据库、Redis、飞书配置可用性检查

### 3.6 运维建议
- PostgreSQL 数据卷必须持久化
- `.env` 不应提交到仓库
- 应定期备份数据库
- Worker 日志建议单独采集

---

## 四、用户部署手册初稿

## 4.1 部署前准备
部署前请准备：
1. 一台可运行 Docker / Docker Compose 的服务器
2. 飞书开放平台中的自建应用
3. 已开通机器人能力与消息事件订阅能力
4. PostgreSQL、Redis 所需磁盘空间

---

## 4.2 创建飞书应用

### 步骤 1：创建应用
在飞书开放平台创建企业自建应用，并开启机器人能力。

### 步骤 2：配置权限
至少申请以下权限：
- 机器人收发消息相关权限
- 飞书文档读取 / 写入权限
- 如需文档定位与层级管理，需补齐文档结构相关权限

### 步骤 3：配置事件订阅
将事件回调地址配置为：
```plaintext
https://<你的域名>/api/v1/feishu/events
```

---

## 4.3 准备部署文件
将以下文件放到服务器部署目录：
- `docker-compose.yml`
- `.env`
- `init.sql`
- 启动脚本 / 停止脚本（可选）

### `.env` 必填项
- 数据库连接
- Redis 地址
- 飞书应用 ID / Secret
- 飞书验证 token
- 文档目录 token（如需要）

---

## 4.4 启动服务

### 方式一：直接启动
```bash
docker compose up -d
```

### 方式二：重新构建后启动
```bash
docker compose up -d --build
```

### 查看服务状态
```bash
docker compose ps
```

### 查看日志
```bash
docker compose logs -f app-server
docker compose logs -f app-worker
```

---

## 4.5 初始化验证
服务启动后，建议按以下顺序验证：

### 验证 1：健康检查
```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

### 验证 2：飞书回调
在飞书开发者后台完成 challenge 校验，并确认消息回调成功。

### 验证 3：机器人消息
给小帮手发送：
```plaintext
/kb add 登录接口 | 登录接口需支持短信验证码
```
确认机器人是否返回候选知识卡片。

### 验证 4：检索能力
发送：
```plaintext
/kb search 登录流程
```
确认是否返回答案、条目和文档位置链接。

---

## 4.6 常用运维命令

### 停止服务
```bash
docker compose down
```

### 重启服务
```bash
docker compose restart
```

### 仅重启 Worker
```bash
docker compose restart app-worker
```

### 查看数据库容器
```bash
docker exec -it knowu-postgres psql -U knowu -d knowu
```

---

## 4.7 常见问题排查

### 问题 1：机器人收不到消息
请检查：
- 飞书应用是否发布到可用状态
- 事件订阅 URL 是否可访问
- 验证 token 是否正确
- 机器人是否已添加到聊天对象

### 问题 2：候选知识生成失败
请检查：
- app-worker 是否正常运行
- AI 服务配置是否正确
- 日志中是否有分类服务或提炼服务错误

### 问题 3：文档同步失败
请检查：
- 飞书文档权限是否已授予
- 文档目录 token 是否正确
- app-worker 日志中的 `DOC_SYNC_FAILED`

### 问题 4：手动修改文档后未同步到数据库
请确认：
- 用户是否显式发送了 `/kb sync-from-doc`
- 该知识是否已有文档映射
- 文档修改位置是否能被系统识别

---

## 4.8 升级与回滚建议

### 升级建议
1. 备份 PostgreSQL 数据
2. 备份 `.env`
3. 更新镜像版本
4. 执行数据库 migration
5. 重启服务并验证健康检查

### 回滚建议
1. 切回旧镜像版本
2. 恢复数据库备份
3. 重启服务
4. 重新验证机器人指令

---

## 五、交付建议

这一阶段建议对外最终交付：
1. API 详细协议文档
2. PostgreSQL Schema SQL 文件
3. Docker Compose 部署方案
4. 用户部署手册
5. 示例 `.env.example`
6. 健康检查接口说明
