---

## 文档信息

<lark-table rows="5" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      项目
    </lark-td>
    <lark-td>
      内容
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      产品名称
    </lark-td>
    <lark-td>
      小帮手 - 技术方案文档
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      文档版本
    </lark-td>
    <lark-td>
      V1.0
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      创建日期
    </lark-td>
    <lark-td>
      2026-03-28
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      技术栈
    </lark-td>
    <lark-td>
      Go + Hertz、React + Ant Design、PostgreSQL、Milvus、Redis
    </lark-td>
  </lark-tr>
</lark-table>

---

# 小帮手技术方案文档
---

## 第一章：技术方案概述
### 1.1 系统定位与目标
**产品定位**：小帮手是一个"私人知识管理助手"，支持用户从飞书消息/Web输入中进行知识记录、结构化整理、智能检索和沉淀输出。
**核心目标**：
1. 将碎片信息转为结构化知识
1. 提供双层存储：数据库（AI检索）+ 飞书文档（人类阅读）
1. 提供Human-in-the-Loop确认机制
1. 提供MCP API封装
**非目标（MVP阶段）**：
- 不做移动端
- 不做复杂多人协作权限模型
- 不做多服务拆分
---

### 1.2 核心技术挑战
1. **非结构化到结构化的稳定转换** - 需控制LLM漂移
1. **向量检索 + 关键词检索融合** - 需混合召回+重排
1. **飞书消息/文档双向同步一致性** - 需幂等、版本号、冲突检测
1. **Human-in-the-Loop的低打扰体验** - 确认链路短、可批量处理
1. **MCP安全开放** - 鉴权、配额、审计
---

### 1.3 技术选型说明

<lark-table rows="6" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      技术
    </lark-td>
    <lark-td>
      选型理由
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **Go + Hertz**
    </lark-td>
    <lark-td>
      高性能、低资源占用、工程化成熟
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **PostgreSQL**
    </lark-td>
    <lark-td>
      事务一致性、JSONB、GIN索引
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **Milvus**
    </lark-td>
    <lark-td>
      向量检索能力强，支持ANN索引
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **Redis**
    </lark-td>
    <lark-td>
      缓存、会话、限流
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **React + Ant Design**
    </lark-td>
    <lark-td>
      快速交付后台型产品
    </lark-td>
  </lark-tr>
</lark-table>

---

## 第二章：系统架构设计
### 2.1 整体架构
```plaintext
用户交互层（Web端 + 飞书机器人）
        ↓
API网关层（Hertz - 统一鉴权、限流）
        ↓
业务服务层（消息采集、知识整理、检索、MCP、同步）
        ↓
数据存储层（PostgreSQL + Milvus + Redis + 飞书API）
        ↓
AI服务层（LLM + Embedding）

```

---

### 2.2 模块划分与职责

<lark-table rows="6" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      模块
    </lark-td>
    <lark-td>
      职责
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **消息采集模块**
    </lark-td>
    <lark-td>
      飞书消息清洗、AI提取结构化知识
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **知识整理模块**
    </lark-td>
    <lark-td>
      AI结构化、置信度评分、人工确认队列
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **知识检索模块**
    </lark-td>
    <lark-td>
      混合检索、重排、答案生成
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **MCP服务模块**
    </lark-td>
    <lark-td>
      Tool schema、Token鉴权、调用审计
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **飞书文档同步模块**
    </lark-td>
    <lark-td>
      文档写入、版本冲突处理
    </lark-td>
  </lark-tr>
</lark-table>

---

### 2.3 数据流向设计
**流程A：消息采集 → 结构化 → 待确认**
1. 飞书消息进入Webhook
1. 幂等校验
1. 调用LLM输出结构化草稿
1. 写入knowledge_draft，状态PENDING_REVIEW
**流程B：人工确认 → 入库 → 向量化 → 同步文档**
1. 用户批准草稿
1. 写入knowledge_item
1. 异步生成embedding存Milvus
1. 触发飞书文档生成
**流程C：检索问答**
1. Query改写 + embedding
1. PG全文召回 + Milvus向量召回
1. 融合重排TopK
1. LLM生成答案并附引用
---

