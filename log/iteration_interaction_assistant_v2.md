<callout background-color="light-blue">
本文件是当前轮次的唯一迭代技术方案文档，用于承接“知识沉淀助手”交互层升级的讨论、收敛与后续实现。

本轮原则：
1. 先持续更新这一份迭代文档
2. 讨论完成后再编码
3. 发版后对本迭代文档归档
4. 下一轮再创建新的版本文档
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | knowledgeBook 交互层迭代技术方案（知识沉淀助手 v2） |
| 迭代版本号 | v2.0 |
| 更新时间 | 2026-04-22 |
| 当前状态 | 已完成，归档 |
| 关联文档 | `knowledgeBook/log/prd_delta_llm_interaction_v1.md`、`knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md`、`knowledgeBook/log/iteration_impl_detail_llm_interaction_v1.md` |
| 当前用途 | 作为本轮唯一迭代文档持续维护，承接讨论、收敛、实现与发版前校对 |

---

## 二、版本管理流程

本项目后续按以下简单流程管理迭代版本：

1. 每次功能迭代先定义一个明确版本号，例如 `v2.0-draft`
2. 该版本的所有讨论内容只维护在一份迭代文档中
3. 随着讨论推进持续更新、优化、删繁就简，保持结构清晰
4. 待方案确认后再进入编码
5. 代码完成并发版后，将该文档归档
6. 下一轮功能迭代创建新的版本文档，循环往复

### 当前建议
- 当前轮次正式版本号：`v2.0-draft`
- 当前轮次唯一迭代文档：`knowledgeBook/log/iteration_interaction_assistant_v2.md`

---

## 三、本轮目标

本轮不是继续增强命令式机器人，而是将 knowledgeBook 的 LLM 交互层升级为一个真正的“知识沉淀助手”。

### 目标定位
机器人应具备明确角色：
- 帮助用户通过自然语言完成知识沉淀
- 帮助用户通过自然语言完成知识检索
- 提炼和优化知识内容
- 通过 MCP 工具完成受控执行
- 关键写入操作必须确认

### 用户体验目标
用户不需要记忆或输入任何机器命令。主流程仅依赖自然语言与飞书卡片交互即可完成：
- 新增知识
- 确认保存
- 拒绝保存
- 修改分类
- 查询知识
- 相似/冲突判断

---

## 四、当前现状与问题

### 4.1 当前实现现状
当前自然语言交互只覆盖：
- `create_knowledge`
- `search_knowledge`
- `check_similarity`
- `clarify`

当前 LLM 更像受控子任务处理器，而不是完整交互层助手：
- 负责 intent parser
- 负责 draft extractor
- 负责 similarity judge
- 负责 answer composer

### 4.2 当前主要问题
当前系统还无法完成完整自然语言知识沉淀闭环，主要缺口包括：

1. 没有 `approve / reject / change_category` 的自然语言意图
2. 没有待确认草稿上下文
3. 没有草稿 TTL / 过期机制
4. 没有飞书卡片确认交互闭环
5. 没有 reply / quote / message context 感知
6. LLM 还没有通过统一 MCP 边界完成全部受控交互能力

### 4.3 现象示例
当前用户说：
- “确认保存”
- “就按这个存”
- “这条不要”
- “改到软件开发/接口治理”

机器人无法稳定理解，因为当前并不知道：
- 用户在确认哪条草稿
- 用户是否引用了上一条消息
- 当前 chat 中是否存在唯一待确认草稿

---

## 五、角色与边界设计

## 5.1 LLM 的角色定义
LLM 的产品角色明确为：

> knowledgeBook 的私人知识沉淀助手

它负责：
- 理解用户意图
- 提炼和优化知识表达
- 发现重复、补充与冲突关系
- 组织最终答复
- 通过 MCP 工具完成受控执行

它不负责：
- 直接写库
- 在未确认时执行正式保存
- 越过后端状态机直接修改系统状态

