<callout background-color="light-blue">
本文件是当前轮次的唯一迭代技术方案文档，用于承接"知识沉淀助手"V3 交互层升级的讨论、收敛与后续实现。

本轮原则：
1. 先持续更新这一份迭代文档
2. 讨论完成后再编码
3. 发版后对本迭代文档归档
4. 下一轮再创建新的版本文档
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | knowledgeBook 交互层迭代技术方案（知识沉淀助手 v3） |
| 迭代版本号 | v3.0-draft |
| 更新时间 | 2026-04-22 |
| 当前状态 | 编码完成，部署验证中 |
| 关联文档 | `knowledgeBook/log/iteration_interaction_assistant_v2.md`（已归档） |
| 当前用途 | 作为本轮唯一迭代文档持续维护 |

---

## 二、本轮目标

### 用户痛点
> "小助手智能太低，沟通更像是跟一个固定程序进行交互，助手根本不理解正在聊的内容是什么"

### 目标定位
将 bot 从"受控意图分发器 + 模板回复"升级为**多轮 Tool-Calling Agent**：
- LLM 维护对话历史，理解上下文
- LLM 自主决定何时调用工具（而非 Go switch-case 分发）
- LLM 直接生成用户可见的自然语言回复（而非 `fmt.Sprintf` 模板拼接）
- 用户体验像和一个懂知识管理的朋友聊天

### 本轮不做
- 联网搜索（留到后续版本）
- 向量/Embedding 检索（继续用 PostgreSQL tsvector）
- 模型切换（继续用 Qwen3-Max）

---

## 三、V2 现状与问题

### 3.1 当前架构
```
用户消息 → parseIntent()（规则解析） → LLM intent_parser（可被规则否决）
         → switch-case 7 个意图 → 调 service 方法 → fmt.Sprintf 拼接回复
```

### 3.2 核心问题（12 项）

1. **LLM 从不生成用户可见文本** — 所有回复都是 Go 模板拼接
2. **无对话历史** — 每次 LLM 调用都是单轮无状态的
3. **`system.md` 定义了人格但从未发送到任何 LLM 调用** — 死代码
4. **刚性 switch-case** — 7 个 intent，其余全部返回静态错误
5. **规则解析器 `parseIntent()` 可否决 LLM** — `shouldPreferIntentFallback()` 经常覆盖 LLM 判断
6. **无通用聊天能力** — 问候、感谢、跟进全部返回 "我还没完全理解你的意思"
7. **MCP tools 注册了 13 个但 bot LLM 从未调用** — Go 代码直接调 service
8. **无 streaming** — 暂不处理
9. **similarity 是串行 N+1** — 每个候选一次 LLM 调用
10. **knowledge extractor / answer composer prompt 过于精简** — 各 5-10 行
11. **LLM 输出被强制为 JSON** — `response_format: json_object`，无法自然回复
12. **intent schema 限死 7 个值** — 任何新场景都需要改代码

---

## 四、V3 架构方案

### 4.1 总体架构

```
用户消息 → Agent.Run()
         → 加载 Redis 对话历史
         → [system_prompt + history + user_message] 发给 LLM
         → LLM 决定：
           a) 直接回复（finish_reason=stop）→ 自然语言回复
           b) 调工具（finish_reason=tool_calls）→ Executor 执行 → 结果传回 LLM → 循环
         → 最终回复保存到 Redis 历史
         → 返回回复文本 + 可选卡片
```

### 4.2 用户决策

| 决策项 | 选择 |
|---|---|
| 架构模式 | 多轮 Tool-Calling Agent |
| 模型 | 继续 Qwen3-Max |
| 联网搜索 | V3 不做 |
| 对话历史存储 | Redis 短期缓存 |

### 4.3 降级策略

Agent 失败（LLM 不可用、超时等）时，自动回退到 V2 的 switch-case 路径，保证基本功能不中断。

---

## 五、详细设计

### 5.1 Redis 对话历史

**文件：** `internal/conversation/history.go`

- Key 格式：`kb:conv:{userID}:{chatID}`（私聊无 chatID 时用 `direct`）
- 数据结构：Redis LIST，每条是 JSON 序列化的 Message
- 容量：最近 20 条消息（约 10 轮对话），LTRIM 自动截断
- TTL：2 小时，每次写入重置
- 操作：`Append`（RPUSH + LTRIM + EXPIRE）、`Load`（LRANGE）、`Clear`（DEL）

