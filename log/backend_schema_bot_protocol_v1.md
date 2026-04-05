<callout background-color="light-blue">
本稿基于最终版 PRD v1.5、设计文档 v2.1、技术方案 v2.1 整理，目标是给出可直接进入研发落地的三项输出：后端模块拆分、数据库 Schema 定稿、飞书机器人指令协议。
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | 小帮手 后端模块拆分、数据库 Schema 与机器人指令协议 |
| 文档版本 | v1.0 |
| 更新时间 | 2026-03-28 |
| 适用范围 | MVP 落地实施 |

---

## 二、总体落地原则

### 2.1 MVP 技术形态
MVP 推荐采用：
- **单仓库**
- **单主服务 + 单异步 Worker**
- **PostgreSQL + Redis**
- **飞书机器人 + 飞书文档**

不建议在 MVP 阶段拆成多微服务。

### 2.2 逻辑模块与部署单元分离
本文中的“模块拆分”是**逻辑模块拆分**，不是部署时必须拆分成多个进程。

MVP 推荐部署方式：
- `app-server`：承载飞书回调、API、命令解析、查询接口
- `app-worker`：承载 AI 推荐分类、文档同步、清理任务、重试任务

### 2.3 核心事实源原则
- **数据库是唯一事实源**
- **飞书文档是派生展示层**
- **用户手动修改文档默认不回写数据库**
- **仅当用户显式发出同步指令时，才回收文档修改到数据库**

---

## 三、后端模块拆分

## 3.1 模块总览

| 模块 | 主要职责 | 是否对外 | 推荐部署 |
| --- | --- | --- | --- |
| bot-gateway | 飞书事件接入、验签、消息解析、消息回发 | 是 | app-server |
| command-router | 命令解析、参数标准化、权限校验、路由分发 | 是 | app-server |
| draft-service | 候选知识生成、待确认、稍后处理待办 | 是 | app-server |
| knowledge-service | 知识新增、更新、软删除、恢复、版本管理 | 是 | app-server |
| category-service | 分类树管理、路径计算、迁移与重命名 | 是 | app-server |
| ai-classifier | 分类推荐、高置信自动分类决策 | 否 | app-worker |
| search-service | 知识检索、答案组装、结果排序 | 是 | app-server |
| doc-sync-service | 飞书文档生成、更新、同步回收、锚点维护 | 否 | app-worker |
| cleanup-service | 软删除到期清理、死信任务补偿 | 否 | app-worker |
| audit-service | 操作日志、审计记录、埋点 | 否 | app-server/app-worker |

---

## 3.2 bot-gateway

### 职责
- 接收飞书机器人事件回调
- 完成验签、challenge 校验
- 解析文本消息、转发消息、卡片回调
- 将统一后的请求转交给 command-router
- 将结果格式化为机器人文本或卡片返回

### 输入
- 飞书 `im.message.receive_v1`
- 飞书卡片交互回调

### 输出
- 标准命令请求对象 `BotCommandRequest`
- 机器人消息响应 `BotCommandResponse`

### 注意点
- 过滤机器人自身消息
- 统一记录 `request_id`、`open_id`、`chat_id`
- 需支持幂等处理，防止飞书重复投递

---

## 3.3 command-router

### 职责
- 解析 `/kb ...` 指令
- 兼容自然语言场景下的显式命令触发
- 进行参数校验、权限校验、用户上下文加载
- 将请求路由至对应业务模块

### 标准命令结构
```json
{
  "requestId": "uuid",
  "userOpenId": "ou_xxx",
  "command": "kb.search",
  "args": {
    "query": "登录流程怎么做",
    "category": "工作/项目A"
  },
  "source": {
    "chatId": "oc_xxx",
    "messageId": "om_xxx"
  }
}
```

### 路由目标
- draft-service
- knowledge-service
- category-service
- search-service
- doc-sync-service

---

## 3.4 draft-service

### 职责
- 处理输入内容到候选知识草稿的生成
- 保存候选知识
- 管理待确认状态与稍后处理待办
- 调用 ai-classifier 获取推荐分类

### 关键动作
- `createDraftFromMessage`
- `listPendingDrafts`
- `approveDraft`
- `ignoreDraft`
- `moveDraftToLater`
- `restoreLaterDraft`

