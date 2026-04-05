<callout background-color="light-blue">
这是一份收敛后的实施附录，用于替代分散的实施文档、API 详细协议、部署手册和测试计划中的重复内容。阅读目标：研发、测试、运维只看这一份，就能进入开工与联调。
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | knowledgeBook MVP 实施附录 |
| 文档版本 | v1.0 |
| 更新时间 | 2026-03-29 |
| 适用对象 | 后端、测试、运维、项目经理 |

---

## 二、推荐实施形态

### 2.1 MVP 部署建议
- 单仓库
- 单主服务 `app-server`
- 单异步 Worker `app-worker`
- PostgreSQL
- Redis
- 飞书机器人
- 飞书文档

### 2.2 推荐模块
| 模块 | 核心职责 |
| --- | --- |
| bot-gateway | 飞书事件接入、验签、回包 |
| command-router | `/kb` 指令解析和路由 |
| draft-service | 候选草稿、确认、待办 |
| knowledge-service | 知识新增、更新、软删除、恢复、版本 |
| category-service | 分类树管理 |
| ai-classifier | 分类推荐与自动分类判定 |
| search-service | 知识检索与答案组装 |
| doc-sync-service | 飞书文档同步、锚点维护、同步回收 |
| cleanup-service | 超期清理与补偿任务 |
| audit-service | 审计日志 |

---

## 三、数据库实施要点

### 3.1 核心表
- `users`
- `categories`
- `knowledge_drafts`
- `knowledge_items`
- `knowledge_versions`
- `ai_category_recommendations`
- `doc_sync_mappings`
- `sync_tasks`
- `operation_logs`

### 3.2 关键规则
- 数据库是唯一事实源
- 所有正式知识必须有版本
- 软删除写入 `removed_at` 与 `purge_at`
- 文档同步必须维护 `doc_sync_mappings`
- 文档回收成功后必须生成新版本

### 3.3 Migration 顺序
```plaintext
M001 users
M002 categories
M003 knowledge_drafts
M004 knowledge_items
M005 knowledge_versions
M006 ai_category_recommendations
M007 doc_sync_mappings
M008 sync_tasks
M009 operation_logs
M010 indexes
M011 seed_categories
```

### 3.4 软删除规则
- 删除后默认不可检索
- 30 天内可恢复
- 超过 30 天自动永久清理

---

## 四、知识沉淀能力实现方案

## 4.1 目标
知识沉淀能力负责把飞书机器人输入的原始内容，转化为可确认、可分类、可版本化、可同步到飞书文档的正式知识。

## 4.2 实现流程
```plaintext
用户输入 / 转发聊天记录
→ bot-gateway 接收消息
→ draft-service 生成候选草稿
→ ai-classifier 输出推荐分类与置信度
→ 用户确认（沉淀 / 忽略 / 稍后处理）
→ knowledge-service 写入正式知识与初始版本
→ category-service 绑定分类路径
→ doc-sync-service 同步到飞书文档
→ search-service 将其纳入检索结果
```

## 4.3 关键实现点
### 4.3.1 候选草稿生成
- 输入统一落入 `knowledge_drafts`
- 字段包含标题、摘要、正文、标签、推荐分类、置信度
- 状态使用：`PENDING_REVIEW / IGNORED / LATER / APPROVED`

### 4.3.2 分类推荐与确认
- AI 输出 TopN 分类建议
- 若满足高置信阈值：
  - `top1 >= 0.85`
  - `top1 - top2 >= 0.15`
  可自动采纳
- 若不满足，必须人工确认
- 用户可手动指定或修改分类路径

### 4.3.3 正式知识入库
- 候选确认后写入 `knowledge_items`
- 同时生成 `knowledge_versions` 的初始版本
- 正式知识默认挂载一个主分类路径 `category_path`
- 所有后续更新必须生成新版本，禁止直接覆盖历史版本

### 4.3.4 稍后处理待办
- `LATER` 状态的草稿进入独立待办列表
- 用户后续可再次执行 approve / ignore