**配置项：**
- `CONV_HISTORY_MAX_MESSAGES`：默认 20
- `CONV_HISTORY_TTL_MINUTES`：默认 120

### 5.2 LLM Client 升级

**文件：** `internal/llm/types.go`、`internal/llm/client.go`

**新增类型：**
- `ChatMessage`：支持 role / content / tool_call_id / tool_calls
- `ToolCall` / `FunctionCall`：工具调用描述
- `ToolDefinition` / `ToolFunctionDef`：工具定义
- `ChatRequest` / `ChatResponse`：多轮对话请求/响应

**新增方法：** `Client.Chat(ctx, ChatRequest) (*ChatResponse, error)`
- OpenAI-compatible 请求格式（messages + tools + tool_choice: "auto"）
- 不使用 `response_format: json_object`（工具调用模式不需要）
- 解析 `finish_reason`：`"stop"` 表示文本回复，`"tool_calls"` 表示需要执行工具

**向后兼容：** `GenerateJSON` 保持不变，worker 中的 extractor / similarity judge 继续用它。

### 5.3 Tool 定义

**文件：** `internal/conversation/tools.go`

7 个工具，描述面向 LLM（中文自然语言），参数只暴露业务字段：

| 工具名 | 说明 | 对应 service 方法 |
|---|---|---|
| `create_knowledge_draft` | 创建知识草稿 | `Services.CreateDraft` |
| `confirm_knowledge_draft` | 确认保存草稿 | `Services.ApproveDraft` |
| `reject_knowledge_draft` | 拒绝草稿 | `Services.RejectDraft` |
| `update_draft_category` | 修改草稿分类 | `Services.UpdateDraftCategory` |
| `search_knowledge` | 搜索知识 | `Services.SearchAnswer` |
| `check_similarity` | 检查相似/冲突 | `Services.CheckSimilarity` |
| `list_pending_drafts` | 列出待确认草稿 | `Services.ListPendingDrafts` |
| `revise_knowledge_draft` | 修订草稿内容（废弃旧草稿+创建新） | `Services.ReviseDraftForAgent`（组合操作） |

`openId` / `userName` / `chatId` 等会话上下文由 Executor 自动注入，不暴露给 LLM。

### 5.4 Tool 执行层

**文件：** `internal/conversation/executor.go`

- `ToolExecutor.Execute(ctx, session, toolName, argsJSON) → (string, error)`
- 对 confirm/reject/update_category：当 `draftId=0` 时自动调用 `ResolvePendingDraftContext` 解析上下文
- 返回 JSON string 作为 tool message content 传回 LLM
- 错误不 panic，序列化为 `{"error":"..."}` 让 LLM 自然处理

**接口设计：** 定义 `ServiceLayer` interface 避免 conversation → service 循环依赖，`service.Services` 通过 `agent_adapter.go` 中的 `*ForAgent` 方法实现该接口。

### 5.5 Agent Loop

**文件：** `internal/conversation/agent.go`

核心流程：
1. 从 Redis 加载对话历史
2. 构建 messages = [system_prompt] + history + [user_message]
3. 循环（最多 5 轮）：
   - 调用 `llm.Chat(messages, tools)`
   - `finish_reason == "stop"` → 提取文本回复 → 保存历史 → 返回
   - `finish_reason == "tool_calls"` → 执行工具 → 追加 tool messages → 继续循环
4. 超过 5 轮 → 返回 fallback 回复
5. 如果本轮调用了 `create_knowledge_draft`，附带 CardMarkdown 给上层发卡片

### 5.6 Service 接入

**文件：** `internal/service/conversation_service.go`

`HandleConversationWithContext` 改为：
1. 如果 `ConvAgent != nil && llmAvailable()` → 调用 `Agent.Run()`
2. Agent 成功 → 返回 `AgentResult` 转换的 `BotConversationResult`
3. Agent 失败 → 降级调用 `handleConversationV2()`（原 V2 switch-case 完整保留）

**文件：** `internal/service/agent_adapter.go`（新增）

7 个 `*ForAgent` 方法，实现 `conversation.ServiceLayer` 接口。

### 5.7 System Prompt

**文件：** `app/agent/prompts/system.md`

完全重写，包含：
- 角色定位：私人知识沉淀助手
- 性格：随和、简洁、像朋友
- 工具使用指导：何时用哪个工具
- 回复风格：自然中文，不输出 JSON/命令格式
- 重要规则：正式保存必须确认、多条草稿时先列出、不编造内容

### 5.8 配置调整