### 状态
- `PENDING_REVIEW`
- `IGNORED`
- `LATER`
- `APPROVED`

---

## 3.5 knowledge-service

### 职责
- 管理正式知识条目
- 支持更新、分类调整、软删除、恢复
- 管理知识版本
- 在需要时触发文档同步任务

### 关键动作
- `createKnowledgeFromDraft`
- `updateKnowledge`
- `moveKnowledgeCategory`
- `softDeleteKnowledge`
- `restoreKnowledge`
- `syncFromDoc`

### 关键规则
- 每次更新都创建新版本
- 软删除时写入 `removed_at` 与 `purge_at`
- 恢复时清空 `removed_at` / `purge_at`
- 文档同步回收成功后也创建新版本，来源标记为 `DOC_SYNC_BACKFILL`

---

## 3.6 category-service

### 职责
- 管理分类树
- 计算分类路径与层级
- 支持新增、改名、迁移、停用
- 为 doc-sync-service 提供文档层级映射依据

### 关键规则
- 最大层级暂定 5 级
- 优先遵循用户预设分类标准
- 若无用户标准，由系统生成初始一级 / 二级分类
- 分类路径唯一

### 关键动作
- `createCategory`
- `renameCategory`
- `moveCategory`
- `disableCategory`
- `ensureInitialCategories`

---

## 3.7 ai-classifier

### 职责
- 基于标题、摘要、内容、标签做分类推荐
- 返回 TopN 推荐路径、置信度、理由
- 决定是否满足“高置信自动分类”条件

### 自动分类默认阈值
建议默认规则：
- `top1_confidence >= 0.85`
- 且 `top1_confidence - top2_confidence >= 0.15`

同时满足时可自动分类。

### 输出示例
```json
{
  "recommendations": [
    {
      "path": "工作/项目A/接口设计",
      "confidence": 0.91,
      "reason": "内容高频涉及接口、鉴权、登录流程"
    },
    {
      "path": "工作/项目A/需求讨论",
      "confidence": 0.62,
      "reason": "包含少量需求描述"
    }
  ],
  "autoAccepted": true
}
```

---

## 3.8 search-service

### 职责
- 根据问题检索知识库
- 结合分类、标签、关键词进行过滤
- 组装“答案 + 条目 + 文档位置链接”的响应

### MVP 检索建议
优先采用：
- PostgreSQL 全文检索 `tsvector`
- 标签匹配
- 分类路径过滤

向量检索可在后续版本增强，不必阻塞 MVP。

### 检索输出
```json
{
  "answer": "登录流程应先进行验证码校验，再创建会话。",
  "items": [
    {
      "knowledgeId": 123,
      "title": "登录接口设计",
      "categoryPath": "工作/项目A/接口设计",
      "docAnchorLink": "https://...#heading=h.xxx"
    }
  ]
}
```

---

## 3.9 doc-sync-service

### 职责
- 将知识与分类同步到飞书文档
- 按“分类子文档为主”生成文档结构
- 维护锚点链接与映射关系
- 响应 `/kb sync-from-doc` 指令进行文档修改回收

### 文档结构策略
- 以分类子文档为主
- 子文档内结合多级标题承载细粒度层级
- 当内容过于稀疏或密集时自动调整分级方式

### 关键动作
- `syncKnowledge`
- `syncCategoryTree`
- `rebuildCategoryDoc`
- `syncFromDoc`
- `rebuildAllDocs`

### 文档修改回收规则
- 默认不自动回收手工修改
- 仅在用户显式发出同步指令时读取文档修改部分
- 若未发出同步指令，后续数据库同步会覆盖手工修改

---

## 3.10 cleanup-service

### 职责
- 定时清理超过 30 天未恢复的软删除知识
- 扫描失败任务并重试
- 扫描过期同步任务并进入死信处理

### 清理规则
- `removed_at IS NOT NULL`
- 且 `purge_at <= now()`
- 执行永久删除

### 建议调度频率
- 每小时运行一次到期清理任务
- 每 5 分钟运行一次任务补偿扫描

---

## 3.11 MVP 推荐部署单元

### app-server
承载：
- bot-gateway
- command-router
- draft-service
- knowledge-service
- category-service
- search-service
- audit-service

