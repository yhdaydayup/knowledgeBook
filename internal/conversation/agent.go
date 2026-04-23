package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"knowledgebook/internal/llm"
)

// recordTriggerWords are the explicit phrases a user must say to trigger draft creation.
// If the user message does not contain any of these, create_knowledge_draft is blocked.
var recordTriggerWords = []string{
	"帮我记", "记一下", "记录下来", "记录一下", "保存这个", "保存一下",
	"帮我记录", "帮我保存", "帮我存", "记下来", "帮我存一下",
	"把这个记", "把这个保存", "记录下", "存下来", "存一下",
}

// hasRecordIntent checks whether the user message contains an explicit record trigger word.
func hasRecordIntent(userMessage string) bool {
	msg := strings.ToLower(userMessage)
	for _, trigger := range recordTriggerWords {
		if strings.Contains(msg, trigger) {
			return true
		}
	}
	// Also check recent history for explicit record intent (e.g. user said "记一下" in prior turn)
	return false
}

// guardCreateDraftResult is the JSON returned to the LLM when create_knowledge_draft is blocked.
func guardCreateDraftResult() string {
	return `{"blocked":true,"reason":"用户没有明确要求记录。请不要调用 create_knowledge_draft，而是正常回复用户，可以建议'要不要帮你记下来？'"}`
}

// isShortMessage checks if the message is very short (under N runes), which helps
// identify commands vs. knowledge-rich statements.
func isShortMessage(msg string, maxRunes int) bool {
	return utf8.RuneCountInString(strings.TrimSpace(msg)) <= maxRunes
}

// Agent implements a multi-turn tool-calling conversation loop.
type Agent struct {
	llm           llm.Client
	tools         []llm.ToolDefinition
	executor      *ToolExecutor
	history       HistoryStore
	systemPrompt  string
	maxToolRounds int
}

// NewAgent creates an Agent wired to the given LLM, tools, and history store.
func NewAgent(llmClient llm.Client, executor *ToolExecutor, history HistoryStore, systemPrompt string) *Agent {
	return &Agent{
		llm:           llmClient,
		tools:         AgentTools(),
		executor:      executor,
		history:       history,
		systemPrompt:  systemPrompt,
		maxToolRounds: 5,
	}
}