| 配置项 | V2 值 | V3 值 | 原因 |
|---|---|---|---|
| `LLM_MAX_TOKENS` | 1200 | 2000 | LLM 生成更长的自然语言回复 |
| `LLM_TIMEOUT_MS` | 20000 | 20000 | 不变 |
| `CONV_HISTORY_MAX_MESSAGES` | - | 20 | 新增 |
| `CONV_HISTORY_TTL_MINUTES` | - | 120 | 新增 |

---

## 六、文件变更清单

### 新增文件
| 文件 | 说明 |
|---|---|
| `internal/conversation/history.go` | Redis 对话历史管理 |
| `internal/conversation/tools.go` | LLM 工具定义（7 个） |
| `internal/conversation/executor.go` | 工具执行层 |
| `internal/conversation/agent.go` | Agent loop 核心 |
| `internal/service/agent_adapter.go` | Service → conversation.ServiceLayer 适配 |

### 修改文件
| 文件 | 说明 |
|---|---|
| `internal/llm/types.go` | 新增 Chat 相关类型 + Client 接口扩展 |
| `internal/llm/client.go` | 新增 `Chat` 方法实现 |
| `internal/service/conversation_service.go` | 主路径切到 Agent，V2 保留为 fallback |
| `internal/service/services.go` | Services 新增 `ConvAgent` 字段 |
| `internal/config/config.go` | 新增对话历史配置项 |
| `cmd/app-server/main.go` | 组装 conversation Agent |
| `app/agent/prompts/system.md` | 完全重写 |
| `deploy/.env` | 调整 LLM_MAX_TOKENS，新增对话历史配置 |

### 不变的部分
- `internal/api/handlers.go` / `ws_handlers.go` — handler 层不变
- `internal/api/mcp.go` — 外部 MCP 端点保留
- `internal/repository/*` — 数据层不变
- `internal/feishu/messenger.go` — 消息发送不变
- `cmd/app-worker/main.go` — Worker 不变
- 卡片回调 — 继续走 `processCardAction` 直接执行

---

## 七、测试方案

### 7.1 编译验证
- `GOFLAGS="-tags=stdjson" go build ./...` — 已通过
- `GOFLAGS="-tags=stdjson" go vet ./...` — 已通过

### 7.2 部署验证
- `docker compose up -d --build`
- 日志出现 `conversation agent enabled`

### 7.3 功能验证

| 场景 | 输入 | 预期 |
|---|---|---|
| 通用聊天 | "你好" / "谢谢" | 自然回复，不再返回 "我还没完全理解" |
| 知识沉淀 | "帮我记一下，飞书验签失败是因为 header token 不匹配" | 调用 create_knowledge_draft + 自然语言回复 + 确认卡片 |
| 多轮对话 | 创建草稿后说 "确认保存" | 基于历史命中草稿，确认保存 |
| 知识查询 | "之前关于接口限流的结论是什么？" | 调用 search_knowledge + 自然语言总结 |
| 降级验证 | 设置 `LLM_ENABLED=false` | 回退到 V2 switch-case 路径 |
| 卡片交互 | 点击卡片按钮 | 仍正常工作（不经过 Agent） |

### 7.4 回归验证
- `/kb search`、`/kb approve` 命令仍正常
- 已有 create / search / similarity 能力不受影响
- 草稿过期 / 提醒 worker 不受影响

---

## 八、实现顺序

| Step | 内容 | 状态 |
|---|---|---|
| 1 | Redis 对话历史 + 配置 | 已完成 |
| 2 | LLM Client Chat 方法 | 已完成 |
| 3-4 | Tool 定义 + Executor | 已完成 |
| 5 | Agent Loop 核心 | 已完成 |
| 6-7 | Service 接入 + 组装 | 已完成 |
| 8 | Config / System Prompt / .env | 已完成 |
| 9 | 部署验证 | 已完成 |
| 10 | 端到端功能验证 | 进行中 |
| 11 | Bug 修复：日期幻觉 + 草稿修订流程 | 已完成 |

---

## 十、部署后 Bug 修复

### 10.1 LLM 日期幻觉

**问题**：用户要求"补充日期"时，LLM 编造了错误日期"2024年12月19日"，实际应为 2026-04-22。

**原因**：LLM 无法感知当前时间，system prompt 中也未注入时间信息。

**修复**：`internal/conversation/agent.go` 中动态将当前时间追加到 system prompt：
```go
now := time.Now()
systemContent := a.systemPrompt + fmt.Sprintf("\n\n当前时间：%s", now.Format("2006年01月02日 15:04 (Monday)"))
```