### app-worker
承载：
- ai-classifier
- doc-sync-service
- cleanup-service

### 基础组件
- PostgreSQL
- Redis

---

## 四、数据库 Schema 定稿

## 4.1 Schema 设计原则
- 数据库是唯一事实源
- 所有用户可见知识都必须有版本记录
- 软删除必须支持恢复窗口与到期清理
- 文档同步必须有映射表和任务表
- 文档修改回收必须生成新版本，不能原地覆盖

---

## 4.2 PostgreSQL DDL

```sql
CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  open_id VARCHAR(64) UNIQUE NOT NULL,
  name VARCHAR(128),
  role VARCHAR(32) NOT NULL DEFAULT 'user',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE categories (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  name VARCHAR(128) NOT NULL,
  parent_id BIGINT REFERENCES categories(id),
  level INT NOT NULL CHECK (level >= 1 AND level <= 5),
  path TEXT NOT NULL,
  path_key TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  source VARCHAR(32) NOT NULL DEFAULT 'system',
  status VARCHAR(32) NOT NULL DEFAULT 'enabled',
  doc_node_key TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, path_key)
);

CREATE TABLE knowledge_drafts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  input_type VARCHAR(32) NOT NULL,
  input_text TEXT NOT NULL,
  title VARCHAR(256),
  summary TEXT,
  content_markdown TEXT,
  tags TEXT[] NOT NULL DEFAULT '{}',
  recommended_category_path TEXT,
  recommendation_confidence NUMERIC(5,4),
  auto_accepted_category BOOLEAN NOT NULL DEFAULT FALSE,
  status VARCHAR(32) NOT NULL DEFAULT 'PENDING_REVIEW',
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE knowledge_items (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  draft_id BIGINT REFERENCES knowledge_drafts(id),
  title VARCHAR(256) NOT NULL,
  summary TEXT,
  content_markdown TEXT NOT NULL,
  tags TEXT[] NOT NULL DEFAULT '{}',
  primary_category_id BIGINT REFERENCES categories(id),
  category_path TEXT NOT NULL,
  confidence NUMERIC(5,4),
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
  current_version INT NOT NULL DEFAULT 1,
  auto_classified BOOLEAN NOT NULL DEFAULT FALSE,
  auto_classify_confidence NUMERIC(5,4),
  doc_link TEXT,
  doc_anchor_link TEXT,
  removed_at TIMESTAMPTZ,
  purge_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE knowledge_versions (
  id BIGSERIAL PRIMARY KEY,
  knowledge_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
  version_no INT NOT NULL,
  source VARCHAR(32) NOT NULL,
  title VARCHAR(256) NOT NULL,
  summary TEXT,
  content_markdown TEXT NOT NULL,
  tags TEXT[] NOT NULL DEFAULT '{}',
  category_path TEXT NOT NULL,
  editor_user_id BIGINT REFERENCES users(id),
  change_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(knowledge_id, version_no)
);

CREATE TABLE ai_category_recommendations (
  id BIGSERIAL PRIMARY KEY,
  draft_id BIGINT NOT NULL REFERENCES knowledge_drafts(id) ON DELETE CASCADE,
  rank_no INT NOT NULL,
  recommended_category_id BIGINT REFERENCES categories(id),
  recommended_path TEXT NOT NULL,
  confidence NUMERIC(5,4) NOT NULL,
  reason TEXT,
  accepted BOOLEAN,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE doc_sync_mappings (
  id BIGSERIAL PRIMARY KEY,
  knowledge_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
  category_id BIGINT REFERENCES categories(id),
  parent_doc_id VARCHAR(128),
  target_doc_id VARCHAR(128) NOT NULL,
  anchor_key VARCHAR(256) NOT NULL,
  doc_link TEXT NOT NULL,
  anchor_link TEXT NOT NULL,
  last_sync_version INT NOT NULL DEFAULT 1,
  last_sync_hash VARCHAR(64),
  sync_status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(knowledge_id)
);

CREATE TABLE sync_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_type VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(32) NOT NULL DEFAULT 'QUEUED',
  retry_count INT NOT NULL DEFAULT 0,
  run_after TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error TEXT,
  executed_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE operation_logs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  actor_type VARCHAR(32) NOT NULL,
  action_type VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT,
  detail JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_user_parent ON categories(user_id, parent_id, sort_order);
CREATE INDEX idx_categories_user_path_key ON categories(user_id, path_key);
CREATE INDEX idx_drafts_user_status ON knowledge_drafts(user_id, status, created_at DESC);
CREATE INDEX idx_items_user_status ON knowledge_items(user_id, status, updated_at DESC);
CREATE INDEX idx_items_user_category ON knowledge_items(user_id, primary_category_id, updated_at DESC);
CREATE INDEX idx_items_purge_at ON knowledge_items(purge_at) WHERE purge_at IS NOT NULL;
CREATE INDEX idx_versions_knowledge_version ON knowledge_versions(knowledge_id, version_no DESC);
CREATE INDEX idx_ai_reco_draft_rank ON ai_category_recommendations(draft_id, rank_no);
CREATE INDEX idx_sync_tasks_status_run_after ON sync_tasks(status, run_after);
CREATE INDEX idx_logs_user_action_time ON operation_logs(user_id, action_type, created_at DESC);
```