// AgentResult is the output of a single agent run.
type AgentResult struct {
	Reply        string         `json:"reply"`
	CardMarkdown string         `json:"cardMarkdown,omitempty"`
	ToolsCalled  []string       `json:"toolsCalled,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

// Run executes one user turn through the agent loop.
func (a *Agent) Run(ctx context.Context, session SessionContext, userMessage string) (*AgentResult, error) {
	userID := session.OpenID

	// 1. Load history
	historyMsgs, err := a.history.Load(ctx, userID, session.ChatID)
	if err != nil {
		log.Printf("[agent] load history failed: %v", err)
		historyMsgs = nil
	}

	// 2. Build messages: system + history + current user message
	now := time.Now()
	systemContent := a.systemPrompt + fmt.Sprintf("\n\n当前时间：%s", now.Format("2006年01月02日 15:04 (Monday)"))
	messages := make([]llm.ChatMessage, 0, len(historyMsgs)+2)
	messages = append(messages, llm.ChatMessage{Role: "system", Content: systemContent})
	for _, m := range historyMsgs {
		messages = append(messages, llm.ChatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  convertToolCalls(m.ToolCalls),
		})
	}
	messages = append(messages, llm.ChatMessage{Role: "user", Content: userMessage})

	// Track state for the result
	var toolsCalled []string
	var lastDraftJSON json.RawMessage
	createdDraft := false

	// 3. Agent loop
	for round := 0; round < a.maxToolRounds; round++ {
		resp, err := a.llm.Chat(ctx, llm.ChatRequest{
			Messages:    messages,
			Tools:       a.tools,
			Temperature: 0.3,
			MaxTokens:   2000,
		})
		if err != nil {
			return nil, fmt.Errorf("llm chat failed: %w", err)
		}

		// Case A: model produced a final text reply
		if resp.FinishReason == "stop" || len(resp.ToolCalls) == 0 {
			reply := strings.TrimSpace(resp.Content)
			if reply == "" {
				reply = "好的，我已处理完了。"
			}

			// Save to history: user message + assistant reply
			now := time.Now().Unix()
			_ = a.history.Append(ctx, userID, session.ChatID,
				Message{Role: "user", Content: userMessage, Timestamp: now},
				Message{Role: "assistant", Content: reply, Timestamp: now},
			)

			result := &AgentResult{
				Reply:       reply,
				ToolsCalled: toolsCalled,
				Data:        map[string]any{},
			}

			// If a draft was created, produce card markdown
			if createdDraft && lastDraftJSON != nil {
				result.Data["draftJSON"] = string(lastDraftJSON)
				result.CardMarkdown = extractCardMarkdown(lastDraftJSON)
				// Extract draft as map so upstream extractDraftID can find draft_id
				var draftMap map[string]any
				if json.Unmarshal(lastDraftJSON, &draftMap) == nil {
					result.Data["draft"] = draftMap
				}
			}

			return result, nil
		}

		// Case B: model requested tool calls
		assistantMsg := llm.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			toolsCalled = append(toolsCalled, tc.Function.Name)
			log.Printf("[agent] tool_call name=%s args=%s", tc.Function.Name, tc.Function.Arguments)

			// Guard: block create_knowledge_draft if user didn't express record intent
			if tc.Function.Name == "create_knowledge_draft" && !hasRecordIntent(userMessage) {
				log.Printf("[agent] BLOCKED create_knowledge_draft — no record intent in user message: %q", userMessage)
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					Content:    guardCreateDraftResult(),
					ToolCallID: tc.ID,
				})
				continue
			}

			toolResult, execErr := a.executor.Execute(ctx, session, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				toolResult = fmt.Sprintf(`{"error":"%s"}`, execErr.Error())
			}

			if (tc.Function.Name == "create_knowledge_draft" || tc.Function.Name == "revise_knowledge_draft") && execErr == nil {
				createdDraft = true
				lastDraftJSON = json.RawMessage(toolResult)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				Content:    toolResult,
				ToolCallID: tc.ID,
			})
		}
	}

	// Exceeded max rounds — return a fallback
	fallbackReply := "我处理这个请求时花了比较长时间，可能有点复杂。你可以再试一次或者换个说法。"
	_ = a.history.Append(ctx, userID, session.ChatID,
		Message{Role: "user", Content: userMessage, Timestamp: time.Now().Unix()},
		Message{Role: "assistant", Content: fallbackReply, Timestamp: time.Now().Unix()},
	)
	return &AgentResult{Reply: fallbackReply, ToolsCalled: toolsCalled}, nil
}

// convertToolCalls maps conversation.ToolCall to llm.ToolCall.
func convertToolCalls(calls []ToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = llm.ToolCall{
			ID:   c.ID,
			Type: c.Type,
			Function: llm.FunctionCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		}
	}
	return out
}

// extractCardMarkdown tries to produce a card markdown from drafts tool result.
func extractCardMarkdown(draftJSON json.RawMessage) string {
	var draft struct {
		ID                      int64    `json:"id"`
		Title                   string   `json:"title"`
		NormalizedSummary       string   `json:"normalized_summary"`
		NormalizedPoints        []string `json:"normalized_points"`
		RecommendedCategoryPath string   `json:"recommended_category_path"`
	}
	if err := json.Unmarshal(draftJSON, &draft); err != nil {
		return ""
	}
	if draft.ID == 0 {
		return ""
	}
	lines := []string{
		"# 待确认知识草稿",
		fmt.Sprintf("- 草稿ID：%d", draft.ID),
		fmt.Sprintf("- 标题：%s", draft.Title),
	}
	if draft.NormalizedSummary != "" {
		lines = append(lines, fmt.Sprintf("- 摘要：%s", draft.NormalizedSummary))
	}
	if len(draft.NormalizedPoints) > 0 {
		lines = append(lines, fmt.Sprintf("- 要点：%s", strings.Join(draft.NormalizedPoints, "；")))
	}
	if draft.RecommendedCategoryPath != "" {
		lines = append(lines, fmt.Sprintf("- 推荐分类：%s", draft.RecommendedCategoryPath))
	}
	lines = append(lines, "- 操作：确认保存 / 拒绝保存 / 修改分类")
	lines = append(lines, "- 提示：1 小时内确认，否则自动失效")
	return strings.Join(lines, "\n")
}