## 5.2 MCP 的角色定义
后端所有交互能力统一封装在一个 embedded MCP server 中，方便维护。

### 原则
- 只有一个 MCP server 边界
- 该 MCP server 下可包含多个细粒度 tools
- LLM 只能通过这一层访问受控能力
- 不再让 LLM 直接依赖内部 service 方法语义

### 结论
本轮采用：
> 一个 embedded MCP server + 多个细粒度 tools

而不是：
> 多个并行执行边界，或一个巨大的万能工具

---

## 六、目标交互流程

## 6.1 新增知识
### 用户输入
“帮我记一下，飞书事件验签生产环境还没补完，需要补测试和回归”

### 系统流程
1. 识别意图为 `create_knowledge`
2. 提取结构化草稿
3. 召回相似知识并判断关系
4. 创建 `PENDING_CONFIRMATION` 草稿
5. 记录该用户在当前 chat 的待确认上下文
6. 返回确认卡片

### 返回内容
- 标题
- 摘要
- 要点
- 推荐分类
- 相似知识提示
- 有效期提示
- 操作按钮

---

## 6.2 确认保存
### 用户表达
- “确认保存”
- “可以，就按这个存”
- “好，保存吧”
- 点击卡片“确认保存”

### 系统流程
1. 识别为 `approve_pending_draft`
2. 解析当前待确认对象
3. 如果存在唯一 pending 草稿，则直接确认
4. 如果存在多条 pending 草稿，则先澄清
5. 调用 MCP `confirm_knowledge_draft`
6. 返回已保存卡片

---

## 6.3 拒绝保存
### 用户表达
- “不要保存”
- “丢弃这条”
- “算了”
- 点击卡片“拒绝保存”

### 系统流程
1. 识别为 `reject_pending_draft`
2. 命中当前 pending 草稿
3. 更新状态为 `REJECTED`
4. 返回已丢弃提示

---

## 6.4 修改分类
### 用户表达
- “改到 软件开发/接口治理”
- “分类不对，放到 知识管理/飞书接入”
- 点击卡片“修改分类”

### 系统流程
1. 识别为 `change_pending_draft_category`
2. 命中当前 pending 草稿
3. 更新草稿的候选分类或正式分类路径
4. 再次返回确认卡片

### 第一版策略
先使用“完整路径候选 + 用户直接自然语言改路径”的轻量方案，不做复杂级联树。

---

## 6.5 查询知识
### 用户表达
- “查一下之前关于飞书验签的结论”
- “我们之前怎么说飞书同步的”

### 系统流程
1. 识别 `search_knowledge`
2. 检索 evidence
3. answer composer 组织答案
4. 返回 `answer + evidence + doc link`

---

## 6.6 相似/冲突判断
### 用户表达
- “这条和之前那条是不是重复”
- “这条是不是和飞书接入方案冲突”

### 系统流程
1. 识别 `check_similarity`
2. DB 先召回候选
3. LLM 判断 relation_type
4. 返回 merge/supplement/conflict/new 的建议

---

## 6.7 草稿过期
### 规则
- 草稿进入 `PENDING_CONFIRMATION` 后默认 1 小时失效
- TTL 可配置

### 本轮调整
用户补充要求：
- 草稿过期检测改为每小时批处理一次
- 到期前做提醒

### 建议实现
- `expires_at = created_at + 60m`
- worker 每小时跑一次
- 扫描即将过期草稿并提醒
- 对已过期草稿更新为 `EXPIRED`
- 用户视角视为“已丢弃”，但数据保留审计记录

### 说明
这种方案更偏批处理，易于维护，但过期和提醒时间粒度会比分钟级粗。

---

## 七、状态机方案

建议统一草稿状态为：
- `PENDING_CONFIRMATION`
- `APPROVED`
- `REJECTED`
- `EXPIRED`
- `DEFERRED`（可选，若保留“稍后处理”）