---

## 4.3 关键字段说明

### `knowledge_items.status`
建议取值：
- `ACTIVE`
- `REMOVED_SOFT`

### `knowledge_versions.source`
建议取值：
- `DRAFT_APPROVE`
- `MANUAL_UPDATE`
- `DOC_SYNC_BACKFILL`
- `RESTORE`

### `knowledge_drafts.status`
建议取值：
- `PENDING_REVIEW`
- `IGNORED`
- `LATER`
- `APPROVED`

### `sync_tasks.task_type`
建议取值：
- `DOC_SYNC_KNOWLEDGE`
- `DOC_SYNC_CATEGORY`
- `DOC_REBUILD_ALL`
- `DOC_SYNC_FROM_DOC`
- `PURGE_SOFT_DELETED`

---

## 4.4 软删除与恢复规则

### 软删除
执行软删除时：
- `knowledge_items.status = 'REMOVED_SOFT'`
- `removed_at = NOW()`
- `purge_at = NOW() + INTERVAL '30 days'`

### 恢复
执行恢复时：
- `status = 'ACTIVE'`
- `removed_at = NULL`
- `purge_at = NULL`
- 创建一条 `knowledge_versions.source = 'RESTORE'`

### 超期清理
定时任务扫描：
```sql
DELETE FROM knowledge_items
WHERE status = 'REMOVED_SOFT'
  AND purge_at IS NOT NULL
  AND purge_at <= NOW();
```

注意：生产实现时建议先删依赖表，再删主表，或使用事务化清理逻辑。

---

## 4.5 文档修改同步回收规则

### 基本规则
- 用户手动修改飞书文档后，不会自动回写数据库
- 用户必须显式发送同步指令
- 系统仅读取知识对应位置的修改部分
- 回收成功后创建新版本

### 推荐回收流程
```plaintext
1. 用户发送 /kb sync-from-doc 知识ID
2. 系统根据 knowledge_id 找到 doc_sync_mappings
3. 读取 target_doc_id + anchor_key 对应位置内容
4. 与 knowledge_items.current_version 对比
5. 若内容变化，则写入 knowledge_versions 新版本
6. 更新 knowledge_items.current_version、content_markdown、updated_at
7. 触发索引更新与必要的文档整理任务
```

---

## 五、飞书机器人指令协议

## 5.1 协议原则
- 所有命令统一前缀：`/kb`
- 参数采用“位置参数 + 键值参数”混合方式
- 能返回结构化结果时优先返回卡片
- 所有破坏性操作要求二次确认

---

## 5.2 命令总览

