package conversation

import "knowledgebook/internal/llm"

// AgentTools returns the tool definitions exposed to the LLM agent.
// Context fields (openId, userName, chatId) are injected by the executor
// and are NOT included in the schemas — the LLM only sees business parameters.
func AgentTools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "create_knowledge_draft",
				Description: "将用户提供的内容整理为知识草稿。当用户想记录一个结论、经验、规则、方案、排查结果时使用。草稿创建后处于待确认状态，需要用户确认后才正式保存。",
				Parameters: objectSchema([]string{"content"},
					stringField("title", "知识标题。如果用户没有明确标题，可以留空让系统自动生成。"),
					stringField("content", "知识的完整内容文本。"),
					stringField("categoryPath", "分类路径，如 '工作/默认项目/接口设计'。不确定时留空。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "confirm_knowledge_draft",
				Description: "确认保存一条待确认的知识草稿，将其正式写入知识库。当用户说'确认保存''保存吧''可以存'时使用。",
				Parameters: objectSchema([]string{},
					intField("draftId", "要确认的草稿ID。如果从对话上下文可以确定草稿，可以不传。"),
					stringField("categoryPath", "确认时修改分类路径。留空则使用草稿的推荐分类。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "reject_knowledge_draft",
				Description: "拒绝（丢弃）一条待确认的知识草稿。当用户说'不要保存''丢弃''算了'时使用。",
				Parameters: objectSchema([]string{},
					intField("draftId", "要拒绝的草稿ID。如果从对话上下文可以确定草稿，可以不传。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "update_draft_category",
				Description: "修改待确认草稿的分类路径。当用户说'改到 XX 分类''分类不对'时使用。",
				Parameters: objectSchema([]string{"categoryPath"},
					intField("draftId", "要修改分类的草稿ID。"),
					stringField("categoryPath", "新的完整分类路径，如 '软件开发/接口治理'。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "search_knowledge",
				Description: "搜索用户的知识库，返回匹配的知识条目和摘要。当用户询问之前记录过的内容、想查历史结论、问'之前怎么说的''有没有记录'时使用。",
				Parameters: objectSchema([]string{"query"},
					stringField("query", "搜索关键词或自然语言查询。"),
					stringField("category", "限定搜索的分类路径，可选。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "check_similarity",
				Description: "检查一段文本与知识库中已有内容的相似性和冲突关系。当用户问'是不是重复''有没有冲突'时使用。",
				Parameters: objectSchema([]string{"text"},
					stringField("text", "要检查相似性的文本内容。"),
					intField("draftId", "如果要检查某条草稿的相似性，传入草稿ID。"),
				),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "list_pending_drafts",
				Description: "列出当前会话中用户的所有待确认草稿。当需要确认但不确定是哪条草稿时使用。",
				Parameters: objectSchema([]string{}, /* no required fields — chatId is injected */),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        "revise_knowledge_draft",
				Description: "修订一条待确认草稿的内容。当用户要求补充、修改、完善刚刚创建的草稿内容时使用（例如'补充日期''标题改一下''内容加上XXX'）。会自动废弃旧草稿并创建修订后的新草稿。",
				Parameters: objectSchema([]string{"content"},
					intField("draftId", "要修订的草稿ID。如果从对话上下文可以确定，可以不传。"),
					stringField("title", "修订后的标题。留空保持不变。"),
					stringField("content", "修订后的完整内容。"),
					stringField("categoryPath", "修订后的分类路径。留空保持不变。"),
				),
			},
		},
	}
}

func objectSchema(required []string, properties ...map[string]any) map[string]any {
	props := map[string]any{}
	for _, p := range properties {
		for k, v := range p {
			props[k] = v
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringField(name, desc string) map[string]any {
	return map[string]any{name: map[string]any{"type": "string", "description": desc}}
}

func intField(name, desc string) map[string]any {
	return map[string]any{name: map[string]any{"type": "integer", "description": desc}}
}