### 状态迁移
- 创建草稿：`NONE -> PENDING_CONFIRMATION`
- 用户确认：`PENDING_CONFIRMATION -> APPROVED`
- 用户拒绝：`PENDING_CONFIRMATION -> REJECTED`
- 用户稍后处理：`PENDING_CONFIRMATION -> DEFERRED`
- 超时：`PENDING_CONFIRMATION -> EXPIRED`

---

## 八、数据结构方案

## 8.1 总体选择
本轮建议不新增独立会话表，优先扩展 `knowledge_drafts`，因为当前交互对象天然围绕草稿。

## 8.2 `knowledge_drafts` 建议新增字段
- `chat_id` TEXT
- `source_message_id` TEXT
- `reply_to_message_id` TEXT
- `card_message_id` TEXT
- `expires_at` TIMESTAMPTZ
- `resolved_at` TIMESTAMPTZ
- `last_reminded_at` TIMESTAMPTZ
- `interaction_context` JSONB NOT NULL DEFAULT '{}'::jsonb

### 字段作用
- `chat_id`：按 `user + chat` 命中当前 pending 草稿
- `source_message_id`：绑定创建草稿的原始消息
- `reply_to_message_id`：支持引用上下文
- `card_message_id`：后续更新确认卡片
- `expires_at`：草稿 TTL 核心字段
- `resolved_at`：确认/拒绝/过期完成时间
- `last_reminded_at`：避免重复提醒
- `interaction_context`：保存上下文、候选分类、相似结果、intent 快照等

## 8.3 索引建议
新增：
- `(user_id, chat_id, status, created_at desc)`
- `(status, expires_at)`
- `(source_message_id)`
- `(reply_to_message_id)`

---

## 九、飞书卡片方案

## 9.1 草稿确认卡片
建议展示：
- 标题
- 摘要
- 关键点
- 推荐分类
- 候选分类
- 相似知识提示
- “1 小时内确认，否则自动失效”提示

按钮：
- 确认保存
- 拒绝保存
- 修改分类

## 9.2 分类选择卡片
第一版不做复杂树选择，只展示：
- 当前推荐分类
- 3~5 个完整路径候选
- 文本修改分类的提示

## 9.3 卡片与文本双通道一致性
无论用户点击卡片还是发自然语言，底层都应收敛到同一套状态动作：
- `approve_pending_draft`
- `reject_pending_draft`
- `change_pending_draft_category`

---

## 十、上下文感知方案

## 10.1 上下文范围
已确认按：
> `user + chat`

而不是用户全局。

## 10.2 第一版上下文输入
每次 LLM 任务都不只接收一句文本，而是接收结构化上下文：
- 当前消息文本
- `chat_id`
- `message_id`
- `reply_to_message_id`
- `quoted_text`
- 当前 pending draft
- 候选分类
- 相似候选

## 10.3 命中规则
### 若有卡片回调
优先用卡片 payload 中的 draft_id 绑定对象。

### 若是纯文本确认
优先匹配：
1. 当前 chat 中唯一 pending 草稿
2. 若多条 pending，则必须澄清，不猜

### 若有引用消息
优先通过 `reply_to_message_id` / 引用消息命中对应草稿

---

## 十一、LLM 方案

## 11.1 角色定位
LLM 不是命令翻译器，而是“私人知识沉淀助手”。

### 职责
- 理解用户意图
- 优化知识表达
- 通过统一 MCP 边界调用后端能力
- 组织清晰、受控、可读的回复

## 11.2 运行方式
LLM 继续使用受控动作规划模式：
1. 输出结构化 action / intent / slots
2. 后端状态机判断是否允许执行
3. 后端调用 MCP tool
4. LLM 组织最终文本或卡片内容

本轮暂不做完全自由的 tool-calling loop。

## 11.3 LLM 配置原则
要让 LLM 成为合格的私人助手，不能只靠一个 prompt，而是需要一整套配置：

### A. System Role
明确写清：
- 你是知识沉淀助手
- 目标是帮助用户仅靠自然语言完成知识沉淀与检索
- 正式写入必须确认
- 遇到歧义先澄清

