# 方案文档与飞书同步约定

<callout background-color="light-blue">
本文件用于约定 knowledgeBook 两份主方案文档的本地 Markdown 与飞书文档之间的一对一映射关系，避免后续多人协作时重复创建、来源混乱或只改单边。
</callout>

## 一、适用范围

当前适用于以下文档：

| 类型 | 本地源文件 | 飞书文档 |
| --- | --- | --- |
| 主产品方案 | `knowledgeBook/log/prd_mvp_final_confirmed.md` | https://www.feishu.cn/docx/EQ8Nd8q8Go7IshxRqsHcMNJenEe |
| 主技术方案 | `knowledgeBook/log/tech_doc_revision_v2.md` | https://www.feishu.cn/docx/Gugad7jfUo73GhxSa3gcbndmnEh |
| 差异 PRD | `knowledgeBook/log/prd_delta_llm_interaction_v1.md` | https://www.feishu.cn/docx/AT7GdeMiKoNysqxPzggcZFhZnJe |
| 差异技术方案 | `knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md` | https://www.feishu.cn/docx/AIMTdPAbkocd3jxQBGlcv1A4nkG |
| 阅读导航 | `knowledgeBook/docs/DOC_READING_GUIDE.md` | https://www.feishu.cn/docx/C8UDdQqQOof0DoxyRqWcLgxXnjd |
| 当前轮迭代技术方案 | `knowledgeBook/log/iteration_interaction_assistant_v2.md` | https://www.feishu.cn/docx/IKbzdxh6zozGJExYQzdc2pJtnEd |

---

## 二、源文件原则

### 2.1 本地 Markdown 是可维护源文件
后续对产品方案、技术方案的内容修改，应优先落到对应的本地 Markdown 文件中。

### 2.2 飞书文档是固定映射的阅读版本
飞书中的两份文档已经建立固定映射：
- 不重复新建同类文档
- 不把同类内容拆成新的平行文档
- 默认在原有 doc_id 上继续更新

### 2.3 两端保持内容一致
每次方案调整，默认要求同时更新：
1. 本地 Markdown
2. 对应飞书文档

不允许长期只改一端而另一端滞后。

---

## 三、更新规则

### 3.1 正常更新流程
```plaintext
先修改本地 md
→ 校对结构与内容
→ 同步更新对应飞书文档
→ 保持标题、章节结构和核心内容一致
```

### 3.2 飞书样式增强规则
允许飞书文档使用更强的展示样式，例如：
- callout
- 表格
- 分栏
- Mermaid 图

但这些样式增强若会影响正文结构，原则上也应回写到本地 Markdown，避免下次同步时被覆盖。

### 3.3 禁止事项
- 禁止为同一主题重复创建新的“产品方案”或“技术方案”飞书文档
- 禁止只在飞书里长期手改而不回写本地源文件
- 禁止把临时讨论稿误当成新的主文档

---

## 四、文档边界

### 4.1 主产品方案文档
主 PRD 必须覆盖完整产品功能，聚焦：
- 产品定位
- 完整功能范围
- 核心规则
- 场景
- 交互
- 全局流程

### 4.2 主技术方案文档
主技术方案必须覆盖完整系统功能，聚焦：
- 整体架构
- 模块
- 协议
- 数据模型
- 状态机
- 关键流程
- 部署与风险

### 4.3 差异方案文档
若某次版本升级只涉及局部新增或改造，应新增差异方案文档。

差异方案文档用于说明：
- 相对主文档的新增点
- 改动点
- 影响范围
- 实施顺序

它不能替代主 PRD 或主技术方案。

如果出现重复内容，应优先把完整能力收敛回主文档，把单次变更收敛到差异文档，而不是继续新增并行主文档。

---

## 五、后续协作约定

若后续需要我继续更新这两份主文档，默认执行策略为：
- 先更新本地源文件
- 再同步更新固定飞书文档
- 维持一对一映射不变
