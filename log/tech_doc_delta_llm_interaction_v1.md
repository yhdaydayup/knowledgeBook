<callout background-color="light-blue">
本稿不是主技术方案，而是“本次变更的差异技术方案”。

目的：说明在完整功能基线不变的前提下，本次 LLM 交互层升级具体新增了什么、改了什么、影响了什么。
</callout>

## 一、文档信息

| 项目 | 内容 |
| --- | --- |
| 文档名称 | knowledgeBook 差异技术方案：LLM 交互层与 MCP 升级 |
| 文档版本 | v1.0 |
| 更新时间 | 2026-03-29 |
| 关联主文档 | `knowledgeBook/log/tech_doc_revision_v2.md` |
| 文档定位 | 单次版本差异说明 |

---

## 二、本次变更背景

当前系统虽然已经具备完整知识链路，但在交互体验和知识质量上仍有明显问题：
- 机器人主要依赖命令，用户交互门槛高
- 原始输入容易直接存储，缺少重点提取
- 相似知识会重复进入知识库，缺少语义级处理能力
- 查询结果更多是“命中返回”，而不是“答案组织 + 证据展示”

因此，本次升级的目标不是扩展一个孤立新功能，而是为完整系统补上一层统一的语义理解与能力编排层。

---

## 三、本次变更目标

本次变更聚焦 4 个目标：
1. 引入 LLM 作为统一交互理解层
2. 引入 MCP 作为 LLM 调用后端能力的标准工具层
3. 在新增知识前补充重点提取、相似知识检测与确认流程
4. 让 LLM 参与知识展示层，提升查询与知识消费体验

---

## 四、与原基线相比的变化概览

<lark-table column-widths="190,250,260" header-row="true">
<lark-tr>
<lark-td>

**维度**

</lark-td>
<lark-td>

**原基线**

</lark-td>
<lark-td>

**本次升级后**

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

交互入口

</lark-td>
<lark-td>

以命令和既有机器人交互为主

</lark-td>
<lark-td>

自然语言成为主入口，命令保留为补充入口

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

新增知识入库前处理

</lark-td>
<lark-td>

主要是候选生成 + 分类推荐

</lark-td>
<lark-td>

增加重点提取、相似检测、合并/冲突建议、用户确认

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

后端能力暴露方式

</lark-td>
<lark-td>

内部服务或命令路由直连

</lark-td>
<lark-td>

统一通过 MCP 工具层暴露给 LLM

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

查询结果组织方式

</lark-td>
<lark-td>

偏向命中结果列表

</lark-td>
<lark-td>

升级为“答案 + 证据 + 冲突提示 + 关联知识”

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

重复知识处理

</lark-td>
<lark-td>

缺少语义级重复识别

</lark-td>
<lark-td>

默认推荐合并；冲突时强制用户决策

</lark-td>
</lark-tr>
</lark-table>

---

## 五、架构差异点

## 5.1 新增 LLM Orchestrator
新增编排层，负责：
- 自然语言意图识别
- 候选知识提取
- 展示内容组织
- 调用 MCP 工具

它不直接写库，也不直接替代领域服务。

## 5.2 新增 Embedded MCP Server / Tool Layer
新增标准能力层，负责：
- 在现有 `app-server` 内直接提供真正的 MCP Server 能力
- 工具注册
- schema 校验
- 权限与幂等控制
- 对内映射到领域服务

本次实现策略不是“先做一层内部兼容假 MCP”，而是：
- **直接采用真正 MCP 协议与 tool schema**
- **先嵌入 `app-server` 部署**
- **暂不独立拆分远程 MCP 服务**

这样可以把“LLM 如何调用能力”和“后端如何实现能力”解耦，同时避免后续再做一轮协议迁移。

## 5.3 新增 similarity-service 能力升级
从原来几乎没有系统化去重能力，升级为：
- 相似知识召回
- merge / supplement / conflict / new 判断
- 为用户提供处理建议

## 5.4 展示层升级
原先检索更偏向返回命中结果；升级后由 LLM 组织：
- 答案
- 证据知识
- 冲突知识
- 关联知识

---

## 六、模块变更清单

## 6.1 新增模块
- `intent-parser`
- `knowledge-extractor`
- `response-composer`
- `mcp-tool-server`
- `mcp-authz`
- `similarity-service`
- `merge-service`

## 6.2 改造模块
- `bot-gateway`：从纯命令接入升级为统一消息接入
- `command-router`：降级为编排层中的补充入口，不再是唯一入口
- `draft-service`：从单纯候选管理升级为承接 LLM 提取结果的草稿中心
- `search-service`：从结果命中升级为支撑“答案 + 证据”输出
- `doc-sync-service`：继续保留，但需要适配新增知识关系与版本变化