### B. Task Prompt
至少包括：
- intent-parser
- knowledge-extractor
- similarity-judge
- answer-composer
- 后续还要补：approve/reject/category-change 相关 action 解析

### C. Context Package
每次调用都应带上结构化上下文，而不是只传一句文本。

### D. MCP Policy
LLM 只能通过一个 embedded MCP server 完成受控操作。

### E. Confirmation Policy
- 新增知识必须确认
- 多条 pending 草稿冲突时必须澄清
- 不允许隐式越权执行保存

### F. Response Policy
- 默认使用自然语言回复
- 关键行为优先卡片交互
- 命令仅作为 fallback，不作为主路径

## 11.4 模型参数建议
当前继续采用单模型方案即可。

建议参数：
- intent parser：temperature 0
- knowledge extractor：temperature 0.2
- similarity judge：temperature 0
- answer composer：temperature 0.2

---

## 十二、MCP 方案

## 12.1 总体原则
用户补充要求：
> LLM 与后端所有交互能力都封装在一个 MCP 里即可，方便维护

本轮建议的解释为：
- 只有一个 embedded MCP server
- 所有后端交互能力统一挂在该 MCP server 下
- 但保留多个细粒度 tool，避免一个过大的万能工具

## 12.2 当前已有工具
- `create_knowledge_draft`
- `get_knowledge_draft`
- `confirm_knowledge_draft`
- `check_similarity`
- `get_similarity_candidates`
- `search_knowledge`
- `get_knowledge`
- `get_related_knowledge`

## 12.3 建议新增工具
- `reject_knowledge_draft`
- `update_draft_category`
- `get_pending_draft_context`
- `list_pending_drafts`
- `expire_pending_drafts`

## 12.4 最终边界
后续交互层所有真实执行能力都应通过这个统一 MCP server 完成，而不是继续扩散到 ad hoc service 调用。

---

## 十三、测试方案

## 13.1 单元测试
- approve/reject/category_change 意图解析
- pending 草稿命中逻辑
- 多条 pending 冲突澄清
- TTL 过期与提醒逻辑
- 分类修改逻辑

## 13.2 集成测试
- create -> card confirm -> approve
- create -> reject
- create -> change category -> approve
- create -> timeout -> expire
- search -> answer + evidence
- card callback -> 状态更新

## 13.3 回归测试
至少确认以下链路不被破坏：
- `/kb search`
- `/kb approve`
- `/mcp`
- 已有 create/search/similarity 能力

---

## 十四、完整最终方案

### 14.1 本轮最终目标
本轮目标不是继续增强 `/kb` 命令，而是把当前系统升级为一个真正的“私人知识沉淀助手”。

用户应仅通过自然语言 + 飞书卡片完成：
- 新增知识
- 确认保存
- 拒绝保存
- 修改分类
- 查询知识
- 相似/冲突判断
- 在待确认草稿上下文里继续交互

核心约束：
- 正式写入必须显式确认
- LLM 不直接写库
- 所有真实执行能力统一走一个 embedded MCP server
- 歧义场景不猜，必须澄清
- 上下文作用域按 `user + chat`

### 14.2 整体架构结论
本轮最终采用 5 层结构：
1. 飞书接入层：接收消息事件、卡片回调，解析上下文并发送文本/卡片
2. LLM 交互层：负责意图理解、草稿提取、上下文感知、action 规划、澄清判断、回复组织
3. MCP 执行层：统一暴露 embedded MCP server，承载所有受控 tools
4. 业务状态机层：负责 draft 状态流转、确认/拒绝/改分类/过期的合法性控制
5. 数据层：负责 draft、knowledge、similarity、sync task 等持久化

### 14.3 数据库最终方案
#### `knowledge_drafts` 保留字段
继续保留当前基础字段与 LLM 结构化字段：
- `input_type`
- `input_text`
- `title`
- `summary`
- `content_markdown`
- `tags`
- `recommended_category_path`
- `recommendation_confidence`
- `auto_accepted_category`
- `status`
- `reviewed_at`
- `raw_content`
- `normalized_title`
- `normalized_summary`
- `normalized_points`
- `llm_confidence`

