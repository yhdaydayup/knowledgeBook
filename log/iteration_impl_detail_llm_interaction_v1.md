<callout background-color="light-blue">
本稿是“进入代码编写前”的修改细节文档，作用不是替代主技术方案或差异技术方案，而是把本次迭代需要真正落到代码里的修改项拆开，方便逐条讨论与确认。

当前目标：先对齐实现细节，再开始编码。
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | knowledgeBook 迭代修改细节文档（LLM 交互层 v1） |
| 文档版本 | v1.0-draft |
| 更新时间 | 2026-03-29 |
| 关联文档 | `knowledgeBook/log/prd_delta_llm_interaction_v1.md`、`knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md` |
| 当前用途 | 实现前评审与讨论 |

---

## 二、本次迭代的实现目标

本次迭代不是一次性重写整套系统，而是在现有可运行 MVP 上，落地下面 4 件事：

1. **引入自然语言交互入口**
   - 让机器人不再只认 `/kb ...` 命令
   - 能识别“新增知识 / 查询知识 / 请求合并 / 询问重复”等自然语言表达

2. **新增知识前做结构化提取**
   - 对用户输入提取标题、摘要、关键要点、标签、分类提示
   - 保留原文，同时生成结构化草稿

3. **新增知识前做相似知识检测**
   - 召回相似知识
   - 输出 merge / supplement / conflict / new 的建议
   - 默认推荐 merge，冲突时进入用户选择

4. **检索结果升级为答案 + 证据**
   - 检索后不只返回命中列表
   - 由 LLM 组织答案，同时返回支撑知识条目和文档位置链接

---

## 三、建议分两阶段落地

为降低一次性改造风险，建议把这次迭代拆为两个开发阶段。

## Phase 1：可用版
优先落地：
- 自然语言新增知识
- 自然语言查询知识
- 候选知识结构化提取
- 相似知识检测（先做推荐，不自动合并）
- 基础 MCP 工具层
- 基础 LLM 展示层

先不在 Phase 1 强推：
- 自动合并执行
- 冲突知识的复杂版本管理 UI
- 多轮复杂对话状态机
- 历史知识批量重整

## Phase 2：增强版
后续补：
- 合并正式执行链路
- 冲突处理完整状态流转
- 更多 MCP 工具
- 更新/删除/恢复全面接入自然语言入口
- 更强的检索重排与展示能力

### 当前建议
如果你希望控制风险，我建议本轮代码实现先做 **Phase 1**。

---

## 四、本轮建议纳入代码实现的具体范围

## 4.1 自然语言意图范围
本轮建议只支持以下 4 类意图：

### A. create_knowledge
示例：
- “帮我记一下，飞书事件验签生产环境还没补完”
- “把这段整理成知识点存起来”

### B. search_knowledge
示例：
- “查一下之前关于 FTS 的结论”
- “我们之前是怎么说飞书同步的”

### C. check_similarity
示例：
- “这条和之前那条是不是重复”
- “这段是不是和缓存方案那条差不多”

### D. clarify / fallback
信息不足时，让机器人追问，而不是乱执行。

### 暂不纳入本轮自然语言入口
- update_knowledge
- delete_knowledge
- restore_knowledge
- sync_from_doc
- category 管理
- merge 正式执行

这些能力本轮仍保留命令入口，但不强行在首轮就纳入自然语言编排。

---

## 五、模块修改清单

## 5.1 bot-gateway
### 需要改什么
- 保留当前飞书接入、challenge、验签、消息回包逻辑
- 在消息解析后，不再只直接走命令路由
- 新增“命令 / 自然语言”分流逻辑

### 建议改法
- 若消息以 `/kb` 开头，继续走现有 command-router
- 否则走新的 conversation entry

### 预期输出
统一请求对象：
- userOpenId
- chatId
- messageId
- rawText
- isCommand
- traceId

---

## 5.2 新增 conversation-orchestrator
### 职责
- 调用 LLM 做意图识别
- 根据意图决定调用哪些 MCP 工具
- 把结果组装成机器人回复

### 为什么单独加这一层
- 避免把 LLM 调用散落到 bot-gateway / search-service / draft-service 中
- 让后续意图扩展更容易

### 本轮最小接口
- `HandleConversation(ctx, req)`

输入：标准化消息对象  
输出：机器人响应对象

---

## 5.3 新增 intent-parser
### 职责
把自然语言映射为：
- create_knowledge
- search_knowledge
- check_similarity
- clarify

### 本轮输出结构
```json
{
  "intent": "create_knowledge",
  "confidence": 0.94,
  "needs_clarification": false,
  "slots": {
    "raw_text": "帮我记一下，飞书事件验签生产环境还没补完"
  }
}
```