### 10.2 草稿修订创建新草稿而非更新

**问题**：用户对刚创建的草稿追加内容（如"补充日期"），Agent 调用 `create_knowledge_draft` 创建了新草稿，旧卡片仍可操作。

**原因**：工具集中没有"修订草稿"工具，LLM 只能创建新草稿。

**修复**：新增 `revise_knowledge_draft` 工具：
- **`tools.go`**：新增工具定义，描述为"修订一条待确认草稿的内容"
- **`executor.go`**：`ServiceLayer` 接口新增 `ReviseDraftForAgent` 方法，Execute 新增分发 case
- **`agent_adapter.go`**：实现 `ReviseDraftForAgent`，流程为：解析旧草稿 → 拒绝旧草稿 → 用 PatchCard 将旧卡片标记为"已修订为新草稿" → 创建修订后的新草稿
- **`agent.go`**：`revise_knowledge_draft` 也触发 CardMarkdown 生成
- **`messenger.go`**：`BuildResolvedCardJSON` 新增 `"revised": "已修订为新草稿"` 状态
- **`system.md`**：工具列表和使用指导增加 `revise_knowledge_draft` 说明

---

## 九、当前确认结论

1. 架构模式采用多轮 Tool-Calling Agent，LLM 自主决定意图和工具调用
2. 对话历史用 Redis LIST 存储，user+chat 维度，20 条 / 2h TTL
3. V2 完整保留为降级路径（LLM 不可用时自动回退）
4. System prompt 完全重写，注入工具使用指导和回复风格要求
5. 卡片回调不走 Agent，继续直接执行
6. 本轮不做联网搜索、向量检索、模型切换

---

## 十一、过期草稿卡片仍可点击修复

**问题**：飞书端过期草稿的确认卡片按钮仍然可以点击，点击后弹出报错提示。

**根因（3 个层面）：**
1. `RejectDraft` 没有状态校验 — 直接调 `UpdateDraftStatus`，无 status 前置条件
2. `ApproveDraft` 不检查 `expires_at` — Worker 30 秒窗口内 status 仍是 PENDING 但时间已过期
3. `UpdateDraftStatus` SQL 无 status 守卫 — 任何路径都可覆盖终态

**修复：**

| 文件 | 改动 |
|---|---|
| `conversation_service.go` RejectDraft | 增加 GetStructuredDraft → status + expires_at 双重校验 |
| `services.go` ApproveDraft | 增加 expires_at 检查 |
| `store.go` UpdateDraftStatus | SQL 增加 `AND status='PENDING_CONFIRMATION'` |
| `handlers.go` processCardAction | change_category 分支增加卡片更新 |

**状态**：已修复、已部署验证

---

## 十二、卡片样式优化 + 搜索召回率修复

### 12.1 卡片样式优化

**问题**：确认/拒绝/过期后的卡片中，`# 草稿 #ID` 作为 H1 最大标题展示，对用户来说草稿 ID 是次要信息，知识标题才是核心。

**修复**：将知识标题作为 H1 标题展示，草稿 ID 降为普通列表项。

| 文件 | 改动 |
|---|---|
| `handlers.go` resolvedMarkdown / confirmedMarkdown | `# {Title}` 作为 H1，`草稿ID：#%d` 降为列表项 |
| `services.go` ExpirePendingDrafts Worker 路径 | 同步调整 |

**效果**：卡片大标题从 "草稿 #112" 变为 "TypeScript as const 断言的作用"。

### 12.2 搜索召回率修复

**问题**：用户确认保存知识后查询刚存的内容，`search_knowledge` 返回空结果。

**根因**：ILIKE 使用整个查询字符串作为连续子串匹配（`%2026-04-23 饮食 训练计划%`），而内容中这些词分散在不同位置，无法匹配。FTS 的 `simple` 配置也不做中文分词。

**修复**：Go 层按空格拆词，SQL 改用 `ILIKE ANY($5)` 按词独立匹配。

| 文件 | 改动 |
|---|---|
| `store.go` SearchKnowledge | `like` string → `likeTerms` []string，ILIKE → ILIKE ANY |
| `draft_similarity_store.go` SearchKnowledgeEvidence | 同上 |

**效果**：查询 "低碳日 有氧训练" 从 0 结果变为 4 条召回。

**已知局限**：纯中文无空格查询（如 "今天的训练计划"）仍无法自动拆词，需引入 pg_zhparser 或应用层分词（留后续迭代）。

**状态**：已修复、已部署验证