#### `knowledge_drafts` 本轮最终新增字段
- `chat_id` TEXT
- `source_message_id` TEXT
- `reply_to_message_id` TEXT
- `card_message_id` TEXT
- `expires_at` TIMESTAMPTZ
- `resolved_at` TIMESTAMPTZ
- `last_reminded_at` TIMESTAMPTZ
- `interaction_context` JSONB NOT NULL DEFAULT '{}'::jsonb

#### 字段作用
- `chat_id`：用于 `user + chat` 范围内命中当前 pending draft
- `source_message_id`：绑定创建草稿的原始消息
- `reply_to_message_id`：支持 reply / quote 命中对应草稿
- `card_message_id`：保存确认卡片消息 id，后续用于更新卡片
- `expires_at`：草稿 TTL 主字段
- `resolved_at`：记录 approve / reject / expire 的最终完成时间
- `last_reminded_at`：防止 reminder 重复发送
- `interaction_context`：保存上下文快照、候选分类、相似候选、intent 快照、卡片状态等

#### `reviewed_at` 处理结论
本轮保留 `reviewed_at` 作为兼容字段；新逻辑主用 `resolved_at`，但在 approve/reject/expire 时同步更新 `reviewed_at`。

#### draft 状态最终清单
统一为：
- `PENDING_CONFIRMATION`
- `APPROVED`
- `REJECTED`
- `EXPIRED`

旧状态迁移建议：
- `PENDING_REVIEW` → `PENDING_CONFIRMATION`
- `IGNORED` → `REJECTED`
- `LATER` 不再作为本轮主流程状态保留

#### 索引最终方案
必须新增：
- `(user_id, chat_id, status, created_at DESC)`
- `(status, expires_at)`
- `(source_message_id)`
- `(reply_to_message_id)`

建议新增：
- `(card_message_id)`

### 14.4 MCP 最终方案
#### 保留工具
- `create_knowledge_draft`
- `get_knowledge_draft`
- `confirm_knowledge_draft`
- `check_similarity`
- `get_similarity_candidates`
- `search_knowledge`
- `get_knowledge`
- `get_related_knowledge`

#### 本轮新增工具
- `reject_knowledge_draft`
- `update_draft_category`
- `get_pending_draft_context`
- `list_pending_drafts`
- `expire_pending_drafts`

#### 各工具职责
- `create_knowledge_draft`：创建 pending draft，保存结构化抽取结果与上下文信息
- `get_knowledge_draft`：读取 draft 详情
- `confirm_knowledge_draft`：确认 draft，正式创建 knowledge item，并将状态置为 `APPROVED`
- `reject_knowledge_draft`：拒绝待确认草稿，状态置为 `REJECTED`
- `update_draft_category`：修改待确认草稿分类，不正式写入 knowledge item
- `get_pending_draft_context`：解析当前文本交互所指向的 draft，是 approve/reject/change-category 的统一前置工具
- `list_pending_drafts`：当同一 chat 存在多条 pending 时列出候选，供澄清使用
- `expire_pending_drafts`：worker/cron 批量过期，并可配套 reminder
- `check_similarity`：判断新内容或 draft 与已有知识的相似/冲突关系
- `get_similarity_candidates`：读取已落库的 similarity 结果
- `search_knowledge`：检索知识并返回 answer + evidence
- `get_knowledge`：读取知识详情
- `get_related_knowledge`：读取关联知识

#### MCP schema 原则
- 所有需要定位当前上下文的工具，都应支持：`chatId`、`messageId`、`replyToMessageId`、`quotedText`
- 执行类工具优先直接接收 `draftId`
- 纯文本 approve/reject/category-change 不直接执行，必须先经过 `get_pending_draft_context`

