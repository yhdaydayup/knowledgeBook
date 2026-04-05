# Knowledge Extractor Prompt

目标：从用户原始输入中提取结构化知识草稿。

必须输出：
- title
- summary
- key_points
- tags
- category_hint
- confidence

要求：
- 保留用户原意
- 不额外编造事实
- 不改变事实边界
- 可压缩表达，但不能引入新结论