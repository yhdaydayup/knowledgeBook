# knowledgeBook 方案文档总览与阅读导航

<callout background-color="light-blue">
本文件用于帮助团队快速理解当前文档体系：
- 哪些是主文档
- 哪些是差异文档
- 先读什么，后读什么
- 不同文档分别解决什么问题
- 后续新增版本时应如何继续维护
</callout>

## 一、文档体系总览

当前方案文档分为两层：

### 1. 主文档
主文档用于描述 knowledgeBook 的**完整功能基线**。

包括：
- `knowledgeBook/log/prd_mvp_final_confirmed.md`
- `knowledgeBook/log/tech_doc_revision_v2.md`

### 2. 差异文档
差异文档用于描述**某一次版本升级**相对主文档的新增点、改动点和影响范围。

包括：
- `knowledgeBook/log/prd_delta_llm_interaction_v1.md`
- `knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md`

---

## 飞书快速入口

如果你在飞书中阅读，建议直接从下面的链接进入：

- [主 PRD（完整功能版）](https://www.feishu.cn/docx/EQ8Nd8q8Go7IshxRqsHcMNJenEe)
- [主技术方案（完整功能版）](https://www.feishu.cn/docx/Gugad7jfUo73GhxSa3gcbndmnEh)
- [差异 PRD（LLM 交互层升级）](https://www.feishu.cn/docx/AT7GdeMiKoNysqxPzggcZFhZnJe)
- [差异技术方案（LLM + MCP 升级）](https://www.feishu.cn/docx/AIMTdPAbkocd3jxQBGlcv1A4nkG)
- [当前导航页](https://www.feishu.cn/docx/C8UDdQqQOof0DoxyRqWcLgxXnjd)

---

## 二、四份核心文档分别解决什么问题

<lark-table column-widths="180,240,280" header-row="true">
<lark-tr>
<lark-td>

**文档**

</lark-td>
<lark-td>

**主要回答的问题**

</lark-td>
<lark-td>

**适合谁读**

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

`knowledgeBook/log/prd_mvp_final_confirmed.md`

</lark-td>
<lark-td>

knowledgeBook 这个产品完整要做什么、用户怎么用、核心规则是什么

</lark-td>
<lark-td>

产品、设计、研发、测试、项目负责人

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

`knowledgeBook/log/tech_doc_revision_v2.md`

</lark-td>
<lark-td>

knowledgeBook 完整系统怎么实现、模块怎么分、协议怎么定、数据怎么落

</lark-td>
<lark-td>

后端、架构、测试、运维

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

`knowledgeBook/log/prd_delta_llm_interaction_v1.md`

</lark-td>
<lark-td>

本次 LLM 交互层升级相对完整产品基线具体新增了哪些产品能力与体验变化

</lark-td>
<lark-td>

产品、设计、研发、项目负责人

</lark-td>
</lark-tr>
<lark-tr>
<lark-td>

`knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md`

</lark-td>
<lark-td>

本次 LLM + MCP 升级相对完整技术基线新增了哪些架构、模块、协议和数据改动

</lark-td>
<lark-td>

后端、架构、测试、运维

</lark-td>
</lark-tr>
</lark-table>

---

## 三、推荐阅读顺序

## 3.1 想快速理解整个项目
建议按以下顺序：

```plaintext
1. 主 PRD
2. 主技术方案
3. 差异 PRD
4. 差异技术方案
```

含义是：
- 先建立完整产品和系统基线
- 再理解本次版本升级具体改了什么

## 3.2 只关心产品
建议顺序：

```plaintext
1. knowledgeBook/log/prd_mvp_final_confirmed.md
2. knowledgeBook/log/prd_delta_llm_interaction_v1.md
```

## 3.3 只关心技术实现
建议顺序：

```plaintext
1. knowledgeBook/log/tech_doc_revision_v2.md
2. knowledgeBook/log/tech_doc_delta_llm_interaction_v1.md
```

## 3.4 只关心本次升级
建议顺序：

```plaintext
1. 差异 PRD
2. 差异技术方案
```

前提是阅读者已经理解主文档定义的完整基线。

---

## 四、主文档与差异文档的关系

### 4.1 主文档的职责
主文档必须长期保持为：
- 完整功能视角
- 稳定基线视角
- 全局规则视角

主文档不能因为某次版本迭代而收缩成“只描述本次任务”的临时文档。

### 4.2 差异文档的职责
差异文档必须聚焦：
- 相对主文档新增了什么
- 改了什么
- 影响了什么
- 怎么落地

差异文档不能替代主文档，也不应该重新把全量内容再写一遍。

### 4.3 两者配合关系
可以把它理解成：

```plaintext
主文档 = 全量地图
差异文档 = 本次施工说明
```

---

## 五、当前这次版本升级的定位

当前这次差异文档描述的是：

> knowledgeBook 从“机器人命令驱动 + 原文入库倾向”的体验，升级为“LLM 交互理解 + MCP 工具编排 + 相似知识处理 + 展示层增强”的版本。

因此，本次升级并不是推翻主系统，而是在完整功能基线上做一层关键增强。

---

## 六、后续维护规则

### 6.1 当出现新版本需求时
按以下方式维护：
- 先更新主文档中的完整基线内容（如果基线被永久改变）
- 再新增或更新对应的差异文档

### 6.2 当只是本次临时讨论时
不要直接改坏主文档结构；优先补差异文档或评审备注。

### 6.3 当文档开始重复时
优先做收敛：
- 主文档保留完整稳定版本
- 差异文档只保留增量信息
- 避免再新增平行的“新主文档”

---

## 七、相关文档

除了四份核心方案文档，当前还建议结合以下文档一起使用：
- `knowledgeBook/docs/DOC_SYNC_CONVENTION.md`：说明本地 Markdown 与飞书文档的同步规则
- `knowledgeBook/docs/DEPLOYMENT.md`：部署相关说明

---

## 八、一句话结论

如果只记住一句话，请记住：

> 主文档负责描述 knowledgeBook 的完整基线；差异文档负责描述某次版本升级相对于完整基线的变化。