### 14.5 LLM 交互层完整改造方案
#### 最终定位
LLM 在 v2.0 中不再只是意图解析器，而是 knowledgeBook 的“私人知识沉淀助手交互编排层”。

它负责：
- 理解用户自然语言
- 提炼知识草稿
- 判断当前上下文
- 决定下一步动作
- 判断是否需要澄清
- 组织最终文本与卡片内容

它不负责：
- 直接写数据库
- 直接越权写入正式知识
- 绕过状态机做非法流转

#### 最终职责拆分
1. Intent Parser：至少支持
   - `create_knowledge`
   - `approve_pending_draft`
   - `reject_pending_draft`
   - `change_pending_draft_category`
   - `search_knowledge`
   - `check_similarity`
   - `clarify`
2. Draft Extractor：抽取 `title` / `summary` / `key_points` / `tags` / `category_hint` / `confidence`
3. Context Resolver：基于 `chat_id`、`message_id`、`reply_to_message_id`、`quoted_text`、当前 pending draft、候选分类和相似候选，判断是否命中草稿、命中方式、是否需要澄清
4. Action Planner：规划是直接创建草稿、先解析上下文再 confirm/reject/change category、直接 search、直接 similarity check，还是进入 clarify
5. Answer Composer：负责文本回复、卡片文案、相似提示、过期提示和澄清文案

#### 运行模式
本轮不做完全自由 tool-calling loop，继续采用“受控 action planning 模式”：
1. 输入消息 + 结构化上下文包
2. LLM 输出 `intent / action / slots / response_mode`
3. 后端按 action 决定调用 MCP tool
4. tool 返回结构化结果
5. LLM 或模板生成最终文本 / 卡片

#### LLM 输入上下文包
每次调用 LLM，都传统一上下文包，而不是只传一句文本：
- `current_message_text`
- `chat_id`
- `message_id`
- `reply_to_message_id`
- `quoted_text`
- `pending_draft`
- `pending_draft_count`
- `candidate_categories`
- `similarity_candidates`
- `recent_action`
- `channel = feishu`

#### approve / reject / change-category 行为规则
- approve：识别 `approve_pending_draft` → 调 `get_pending_draft_context` → 唯一命中则 `confirm_knowledge_draft`，多条 pending 则澄清
- reject：识别 `reject_pending_draft` → 调 `get_pending_draft_context` → 唯一命中则 `reject_knowledge_draft`，多条 pending 则澄清
- change-category：识别 `change_pending_draft_category` → 抽取分类路径 → 调 `get_pending_draft_context` → 唯一命中则 `update_draft_category` → 返回更新后的确认卡片

#### 澄清策略
以下情况必须澄清：
- 当前 chat 中有多条 pending 草稿
- 用户说“保存吧”但当前没有 pending 草稿
- 用户要改分类但没有给出明确分类路径
- 相似/冲突结果不明确，需要人工确认

原则：不猜、不隐式写入、澄清优先于误操作。

#### 输出协议建议
LLM 输出不只返回 intent，而是统一结构：
- `intent`
- `confidence`
- `needs_clarification`
- `action`
- `slots`
- `response_mode`

其中 `action` 建议支持：
- `create_draft`
- `resolve_pending_then_confirm`
- `resolve_pending_then_reject`
- `resolve_pending_then_change_category`
- `search_knowledge`
- `check_similarity`
- `clarify`

`response_mode` 建议支持：
- `text`
- `card`
- `text_and_card`

#### prompt / policy 原则
- System Role：你是知识沉淀助手；正式保存必须确认；歧义必须澄清；不得隐式执行写入
- MCP Policy：只能通过统一 MCP server 调后端能力
- Confirmation Policy：新增知识必须确认；多条 pending 必须澄清；无 pending 不得默认猜对象
- Response Policy：默认自然语言；确认类动作优先卡片；命令只是 fallback