### 4.3.5 文档同步
- `doc-sync-service` 将知识同步到分类子文档
- 写入 `doc_sync_mappings`
- 返回 `doc_link` 与 `doc_anchor_link`

## 4.4 MVP 选型
### 存储选型
- **PostgreSQL**：主数据、分类树、版本、审计、同步任务
- **Redis**：轻量缓存、幂等键、任务协助

### 为什么这样选
- 沉淀链路强依赖事务一致性、版本记录与状态流转
- PostgreSQL 更适合承担“唯一事实源”
- Redis 用于降低重复处理与热点访问压力

## 4.5 非 MVP 不做
- 不做原始聊天追溯回放
- 不做自动监控群聊沉淀
- 不做多人协同编辑

---

## 五、检索能力实现方案与选型

## 5.1 目标
检索能力负责根据用户通过飞书机器人输入的自然语言问题，返回：
- AI 总结答案
- 相关知识条目
- 飞书文档对应位置链接

## 5.2 MVP 检索流程
```plaintext
用户输入检索问题
→ command-router 解析 /kb search
→ search-service 执行检索
→ PostgreSQL 全文检索 + 标签匹配 + 分类过滤
→ 返回候选知识集合
→ 组装答案摘要
→ 返回“答案 + 条目 + 文档位置链接”
```

## 5.3 MVP 实现策略
### 5.3.1 检索源
MVP 仅检索 `knowledge_items` 中状态为 `ACTIVE` 的知识。

### 5.3.2 检索方式
采用三层组合：
1. **全文检索**：基于 PostgreSQL `tsvector`
2. **标签匹配**：优先命中 tags
3. **分类过滤**：按 `category_path` 限定结果范围

### 5.3.3 结果组装
- 先返回命中的知识条目
- 再基于摘要和正文生成简短答案
- 每条结果带 `doc_anchor_link`
- 不返回原始聊天链接

## 5.4 MVP 选型
### 当前选型
- **PostgreSQL Full Text Search** 作为主检索能力
- **不在 MVP 强依赖向量数据库**

### 选型原因
- 当前知识规模在 MVP 阶段可控
- 需求重点是“可用、可解释、可维护”，不是极致语义召回
- PostgreSQL 全文检索 + 标签 + 分类过滤就足以支撑首版
- 这样可以减少额外组件复杂度、部署成本和调优成本

## 5.5 后续增强方向
当知识规模扩大或问法更模糊时，再考虑引入：
- 向量检索
- 重排模型
- 混合召回（全文 + 向量）

## 5.6 检索验收口径
- 仅返回 `ACTIVE` 知识
- 软删除知识不参与默认检索
- 返回结构必须包含文档位置链接
- 查询结果不依赖原始聊天追溯

---

## 六、API 最小集合

## 4.1 飞书接入
- `POST /api/v1/feishu/events`

## 4.2 草稿与确认
- `POST /api/v1/knowledge`
- `POST /api/v1/drafts/{id}/approve`
- `POST /api/v1/drafts/{id}/ignore`
- `POST /api/v1/drafts/{id}/later`
- `GET /api/v1/drafts/later`

## 4.3 知识管理
- `GET /api/v1/knowledge/search`
- `GET /api/v1/knowledge/{id}`
- `PUT /api/v1/knowledge/{id}`
- `POST /api/v1/knowledge/{id}/move-category`
- `DELETE /api/v1/knowledge/{id}`
- `POST /api/v1/knowledge/{id}/restore`
- `POST /api/v1/knowledge/{id}/sync-from-doc`

## 4.4 分类管理
- `GET /api/v1/categories/tree`
- `POST /api/v1/categories`
- `PUT /api/v1/categories/{id}`
- `POST /api/v1/categories/{id}/move`

## 4.5 文档同步
- `POST /api/v1/doc-sync/knowledge/{id}`
- `POST /api/v1/doc-sync/category/{id}`
- `POST /api/v1/doc-sync/rebuild`
- `GET /api/v1/doc-sync/task/{id}`

