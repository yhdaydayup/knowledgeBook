# knowledgeBook Runtime Agent 配置与记忆分层设计

## 一、背景
当前需要明确区分两套完全不同的 agent 体系：

1. **Aiden 开发协作层**
   - 服务于开发过程
   - 帮助讨论方案、维护文档、实现代码
   - 不属于 knowledgeBook 产品部署物

2. **knowledgeBook runtime agent 层**
   - 服务于最终用户
   - 属于产品能力的一部分
   - 必须跟随版本与部署一起迁移

本文件描述的是第 2 层，即 knowledgeBook runtime agent 的正式配置与 runtime memory 分层方案。

---

## 二、分层原则

### 1. 配置（Configuration）
特点：
- 稳定
- 版本化
- 跟代码一起提交
- 跟部署一起迁移
- 不能被用户输入直接修改

建议位置：
- `knowledgeBook/app/agent/`

### 2. 记忆（Memory）
特点：
- 动态
- 运行时积累
- 用于提升理解与输出质量
- 可以被用户行为和系统反馈影响
- 但必须通过受控逻辑写入

建议位置：
- 数据库

---

## 三、当前目录结构

```plaintext
knowledgeBook/app/agent/
├── agent.yaml
├── prompts/
│   ├── system.md
│   ├── intent-parser.md
│   ├── knowledge-extractor.md
│   ├── similarity-judge.md
│   └── answer-composer.md
├── schemas/
│   ├── intent.schema.json
│   ├── draft.schema.json
│   ├── similarity.schema.json
│   └── answer.schema.json
├── policies/
│   ├── safety.yaml
│   └── memory-write-policy.yaml
└── mcp/
    ├── server.yaml
    └── tools.yaml
```

### 各文件作用
- `agent.yaml`：runtime agent 主配置入口
- `prompts/`：各能力阶段的 prompt 定义
- `schemas/`：LLM 输入输出的强约束结构
- `policies/`：安全边界与 memory 写入边界
- `mcp/`：项目内 runtime agent 可调用的 MCP 配置

---

## 四、runtime memory 建议

## 4.1 建议不要放文件里
runtime memory 更适合放数据库，而不是放在仓库文件里。

原因：
- 它会随着用户行为持续变化
- 不适合频繁进入 Git 版本库
- 需要支持按用户隔离
- 需要支持清理、压缩、权重和统计

## 4.2 建议的数据域
后续建议引入：
- `agent_user_profiles`
- `agent_user_preferences`
- `agent_memory_entries`
- `agent_memory_summaries`
- `agent_feedback_events`
- `agent_conversation_sessions`

## 4.3 写入原则
允许进入 runtime memory 的内容：
- 用户偏好
- 长期使用习惯
- 经常接受或拒绝的输出方式
- 对系统回答的纠偏反馈

不允许用户直接改写的内容：
- agent 角色定义
- system prompt
- schema
- safety policy
- tool 暴露边界

---

## 五、与 Aiden 协作层的关系

### Aiden 协作层
放在：
- `~/.aiden/`
- `aiden-assets/`

作用：
- 服务开发过程
- 跟着开发者走
- 不随 knowledgeBook 产品一起部署

### knowledgeBook runtime agent 层
放在：
- `knowU/knowledgeBook/app/agent/`
- knowledgeBook 数据库

作用：
- 服务最终用户
- 跟着项目版本和部署走
- 不能受开发协作记忆直接污染

---

## 六、当前建议

1. `knowledgeBook/app/agent/` 作为 knowledgeBook runtime agent 的唯一配置入口。
2. 未来所有 runtime agent 配置改动都通过 Git / PR / commit 管理。
3. runtime memory 单独设计为数据库数据域，不并入 Aiden 的 memory。
4. 项目中的 Aiden 协作资产后续应尽量迁出到全局资产仓库或全局运行目录。 