### 14.6 飞书消息 / 卡片最终方案
#### 飞书消息接入改造
当前接入层只拿到 `open_id`、`message_id`、`text`。本轮要求尽可能解析并纳入统一上下文包：
- `chat_id`
- `message_id`
- `reply_to_message_id`
- `quoted_text`
- `message_type`
- `chat_type`

若 payload 中存在则直接解析；若暂时拿不到则允许为空，但接入结构要预留。

#### 草稿确认卡片
卡片展示：
- 标题
- 摘要
- 关键点
- 推荐分类
- 候选分类（3~5 个）
- 相似知识提示
- TTL 提示：“1 小时内确认，否则自动失效”

按钮：
- 确认保存
- 拒绝保存
- 修改分类

卡片 payload 必带：
- `draft_id`
- `action`
- `chat_id`
- `card_version`

#### 分类修改卡片
第一版不做级联树，只展示：
- 当前推荐分类
- 3~5 个完整路径候选
- 文本修改分类提示

最终策略：卡片做轻量候选，复杂路径继续走自然语言输入。

#### 文本与卡片双通道一致性
无论用户通过文本还是卡片触发操作，底层都收敛到同一组动作：
- approve
- reject
- category-change

### 14.7 上下文命中机制
#### 命中优先级
1. 卡片 payload 自带 `draft_id`
2. 通过 `reply_to_message_id` / `source_message_id` 命中草稿
3. 当前 `user + chat` 中唯一 pending 草稿
4. 若有多条 pending，则必须澄清

#### `get_pending_draft_context` 输出职责
统一输出：
- 是否命中
- 命中方式
- 当前 pending 数量
- 候选草稿列表
- 是否允许直接执行

### 14.8 TTL / reminder 最终方案
- draft 进入 `PENDING_CONFIRMATION` 后默认 1 小时失效
- `expires_at = created_at + ttl`
- 采用批处理，不做分钟级实时轮询
- worker 每小时跑一次，处理 reminder 与 expire
- reminder 规则建议为：当 `expires_at - now <= 15m` 且 `last_reminded_at is null` 时发送提醒
- 过期规则：满足 `status = PENDING_CONFIRMATION` 且 `expires_at <= now` 时，更新为 `EXPIRED`，并写入 `resolved_at` 与 `reviewed_at`

---

## 十五、实现顺序建议

### Step 1
数据库与状态机收敛：扩 `knowledge_drafts` 字段、统一状态、补索引

### Step 2
Repository + Service 基础能力：补 `get_pending_draft_context`、`reject_knowledge_draft`、`update_draft_category`、expire / reminder 基础逻辑

### Step 3
MCP 工具补齐：新增 5 个 tools，并扩展 schema 到 chat/message 上下文

### Step 4
LLM 交互层改造：扩 intent schema、action、context package、clarify 逻辑、response mode

### Step 5
飞书卡片闭环：生成确认卡片、接卡片回调、更新卡片状态

### Step 6
TTL + reminder worker：补 reminder 与 expire 批处理

### Step 7
回归验证：确保 `/kb search`、`/kb approve`、`/mcp`、create/search/similarity 旧能力不被破坏

---

## 十六、当前确认结论

当前讨论后，已最终确认如下：

1. 当前轮次唯一迭代文档采用版本化维护，当前版本号为 `v2.0-draft`
2. 交互层目标不是命令翻译器，而是自然语言优先的知识沉淀助手
3. 关键写入行为必须确认
4. `knowledge_drafts` 扩字段方案最终明确
5. draft 状态统一收敛为 `PENDING_CONFIRMATION / APPROVED / REJECTED / EXPIRED`
6. 文本确认与卡片确认并行、底层动作统一收敛
7. MCP 工具最终补齐方案已明确
8. LLM 交互层将从意图解析器升级为交互编排层，并补齐 approve/reject/category-change/context/clarify/response mode
9. 飞书消息侧需补 chat / reply / quote 上下文，卡片需补 payload 与回调闭环
10. 草稿过期检测采用每小时批处理，并配套 reminder 机制
11. LLM 与后端所有交互能力统一封装在一个 embedded MCP server 边界内