---

## 五、机器人命令最小集合

| 命令 | 用途 |
| --- | --- |
| `/kb add` | 新增知识 / 候选生成 |
| `/kb review` | 查看候选 |
| `/kb approve` | 确认候选 |
| `/kb ignore` | 忽略候选 |
| `/kb later` | 稍后处理 |
| `/kb later list` | 查看待办 |
| `/kb search` | 检索知识 |
| `/kb update` | 更新知识 |
| `/kb move` | 调整分类 |
| `/kb remove` | 软删除 |
| `/kb restore` | 恢复软删除 |
| `/kb sync-from-doc` | 回收文档手动修改 |
| `/kb category add` | 新增分类 |
| `/kb category rename` | 分类改名 |
| `/kb sync` | 手动触发同步 |

---

## 六、Docker Compose 部署方案

### 6.1 推荐服务
```plaintext
postgres
redis
app-server
app-worker
```

### 6.2 关键环境变量
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

### 6.3 健康检查
- `GET /healthz`
- `GET /readyz`

### 6.4 部署最小命令
```bash
docker compose up -d
docker compose ps
docker compose logs -f app-server
docker compose logs -f app-worker
```

---

## 七、用户部署手册最小步骤

### 7.1 部署前准备
- 一台可运行 Docker / Docker Compose 的服务器
- 飞书自建应用
- 已开通机器人与文档权限

### 7.2 飞书配置
- 创建应用
- 开启机器人能力
- 配置事件订阅：
```plaintext
https://<你的域名>/api/v1/feishu/events
```

### 7.3 启动服务
- 准备 `docker-compose.yml`
- 准备 `.env`
- 启动：`docker compose up -d`

### 7.4 初始化验证
1. `healthz` / `readyz` 成功
2. 飞书 challenge 校验成功
3. 发送 `/kb add ...` 收到候选卡片
4. 发送 `/kb search ...` 收到结果和文档链接

---

## 八、开发任务建议

### 里程碑 M1：基础打通
- 服务骨架
- 数据库 migration
- 飞书回调接入
- 命令路由

### 里程碑 M2：知识链路打通
- 草稿生成
- AI 分类推荐
- 候选确认 / 忽略 / 稍后处理
- 正式知识入库

### 里程碑 M3：文档与检索打通
- 飞书文档同步
- 文档映射
- 检索接口与机器人回包

### 里程碑 M4：维护与交付闭环
- 更新知识
- 软删除 / 恢复 / 清理
- 文档手改同步回收
- Docker Compose 与部署手册
- 联调与验收

---

## 九、测试计划最小集合

### 单元测试
- 命令解析
- 分类路径计算
- 高置信自动分类判定
- 软删除恢复窗口判定

### 集成测试
- 数据库 CRUD
- 版本写入
- 同步任务
- 文档映射
- 文档回收

### 端到端测试
- 输入 → 候选 → 确认 → 文档同步 → 检索
- 更新 → 文档再同步
- 软删除 → 恢复
- 文档手改 → 显式回收
- Docker Compose 部署验证

### 核心验收
- [ ] 检索返回“答案 + 条目 + 文档位置链接”
- [ ] 软删除后默认不可检索
- [ ] 30 天内可恢复
- [ ] 文档手改不自动回写，显式回收后生成新版本
- [ ] 部署手册可指导用户独立完成启动

---

## 十、建议停止维护的旧文档类型

为减少后续阅读成本，建议不再并行维护以下旧类型文档：
- 过度展开的单独 PRD / 设计 / 技术修订版
- 单独的 API 文档
- 单独的测试计划文档
- 单独的部署手册文档

推荐后续只维护两份：
1. **统一总方案**
2. **实施附录**

---

## 十一、推荐阅读方式

### 给产品 / 设计 / 管理者
先读：
1. 统一总方案

### 给研发 / 测试 / 运维
再读：
2. 实施附录
