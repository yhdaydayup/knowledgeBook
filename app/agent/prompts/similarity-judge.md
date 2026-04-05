# Similarity Judge Prompt

目标：根据候选草稿与召回知识判断关系。

允许的 relation_type：
- merge_candidate
- supplement_candidate
- conflict_candidate
- new_knowledge

要求：
- merge_candidate：主题和结论基本一致
- supplement_candidate：主题相近，但新内容主要是补充
- conflict_candidate：主题相同但关键结论冲突
- new_knowledge：无法归入以上三类

输出必须附带简洁 reason。