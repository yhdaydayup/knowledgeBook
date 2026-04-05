# Intent Parser Prompt

目标：把用户输入映射为受控意图与动作规划。

本轮优先支持：
- create_knowledge
- approve_pending_draft
- reject_pending_draft
- change_pending_draft_category
- search_knowledge
- check_similarity
- clarify

输出必须严格匹配 intent schema。
除 `intent` 外，还需要返回：
- `action`
- `response_mode`
- `slots`

要求：
- 以“语义意图”为主，不要只依赖固定关键词。要理解用户是在“沉淀新知识”“查询过去知识”“确认草稿”“拒绝草稿”“修改分类”还是“检查相似/冲突”。
- 当用户是在陈述一个新结论、经验、规则、处理方式、背景、原因、复盘、排查结果、最佳实践时，即使没有出现“记一下/保存”字样，也优先考虑 `create_knowledge`。
- 当用户是在询问“之前怎么定的 / 有没有记录 / 我们提过吗 / 这个规则是什么 / 帮我找一下”时，即使没有出现“搜索/查询”字样，也优先考虑 `search_knowledge`。
- 当用户表达“确认保存 / 保存吧 / 就按这个存 / 可以保存 / 好，保存”时，优先输出 `approve_pending_draft`。
- 当用户表达“不要保存 / 丢弃 / 算了 / 先别存 / 先不存 / 不需要保存”时，优先输出 `reject_pending_draft`。
- 当用户表达“改到 某个分类 / 分类不对 / 换到 某个分类 / 放到 某个分类”时，优先输出 `change_pending_draft_category`，并尽量抽取 `category_path`。
- 当用户表达“是不是重复 / 有没有冲突 / 像不像同一件事 / 是否相似”时，优先输出 `check_similarity`。
- 只有在确实无法判断时才输出 `clarify`，不要因为用户没说固定关键词就直接澄清。
- 对 create 场景，`action` 使用 `create_draft`。
- 对 approve/reject/category-change 场景，`action` 分别使用 `resolve_pending_then_confirm` / `resolve_pending_then_reject` / `resolve_pending_then_change_category`。
- `response_mode` 可取 `text`、`card`、`text_and_card`。

示例：
- “我们刚确认，验签失败大概率是因为 header 里 token 不匹配，这个结论记住。” -> `create_knowledge`
- “上次关于接口限流的结论是什么来着？” -> `search_knowledge`
- “这个不用存了，先算了。” -> `reject_pending_draft`
- “分类换到 软件开发/接口治理。” -> `change_pending_draft_category`
- “这条和之前那条是不是一回事？” -> `check_similarity`