## 第三章：飞书集成方案
### 3.1 飞书机器人接入
**接入步骤**：
1. 创建企业自建应用，开通机器人能力
1. 配置事件订阅URL：`POST /api/v1/feishu/events`
1. 校验challenge
1. 验签
---

### 3.2 消息事件处理
**支持事件**：
- `im.message.receive_v1`
**处理策略**：
- 忽略机器人自身消息
- 提取文本、图片、文件、引用消息
- 附件下载采用异步任务
---

### 3.3 飞书文档API集成
**功能**：
1. 创建知识库文档（按主题/月份）
1. 追加知识条目
1. 文档ID与知识ID映射
**文档模板**：
```markdown
# {{topic}} 知识沉淀

## {{date}} - {{title}}
- 来源：{{source}}
- 标签：{{tags}}
- 摘要：{{summary}}

```

---

### 3.4 用户认证流程（OAuth）
采用飞书OAuth2：
- Web登录：授权码模式
- 机器人场景：通过open_id绑定站内用户
---

## 第四章：数据库设计
### 4.1 核心表结构
#### 用户表
```sql
CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  open_id VARCHAR(64) UNIQUE NOT NULL,
  name VARCHAR(128),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

#### 原始消息表
```sql
CREATE TABLE raw_messages (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  source VARCHAR(32) NOT NULL,
  content TEXT,
  metadata JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

#### 知识草稿表
```sql
CREATE TABLE knowledge_drafts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  title VARCHAR(256),
  summary TEXT,
  content_markdown TEXT,
  tags TEXT[],
  confidence NUMERIC(5,4),
  status VARCHAR(32) DEFAULT 'PENDING_REVIEW',
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

#### 知识主表
```sql
CREATE TABLE knowledge_items (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  title VARCHAR(256) NOT NULL,
  content_markdown TEXT NOT NULL,
  tags TEXT[],
  version INT DEFAULT 1,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

#### 知识切片表
```sql
CREATE TABLE knowledge_chunks (
  id BIGSERIAL PRIMARY KEY,
  item_id BIGINT NOT NULL REFERENCES knowledge_items(id),
  chunk_text TEXT NOT NULL,
  tsv tsvector,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

#### MCP客户端表
```sql
CREATE TABLE mcp_clients (
  id BIGSERIAL PRIMARY KEY,
  owner_user_id BIGINT NOT NULL REFERENCES users(id),
  client_id VARCHAR(64) UNIQUE NOT NULL,
  scopes TEXT[] NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

```

---

### 4.2 向量索引设计
**Milvus Collection**: `knowledge_chunk_vectors`

<lark-table rows="4" cols="3" header-row="true" column-widths="244,244,244">

  <lark-tr>
    <lark-td>
      字段
    </lark-td>
    <lark-td>
      类型
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      chunk_id
    </lark-td>
    <lark-td>
      Int64
    </lark-td>
    <lark-td>
      主键
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      user_id
    </lark-td>
    <lark-td>
      Int64
    </lark-td>
    <lark-td>
      用户ID
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      embedding
    </lark-td>
    <lark-td>
      FloatVector
    </lark-td>
    <lark-td>
      向量（dim=1536）
    </lark-td>
  </lark-tr>
</lark-table>

**索引建议**：
- HNSW(M=16, efConstruction=200)
---

## 第五章：核心功能实现方案
### 5.1 消息采集模块
#### Prompt设计
```plaintext
系统角色：你是知识整理助手，输出必须为JSON。

任务：从输入中提取 title/summary/tags/entities。

约束：
1) title <= 30字
2) summary <= 120字
3) tags 3~8个

```

#### 置信度计算
```plaintext
confidence = 0.45*model_score + 0.25*schema_pass + 0.2*keyword_hit

```

---

### 5.2 知识整理模块
#### Human-in-the-Loop机制
**状态机**：
```plaintext
PENDING_REVIEW → APPROVED → SYNCED
                ↘ REJECTED