| 命令 | 作用 | 示例 |
| --- | --- | --- |
| `/kb add` | 新增知识 | `/kb add 登录接口 | 登录接口需短信验证` |
| `/kb review` | 查看待确认候选 | `/kb review` |
| `/kb approve` | 确认候选 | `/kb approve 123` |
| `/kb ignore` | 忽略候选 | `/kb ignore 123` |
| `/kb later` | 稍后处理 | `/kb later 123` |
| `/kb later list` | 查看待办 | `/kb later list` |
| `/kb search` | 检索知识 | `/kb search 登录流程怎么做` |
| `/kb update` | 更新知识 | `/kb update 1001 | 内容=补充验证码逻辑` |
| `/kb move` | 调整分类 | `/kb move 1001 | 工作/项目A/接口设计` |
| `/kb remove` | 软删除知识 | `/kb remove 1001` |
| `/kb restore` | 恢复软删除知识 | `/kb restore 1001` |
| `/kb sync-from-doc` | 回收文档手改 | `/kb sync-from-doc 1001` |
| `/kb category add` | 新增分类 | `/kb category add 工作/项目A | 接口设计` |
| `/kb category rename` | 分类改名 | `/kb category rename 工作/项目A/接口设计 | API设计` |
| `/kb sync` | 手动触发同步 | `/kb sync knowledge 1001` |

---

## 5.3 命令详细协议

### 5.3.1 新增知识
```plaintext
/kb add 标题 | 内容
/kb add 标题 | 内容 | 分类路径
```

返回：
- 候选知识卡片
- 推荐分类
- 操作按钮：沉淀 / 忽略 / 稍后处理 / 修改分类

### 5.3.2 查看待确认候选
```plaintext
/kb review
/kb review 123
```

返回：
- 待确认列表或单条候选详情

### 5.3.3 确认候选
```plaintext
/kb approve 123
/kb approve 123 | 分类=工作/项目A/接口设计
```

返回：
- 已沉淀成功
- 分类路径
- 文档位置链接

### 5.3.4 稍后处理
```plaintext
/kb later 123
/kb later list
/kb later approve 123
/kb later ignore 123
```

返回：
- 已加入待办 / 待办列表 / 处理结果

### 5.3.5 检索知识
```plaintext
/kb search 登录流程怎么做
/kb search 分类:工作/项目A 登录流程
```

返回：
- 答案摘要
- 相关知识条目
- 文档位置链接

### 5.3.6 更新知识
```plaintext
/kb update 1001 | 标题=登录接口设计V2 | 内容=补充短信验证码逻辑
```

返回：
- 更新成功
- 新版本号
- 文档位置链接

### 5.3.7 分类调整
```plaintext
/kb move 1001 | 工作/项目A/API设计
```

返回：
- 分类调整成功
- 新分类路径
- 新文档位置链接

### 5.3.8 软删除与恢复
```plaintext
/kb remove 1001
/kb restore 1001
```

删除返回：
- 已软删除
- 可恢复截止时间

恢复返回：
- 已恢复
- 当前状态 ACTIVE
- 文档位置链接

### 5.3.9 文档修改回收
```plaintext
/kb sync-from-doc 1001
/kb sync-from-doc 文档链接
```

返回：
- 已读取文档修改并同步回数据库
- 新版本号
- 若未识别到修改，返回“未检测到有效差异”

---

## 5.4 错误反馈规范

| 错误码 | 含义 | 建议提示 |
| --- | --- | --- |
| `KB_NOT_FOUND` | 未找到知识 | 请检查知识 ID 或先用 `/kb search` 查询 |
| `CATEGORY_NOT_FOUND` | 分类不存在 | 请重新选择分类路径或先创建分类 |
| `DOC_SYNC_FAILED` | 文档同步失败 | 已保存数据库，文档同步稍后重试 |
| `DOC_BACKFILL_NOT_FOUND` | 未找到文档映射 | 请确认该知识已同步到飞书文档 |
| `DOC_BACKFILL_NO_DIFF` | 未检测到修改差异 | 文档内容与数据库一致，无需同步 |
| `KNOWLEDGE_EXPIRED` | 已超恢复期 | 该知识已超过 30 天恢复窗口，无法恢复 |

---

## 六、推荐开发顺序

### Phase 1
- bot-gateway
- command-router
- draft-service
- knowledge-service
- 基础 categories / knowledge / versions 表

### Phase 2
- ai-classifier
- search-service
- doc-sync-service
- doc_sync_mappings / sync_tasks

### Phase 3
- cleanup-service
- `/kb restore`
- `/kb sync-from-doc`
- 用户部署手册与 Docker Compose

---

## 七、交付建议

本阶段建议最终交付 4 份产物：
1. 后端模块拆分文档
2. PostgreSQL Schema SQL 文件
3. 飞书机器人指令协议文档
4. Docker Compose 部署包 + 用户部署手册