### 讨论点
- 是否需要做 rule-based fallback
- 当 LLM 不可用时，是否要回退到命令提示而不是完全失败

我建议：**需要简单 fallback**，至少保证系统不会完全失语。

---

## 5.4 改造 draft-service
### 需要改什么
当前 draft-service 更多承接候选知识管理，本轮需要增强为：
- 保存 raw_content
- 保存结构化提取结果
- 保存分类 hint
- 保存 llm_confidence

### 建议新增字段
- `raw_content`
- `normalized_title`
- `normalized_summary`
- `normalized_points` JSONB
- `tags` TEXT[]
- `recommended_category_path`
- `llm_confidence`

### 状态建议
- `PENDING_CONFIRMATION`
- `IGNORED`
- `DEFERRED`
- `CONFLICT_PENDING`
- `CONFIRMED`

### 本轮建议
保留旧状态兼容，但新增字段先落地。

---

## 5.5 新增 similarity-service
### 职责
- 召回相似知识
- 计算相似度
- 判断 relation_type

### 本轮实现建议
#### 召回层
先采用：
- PostgreSQL FTS 召回 + tag/category 过滤
- 若已有 embedding 能力，则补充向量召回 topK

#### 判断层
由 LLM 对候选结果做关系判断：
- merge_candidate
- supplement_candidate
- conflict_candidate
- new_knowledge

### 本轮先不做
- 自动执行 merge
- 批量去重历史数据

---

## 5.6 改造 search-service
### 需要改什么
当前 search-service 主要返回命中结果，本轮改为返回：
- answer draft 所需的 evidence
- related / conflict candidates
- doc links

### 建议输出结构
```json
{
  "query": "之前关于 FTS 的结论是什么",
  "hits": [
    {
      "knowledge_id": 101,
      "title": "PostgreSQL FTS 方案",
      "summary": "...",
      "category_path": "...",
      "doc_anchor_link": "..."
    }
  ],
  "related": [],
  "conflicts": []
}
```

### 说明
最终 answer 仍由 orchestrator / response composer 调 LLM 生成。

---

## 5.7 新增 Embedded MCP Server
### 为什么这轮就要加
因为这是你明确要求的边界：
- 后端功能要标准 MCP 化
- 不希望 LLM 直接调用内部 service
- 希望少走一轮“内部兼容层 → 真 MCP”的重构弯路

### 本轮最小工具集合
#### create flow
- `create_knowledge_draft`
- `get_knowledge_draft`
- `confirm_knowledge_draft`

#### similarity flow
- `check_similarity`
- `get_similarity_candidates`

#### search flow
- `search_knowledge`
- `get_knowledge`
- `get_related_knowledge`

### 暂不在本轮纳入
- `merge_knowledge`
- `resolve_knowledge_conflict`
- `sync_from_doc`
- `update_knowledge`

### 建议实现方式
本轮直接做**真正的 MCP Server**，但采用轻量部署方式：
- 直接在现有 `app-server` 内嵌 MCP Server
- 使用真正 MCP 的 tool schema、调用协议和返回结构
- 由 MCP Server 路由到现有 service / repository
- 暂不独立拆分远程 MCP 服务进程

也就是说：
> **协议做真，部署做轻。**

这样可以一次性把边界定对，后续如果需要开放给更多 agent 或客户端，直接在现有 MCP 基础上扩展即可。

---

## 六、数据库修改建议

## 6.1 本轮建议新增或修改的表/字段

### A. `knowledge_drafts` 增量字段
- `raw_content` TEXT
- `normalized_title` VARCHAR(256)
- `normalized_summary` TEXT
- `normalized_points` JSONB
- `recommended_category_path` TEXT
- `llm_confidence` NUMERIC(5,4)

### B. 新增 `knowledge_similarity`
建议字段：
- `id`
- `draft_id`
- `knowledge_id`
- `similarity_score`
- `relation_type`
- `reason`
- `created_at`

### C. 可选：`knowledge_items` 补充字段
如果当前正式知识表缺少结构化展示字段，建议补：
- `key_points` JSONB

### 本轮建议不新增的表
- `knowledge_conflicts`
- `knowledge_merge_relations`

原因：
如果本轮只做“检测 + 建议 + 用户确认创建”，而不正式做 merge/resolve 执行，这两张表可以放到 Phase 2。

这是一个我建议你重点确认的范围收敛点。

---

## 七、机器人交互修改建议

## 7.1 新增知识卡片
本轮建议展示：
- 标题
- 摘要
- 关键要点
- 推荐分类
- 相似知识提示
- 推荐动作