```

**批量操作**：支持200条/批，每50条一个事务
---

### 5.3 知识检索模块（RAG）
#### 检索策略
1. Query理解与改写
1. 关键词召回（PG tsvector）
1. 向量召回（Milvus）
1. 融合去重（RRF）
1. 重排（cross-encoder）
1. 生成答案 + 引用
---

### 5.4 MCP服务模块
#### 接口设计
- `POST /mcp/v1/tools/knowledge.search`
- `POST /mcp/v1/tools/knowledge.create_draft`
- `POST /mcp/v1/tools/knowledge.confirm`
#### 授权机制
- client_id + client_secret获取access token
- scope粒度：knowledge:read, knowledge:write
- token哈希存储
---

### 5.5 飞书文档同步
**策略**：
- 按"用户-主题-月份"生成文档
- 单向主写：系统→飞书
- 版本管理：sync_version联动
---

## 第六章：Web端实现方案
### 6.1 页面路由

<lark-table rows="5" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      路由
    </lark-td>
    <lark-td>
      功能
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `/login`
    </lark-td>
    <lark-td>
      登录页
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `/drafts`
    </lark-td>
    <lark-td>
      AI草稿确认页
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `/knowledge`
    </lark-td>
    <lark-td>
      知识库列表
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      `/search`
    </lark-td>
    <lark-td>
      智能检索
    </lark-td>
  </lark-tr>
</lark-table>

---

### 6.2 技术方案
**状态管理**：Redux Toolkit + RTK Query
**API对接**：REST接口
**实时更新**：轮询 / SSE推送
---

## 第七章：AI服务集成
### 7.1 LLM服务
**场景模型分配**：
- 抽取模型（低成本）
- 生成模型（高质量）
**超时控制**：8-15秒
---

### 7.2 Embedding服务
- 统一维度：1536
- 批量向量化：每批32~64
---

### 7.3 Prompt管理
- 版本化：prompt_key + version
- A/B测试
- 自动回滚
---

## 第八章：性能与安全
### 8.1 性能指标

<lark-table rows="4" cols="2" header-row="true" column-widths="350,350">

  <lark-tr>
    <lark-td>
      指标
    </lark-td>
    <lark-td>
      目标
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      消息采集延迟
    </lark-td>
    <lark-td>
      < 5s
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      检索响应时间
    </lark-td>
    <lark-td>
      < 3s
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      批量确认
    </lark-td>
    <lark-td>
      20条 < 10s
    </lark-td>
  </lark-tr>
</lark-table>

---

### 8.2 安全措施
**认证**：飞书OAuth2 + JWT
**数据保护**：HTTPS + 敏感字段加密
**限流**：用户级100 req/min
---

## 第九章：部署与运维
### 9.1 部署架构
**初期单服务部署**：
```plaintext
单台服务器：
├── Web服务
├── API服务
├── PostgreSQL
├── Milvus
└── Redis

```

---

### 9.2 监控告警
- 服务健康检查
- API响应时间（P95）
- 错误率统计
---

## 第十章：开发计划
### 10.1 MVP功能
- ✅ 飞书机器人对话
- ✅ 内容采集
- ✅ AI提取
- ✅ Human-in-the-Loop确认
- ✅ 基础检索
- ✅ Web端基础页面
---

### 10.2 开发里程碑
**Phase 1：基础架构（2周）**
- 项目初始化
- 数据库设计
- 飞书机器人接入
**Phase 2：核心功能（3周）**
- 消息采集
- AI提取
- 知识整理
**Phase 3：检索与同步（2周）**
- RAG检索
- 飞书文档同步
- Web端开发
**Phase 4：MCP与上线（2周）**
- MCP服务
- 安全加固
- 灰度发布
---

## 更新记录

<lark-table rows="2" cols="4" header-row="true" column-widths="183,183,183,183">

  <lark-tr>
    <lark-td>
      版本
    </lark-td>
    <lark-td>
      日期
    </lark-td>
    <lark-td>
      更新内容
    </lark-td>
    <lark-td>
      作者
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      V1.0
    </lark-td>
    <lark-td>
      2026-03-28
    </lark-td>
    <lark-td>
      初始版本
    </lark-td>
    <lark-td>
      AI Assistant
    </lark-td>
  </lark-tr>
</lark-table>