---

## 十七、进入实现阶段的前提

本轮方案已收敛完成，可正式进入实现。实现阶段应严格遵循：
- 先完成数据库与状态机收敛
- 再补 repository / service / MCP 基础能力
- 再完成 LLM 交互层改造
- 最后接飞书卡片、TTL worker 与整体验证

发版前需至少完成：
- 编译验证
- MCP 回归验证
- create/search/similarity 回归验证
- approve/reject/change-category 主链路验证
- 飞书文本与卡片交互验证

---

## 十八、V2 迭代完成总结

> 本轮迭代已于 2026-04-22 完成全部功能实现与部署验证，文档归档。

### 已完成 Gap 清单

| # | Gap | 状态 |
|---|---|---|
| 1 | approve/reject/change_category 自然语言意图 + LLM context-aware intent parsing | 已完成 |
| 2 | 待确认草稿上下文：user+chat 范围 pending draft 解析、多条澄清 | 已完成 |
| 3 | 草稿 TTL / 过期 / 提醒机制（1h TTL, 15min reminder, 30s worker tick） | 已完成 |
| 4 | 飞书卡片确认交互闭环：confirm/reject/change_category 按钮 + PatchCard 状态更新 | 已完成 |
| 5 | reply / quote / message context 感知 + 引用命中草稿 | 已完成 |
| 6 | 统一 embedded MCP server 补齐 5 个新 tools | 已完成 |
| 7 | 候选分类推荐（LLM hint + 关键词规则 + 用户常用路径） | 已完成 |

### 本轮额外完成

| 项目 | 说明 |
|---|---|
| 飞书 WebSocket 长连接迁移 | HTTP 回调 → WS 长连接，解决无公网 IP 问题；vendor patch SDK MessageTypeCard 支持 |
| 卡片过期自动更新 | 草稿过期时自动 PatchCard 为"已过期失效"样式；点击已过期卡片也触发更新 |
| vendor 构建模式 | Dockerfile 改为 COPY vendor + go build -mod=vendor，解决 Docker 中依赖下载问题 |

### 关键文件变更

| 文件 | 改动说明 |
|---|---|
| `go.mod` / `go.sum` | 新增 larksuite/oapi-sdk-go/v3 + 传递依赖 |
| `internal/api/handlers.go` | 提取 processMessageEvent/processCardAction 共享逻辑；卡片过期时也更新样式 |
| `internal/api/ws_handlers.go` | **新增** WebSocket 事件/卡片回调适配层 |
| `internal/api/mcp.go` | MCP tool 注册调整 |
| `internal/feishu/wsconn.go` | **新增** WSClient + SDK 事件→领域类型适配 |
| `internal/feishu/messenger.go` | 新增 BuildDraftCardJSON / BuildResolvedCardJSON / PatchCard；支持 expired 状态 |
| `internal/config/config.go` | 新增 FeishuWSEnabled 配置项 |
| `internal/repository/store.go` | 新增 ExpiredDraftInfo / CountPendingDraftsByChat / ListTopCategoryPaths 等 |
| `internal/service/conversation_service.go` | LLM context-aware intent、候选分类推荐、ResolvePendingDraftContext 增强 |
| `internal/service/llm_tasks.go` | 新增 parseIntentWithContext + ConversationContext |
| `internal/service/services.go` | ExpirePendingDrafts 自动 PatchCard；RemindPendingDrafts 逻辑 |
| `cmd/app-server/main.go` | 条件启动 WS 长连接客户端 |
| `cmd/app-worker/main.go` | worker tick 间隔调整 |
| `deploy/Dockerfile.server` / `Dockerfile.worker` | vendor 构建模式 |
| `deploy/.env` | 新增 FEISHU_WS_ENABLED=true |
| `vendor/` | 新增 oapi-sdk-go/v3 + gorilla/websocket + gogo/protobuf；SDK WS client.go 补丁 |