### 用户可点动作
- 确认保存
- 暂不保存
- 稍后处理

### 相似时附加说明
- 发现 1~3 条相似知识
- 系统建议：合并 / 补充 / 存为新知识

### 本轮建议
本轮先不要在卡片里做过多复杂分叉按钮，先保证：
- 看得清
- 能确认
- 能取消

---

## 7.2 查询结果卡片
本轮建议展示：
- 一段答案摘要
- 支撑知识条目
- 文档跳转链接
- 若有冲突，显示“存在不同版本结论”提示

---

## 八、LLM prompt / schema 建议

## 8.1 意图识别 schema
建议强约束输出：
- intent
- confidence
- needs_clarification
- slots

## 8.2 知识提取 schema
建议强约束输出：
- title
- summary
- key_points
- tags
- category_hint
- confidence

## 8.3 相似关系判断 schema
建议强约束输出：
- relation_type
- score
- reason
- suggested_action

### 为什么强调 schema
如果不强约束，后续 orchestrator 会很难稳定消费模型输出。

---

## 九、测试计划

## 9.1 单元测试
- intent parser 输出结构校验
- draft 持久化字段校验
- similarity relation 映射逻辑
- search evidence 结构校验

## 9.2 集成测试
- 自然语言新增 -> intent parser -> MCP tool 调用 -> draft 创建 -> 相似检测 -> 卡片返回
- 自然语言查询 -> MCP tool 调用 -> 检索 -> answer + evidence 返回
- Embedded MCP Server tool 注册与调用链路可用

## 9.3 回归测试
本轮不以“兼容老交互行为”作为硬约束，但至少要确认以下基础能力没有被无意破坏：
- `/kb add`
- `/kb search`
- `/kb approve`
- `/kb update`
- `/kb remove`
- `/kb restore`
- `/kb sync-from-doc`

---

## 十、我建议优先确认的 8 个实现问题

1. **本轮是否只做 Phase 1？**
   - 我建议：是

2. **自然语言入口本轮是否只覆盖 create/search/check_similarity？**
   - 我建议：是

3. **MCP 是否直接做真正的嵌入式 MCP Server？**
   - 我建议：是

4. **本轮是否只做“相似建议”，暂不做正式 merge 执行？**
   - 我建议：是

5. **`knowledge_conflicts` / `knowledge_merge_relations` 是否放到 Phase 2？**
   - 我建议：是

6. **查询结果是否必须升级为 answer + evidence？**
   - 我建议：是

7. **LLM 不可用时是否提供 fallback？**
   - 我建议：是，至少要回退为提示用户改用命令

8. **是否要求本轮保留所有旧命令路径完全兼容？**
   - 我建议：是

---

## 十一、建议的开发顺序

### Step 1
搭 conversation-orchestrator + intent-parser 壳子

### Step 2
改造 draft-service 数据结构与草稿创建链路

### Step 3
补 similarity-service 与相似关系输出

### Step 4
改造 search-service 输出 evidence 结构

### Step 5
补 Embedded MCP Server 与首批 tools

### Step 6
补机器人卡片与结果展示

### Step 7
补测试与回归

---

## 十二、当前建议的讨论结论模板

如果后续还有调整，可以继续按下面模板讨论：

```plaintext
1. 迭代范围：Phase 1 / 全量
2. 自然语言支持意图范围
3. MCP 实现形态
4. 相似知识只建议还是直接执行 merge
5. 本轮数据库变更范围
6. 查询结果展示样式
7. fallback 策略
8. 回归兼容要求
```

---

## 十三、当前已确认的实现结论

本轮讨论后，当前确认如下：

1. **迭代范围**：按 **Phase 1** 推进。
2. **自然语言支持意图范围**：本轮先支持 `create_knowledge`、`search_knowledge`、`check_similarity`。
3. **MCP 实现形态**：直接做**嵌入在 `app-server` 内的真正 MCP Server**，暂不拆独立远程 MCP 服务。
4. **相似知识处理方式**：本轮先做**相似建议**，不直接执行正式 merge。
5. **本轮数据库变更范围**：先按建议增加 `knowledge_drafts` 增量字段，并新增 `knowledge_similarity`；`knowledge_conflicts`、`knowledge_merge_relations` 放到后续阶段。
6. **查询结果展示样式**：本轮必须升级为 `answer + evidence + doc link`。
7. **fallback 策略**：需要；当 LLM 不可用时，至少退回到提示用户改用命令或稍后再试。
8. **回归兼容要求**：由于当前线上没有真实用户，本轮**不要求兼容老版本自然语言/旧交互行为**；但建议尽量避免无必要破坏已有基础命令链路。