## 6.3 不变模块边界
以下边界保持不变：
- 数据库仍是唯一事实源
- 正式写入仍必须走领域服务
- 飞书文档仍是派生展示层
- 文档回收仍必须显式触发

---

## 七、协议差异点

## 7.1 新增统一对话请求协议
新增：`NormalizedConversationRequest`

作用：
- 把飞书消息、命令、卡片回调统一成一个标准请求体

## 7.2 新增 LLM 理解协议
新增：
- `IntentUnderstandingResult`
- `ExtractedKnowledgeDraft`

作用：
- 把意图识别与知识提取结果标准化，便于后续工具调用

## 7.3 新增 MCP 工具调用协议
新增：
- `ToolCallRequest`
- `ToolCallResult`

作用：
- 让 LLM 到后端能力之间的调用边界标准化

## 7.4 新增展示结果协议
新增：`KnowledgeSearchResult`

作用：
- 统一输出答案、证据、冲突与关联知识

---

## 八、数据模型差异点

## 8.1 新增或强化的数据实体
相较原基线，本次需要新增或强化：
- `knowledge_similarity`
- `knowledge_conflicts`
- `knowledge_merge_relations`
- `knowledge_drafts` 中的提取结果字段
- `knowledge_items` 中支撑展示层与合并关系的字段

## 8.2 `knowledge_drafts` 增量字段
建议新增：
- `raw_content`
- `normalized_title`
- `normalized_summary`
- `normalized_points`
- `recommended_category_path`
- `llm_confidence`

## 8.3 新增 `knowledge_similarity`
用于保存：
- 草稿与正式知识之间的相似关系
- 相似度分值
- 关系类型
- 判断理由

## 8.4 新增 `knowledge_conflicts`
用于保存：
- 冲突候选
- 用户最终保留策略
- 冲突处理结果

## 8.5 新增 `knowledge_merge_relations`
用于保存：
- 被合并知识与主知识的关系
- 合并动作发起人
- 合并时间与类型

---

## 九、关键流程差异点

## 9.1 新增知识流程差异
原流程：
```plaintext
输入 → 候选 → 确认 → 入库
```

升级后：
```plaintext
输入 → 意图识别 → 知识提取 → 相似检测 → 合并/冲突建议 → 用户确认 → 入库
```

## 9.2 查询流程差异
原流程：
```plaintext
提问 → 检索 → 返回结果
```

升级后：
```plaintext
提问 → 意图识别 → 检索 → 聚合证据 → LLM 组织答案 → 返回答案+证据
```

## 9.3 冲突处理流程新增
新增专门的冲突处理链路：
```plaintext
检测出 conflict_candidate
→ 展示新旧知识差异
→ 用户选择 keep_new / keep_old / keep_both / cancel
→ 写入冲突结果与版本关系
```

---

## 十、实施影响范围

## 10.1 接口与服务影响
需要影响的主要模块：
- 接入层
- 草稿链路
- 检索链路
- 同步链路
- 数据库 schema

## 10.2 测试影响
需要新增测试：
- 自然语言意图识别分流
- 候选知识提取准确性
- 相似知识关系判定
- 冲突处理分支
- MCP 工具调用与权限校验
- 检索答案与 evidence 输出结构

## 10.3 运维影响
需要关注：
- LLM 调用耗时与成本
- 向量计算或检索资源消耗
- MCP 工具调用日志与幂等控制

---

## 十一、实施顺序建议

### Phase 1：引入自然语言编排层与 Embedded MCP Server
- 接入 intent-parser
- 打通自然语言到既有知识链路
- 在 `app-server` 内嵌真正 MCP Server
- 把首批核心能力暴露为 MCP tools

### Phase 2：引入相似知识与冲突建议
- 接入向量召回或等价相似召回
- 打通 merge / conflict 处理前的建议链路

### Phase 3：升级展示层
- search-service 输出 evidence
- response-composer 组织答案
- 让查询结果稳定输出 answer + evidence + doc link

---

## 十二、兼容与风险控制

## 12.1 向下兼容原则
- 显式命令仍可继续使用
- 现有知识表与同步链路不推翻重做
- 旧流程逐步迁移到新编排层，而不是一次性切换

## 12.2 风险控制
- LLM 不直接写库
- 所有写入都需要确认与审计
- 去重先建议、后执行
- 冲突必须用户决策
- 展示答案必须带 evidence

---

## 十三、文档使用说明

本文件只描述“本次变更相对于完整基线的技术差异”。

阅读建议：
1. 先读主技术方案：`knowledgeBook/log/tech_doc_revision_v2.md`
2. 再读本差异方案，理解这次具体改了什么

不要用本文件替代主技术方案。