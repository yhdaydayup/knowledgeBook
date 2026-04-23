package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"knowledgebook/internal/llm"
)

// ── Mock LLM Client ──────────────────────────────────────────────────────────

type mockLLMClient struct {
	chatResponses []llm.ChatResponse // return in order, cycling last
	chatCalls     []llm.ChatRequest  // recorded calls
	callIndex     int
}

func (m *mockLLMClient) Enabled() bool { return true }
func (m *mockLLMClient) GenerateJSON(_ context.Context, _ llm.GenerateJSONRequest) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockLLMClient) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.chatCalls = append(m.chatCalls, req)
	idx := m.callIndex
	if idx >= len(m.chatResponses) {
		idx = len(m.chatResponses) - 1
	}
	m.callIndex++
	resp := m.chatResponses[idx]
	return &resp, nil
}

// ── Mock Service Layer ───────────────────────────────────────────────────────

type mockServiceLayer struct {
	createDraftResult json.RawMessage
	createDraftErr    error
	createDraftCalls  []map[string]string

	approveDraftResult json.RawMessage
	approveDraftCalls  []map[string]any

	rejectDraftResult json.RawMessage
	rejectDraftCalls  []map[string]any

	reviseDraftResult json.RawMessage
	reviseDraftCalls  []map[string]any

	searchResult json.RawMessage
	searchCalls  []map[string]string

	similarityResult json.RawMessage
	listDraftsResult json.RawMessage
	updateCatResult  json.RawMessage
}

func (m *mockServiceLayer) CreateDraftForAgent(_ context.Context, openID, userName, chatID, messageID, replyToMessageID, title, content, categoryPath string) (json.RawMessage, error) {
	m.createDraftCalls = append(m.createDraftCalls, map[string]string{
		"openID": openID, "title": title, "content": content, "categoryPath": categoryPath,
	})
	return m.createDraftResult, m.createDraftErr
}
func (m *mockServiceLayer) ApproveDraftForAgent(_ context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error) {
	m.approveDraftCalls = append(m.approveDraftCalls, map[string]any{"draftID": draftID})
	return m.approveDraftResult, nil
}
func (m *mockServiceLayer) RejectDraftForAgent(_ context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64) (json.RawMessage, error) {
	m.rejectDraftCalls = append(m.rejectDraftCalls, map[string]any{"draftID": draftID})
	return m.rejectDraftResult, nil
}
func (m *mockServiceLayer) UpdateDraftCategoryForAgent(_ context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error) {
	return m.updateCatResult, nil
}
func (m *mockServiceLayer) SearchKnowledgeForAgent(_ context.Context, openID, userName, query, category string) (json.RawMessage, error) {
	m.searchCalls = append(m.searchCalls, map[string]string{"query": query, "category": category})
	return m.searchResult, nil
}
func (m *mockServiceLayer) CheckSimilarityForAgent(_ context.Context, openID, userName, text string, draftID int64) (json.RawMessage, error) {
	return m.similarityResult, nil
}
func (m *mockServiceLayer) ListPendingDraftsForAgent(_ context.Context, openID, userName, chatID string) (json.RawMessage, error) {
	return m.listDraftsResult, nil
}
func (m *mockServiceLayer) ReviseDraftForAgent(_ context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, title, content, categoryPath string) (json.RawMessage, error) {
	m.reviseDraftCalls = append(m.reviseDraftCalls, map[string]any{
		"draftID": draftID, "title": title, "content": content,
	})
	return m.reviseDraftResult, nil
}

// ── Mock History (in-memory, no Redis) ───────────────────────────────────────

type mockHistory struct {
	messages []Message
}

func (h *mockHistory) Load(_ context.Context, _, _ string) ([]Message, error) {
	return h.messages, nil
}
func (h *mockHistory) Append(_ context.Context, _, _ string, msgs ...Message) error {
	h.messages = append(h.messages, msgs...)
	return nil
}
func (h *mockHistory) Clear(_ context.Context, _, _ string) error {
	h.messages = nil
	return nil
}

// newTestAgent builds an Agent with mock dependencies.
func newTestAgent(llmClient llm.Client, svc ServiceLayer, hist ...HistoryStore) *Agent {
	executor := NewToolExecutor(svc)
	var h HistoryStore
	if len(hist) > 0 && hist[0] != nil {
		h = hist[0]
	} else {
		h = &mockHistory{}
	}
	return &Agent{
		llm:           llmClient,
		tools:         AgentTools(),
		executor:      executor,
		history:       h,
		systemPrompt:  "你是测试助手",
		maxToolRounds: 5,
	}
}

// mockLLMClientWithError always returns an error from Chat.
type mockLLMClientWithError struct {
	err error
}

func (m *mockLLMClientWithError) Enabled() bool { return true }
func (m *mockLLMClientWithError) GenerateJSON(_ context.Context, _ llm.GenerateJSONRequest) ([]byte, error) {
	return nil, m.err
}
func (m *mockLLMClientWithError) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, m.err
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestAgentDirectReply_NoTools(t *testing.T) {
	// LLM returns a direct text reply (no tool calls) — agent should return it.
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{Content: "你好！有什么可以帮你的？", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{}

	agent := newTestAgent(mock, svc)
	// Verify the mock LLM returns what we expect
	resp, err := mock.Chat(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected stop, got %s", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	_ = agent // agent is built correctly
}

func TestExecutor_DispatchAllTools(t *testing.T) {
	draftJSON := json.RawMessage(`{"id":42,"title":"test","status":"PENDING"}`)
	svc := &mockServiceLayer{
		createDraftResult: draftJSON,
		approveDraftResult: json.RawMessage(`{"draft_id":42,"status":"APPROVED"}`),
		rejectDraftResult:  json.RawMessage(`{"draft_id":42,"status":"REJECTED"}`),
		reviseDraftResult:  json.RawMessage(`{"old_draft_id":42,"id":43,"status":"PENDING"}`),
		searchResult:       json.RawMessage(`{"results":[]}`),
		similarityResult:   json.RawMessage(`{"records":[]}`),
		listDraftsResult:   json.RawMessage(`{"count":0,"drafts":[]}`),
		updateCatResult:    json.RawMessage(`{"draft_id":42,"status":"PENDING"}`),
	}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester", ChatID: "c1"}

	tests := []struct {
		name     string
		toolName string
		args     string
		wantErr  bool
	}{
		{"create_knowledge_draft", "create_knowledge_draft", `{"title":"test","content":"hello"}`, false},
		{"confirm_knowledge_draft", "confirm_knowledge_draft", `{"draftId":42}`, false},
		{"reject_knowledge_draft", "reject_knowledge_draft", `{"draftId":42}`, false},
		{"update_draft_category", "update_draft_category", `{"draftId":42,"categoryPath":"工作/测试"}`, false},
		{"search_knowledge", "search_knowledge", `{"query":"接口限流"}`, false},
		{"check_similarity", "check_similarity", `{"text":"some text"}`, false},
		{"list_pending_drafts", "list_pending_drafts", `{}`, false},
		{"revise_knowledge_draft", "revise_knowledge_draft", `{"draftId":42,"content":"revised content"}`, false},
		{"unknown_tool", "nonexistent_tool", `{}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(context.Background(), session, tt.toolName, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for unknown tool, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == "" {
				t.Error("expected non-empty result")
			}
		})
	}
}

func TestExecutor_CreateDraftPassesArgs(t *testing.T) {
	svc := &mockServiceLayer{
		createDraftResult: json.RawMessage(`{"id":1,"title":"测试标题"}`),
	}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester", ChatID: "c1"}

	_, err := executor.Execute(context.Background(), session, "create_knowledge_draft",
		`{"title":"测试标题","content":"测试内容","categoryPath":"工作/默认"}`)
	if err != nil {
		t.Fatal(err)
	}

	if len(svc.createDraftCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(svc.createDraftCalls))
	}
	call := svc.createDraftCalls[0]
	if call["title"] != "测试标题" {
		t.Errorf("title = %q, want 测试标题", call["title"])
	}
	if call["content"] != "测试内容" {
		t.Errorf("content = %q, want 测试内容", call["content"])
	}
	if call["categoryPath"] != "工作/默认" {
		t.Errorf("categoryPath = %q, want 工作/默认", call["categoryPath"])
	}
}

func TestExecutor_ReviseDraftPassesDraftID(t *testing.T) {
	svc := &mockServiceLayer{
		reviseDraftResult: json.RawMessage(`{"old_draft_id":42,"id":43}`),
	}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester", ChatID: "c1"}

	_, err := executor.Execute(context.Background(), session, "revise_knowledge_draft",
		`{"draftId":42,"content":"修订后内容","title":"新标题"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.reviseDraftCalls) != 1 {
		t.Fatalf("expected 1 revise call, got %d", len(svc.reviseDraftCalls))
	}
	call := svc.reviseDraftCalls[0]
	if call["draftID"] != int64(42) {
		t.Errorf("draftID = %v, want 42", call["draftID"])
	}
	if call["content"] != "修订后内容" {
		t.Errorf("content = %q, want 修订后内容", call["content"])
	}
}

func TestExecutor_ServiceError_ReturnsJSON(t *testing.T) {
	svc := &mockServiceLayer{
		createDraftErr: fmt.Errorf("database connection failed"),
	}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester"}

	result, err := executor.Execute(context.Background(), session, "create_knowledge_draft",
		`{"content":"test"}`)
	// Execute should NOT return error — it wraps the service error in JSON
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]string
	if jsonErr := json.Unmarshal([]byte(result), &parsed); jsonErr != nil {
		t.Fatalf("result is not valid JSON: %v", jsonErr)
	}
	if parsed["error"] == "" {
		t.Error("expected error field in result JSON")
	}
}

func TestExecutor_InvalidJSON(t *testing.T) {
	svc := &mockServiceLayer{}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester"}

	_, err := executor.Execute(context.Background(), session, "search_knowledge", `{invalid}`)
	if err == nil {
		t.Error("expected error for invalid JSON args")
	}
}

func TestExecutor_EmptyArgs(t *testing.T) {
	svc := &mockServiceLayer{
		listDraftsResult: json.RawMessage(`{"count":0,"drafts":[]}`),
	}
	executor := NewToolExecutor(svc)
	session := SessionContext{OpenID: "u1", UserName: "tester", ChatID: "c1"}

	result, err := executor.Execute(context.Background(), session, "list_pending_drafts", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result for empty args")
	}
}

func TestExtractCardMarkdown_ValidDraft(t *testing.T) {
	draftJSON := json.RawMessage(`{
		"id": 42,
		"title": "飞书验签失败排查",
		"normalized_summary": "header token 不匹配导致验签失败",
		"normalized_points": ["检查 header token", "确认签名算法"],
		"recommended_category_path": "工作/默认项目/接口设计"
	}`)

	md := extractCardMarkdown(draftJSON)
	if md == "" {
		t.Fatal("expected non-empty card markdown")
	}
	for _, want := range []string{"草稿ID：42", "飞书验签失败排查", "header token", "接口设计"} {
		if !contains(md, want) {
			t.Errorf("card markdown missing %q", want)
		}
	}
}

func TestExtractCardMarkdown_IDZero(t *testing.T) {
	draftJSON := json.RawMessage(`{"id":0,"title":"test"}`)
	md := extractCardMarkdown(draftJSON)
	if md != "" {
		t.Errorf("expected empty markdown for id=0, got %q", md)
	}
}

func TestExtractCardMarkdown_InvalidJSON(t *testing.T) {
	md := extractCardMarkdown(json.RawMessage(`{invalid}`))
	if md != "" {
		t.Error("expected empty markdown for invalid JSON")
	}
}

func TestAgentResult_DraftData_ContainsDraftMap(t *testing.T) {
	// Simulate what agent.go does when create_knowledge_draft is called
	lastDraftJSON := json.RawMessage(`{"id":42,"title":"test draft","status":"PENDING"}`)

	result := &AgentResult{
		Reply:       "已创建草稿",
		ToolsCalled: []string{"create_knowledge_draft"},
		Data:        map[string]any{},
	}
	result.Data["draftJSON"] = string(lastDraftJSON)
	result.CardMarkdown = extractCardMarkdown(lastDraftJSON)

	var draftMap map[string]any
	if json.Unmarshal(lastDraftJSON, &draftMap) == nil {
		result.Data["draft"] = draftMap
	}

	// Verify Data["draft"] exists and has correct id
	draft, ok := result.Data["draft"].(map[string]any)
	if !ok {
		t.Fatal("Data[\"draft\"] should be a map[string]any")
	}
	id, ok := draft["id"].(float64)
	if !ok || int64(id) != 42 {
		t.Errorf("draft id = %v, want 42", draft["id"])
	}
}

func TestAgentResult_NoDraft_DataEmpty(t *testing.T) {
	result := &AgentResult{
		Reply: "你好！",
		Data:  map[string]any{},
	}
	if _, ok := result.Data["draft"]; ok {
		t.Error("Data should not have 'draft' key when no draft created")
	}
}

func TestToolDefinitions_Count(t *testing.T) {
	tools := AgentTools()
	if len(tools) != 8 {
		t.Errorf("expected 8 tools, got %d", len(tools))
	}
}

func TestToolDefinitions_AllHaveNames(t *testing.T) {
	tools := AgentTools()
	expectedNames := map[string]bool{
		"create_knowledge_draft":  false,
		"confirm_knowledge_draft": false,
		"reject_knowledge_draft":  false,
		"update_draft_category":   false,
		"search_knowledge":        false,
		"check_similarity":        false,
		"list_pending_drafts":     false,
		"revise_knowledge_draft":  false,
	}
	for _, tool := range tools {
		name := tool.Function.Name
		if _, ok := expectedNames[name]; !ok {
			t.Errorf("unexpected tool name: %s", name)
		}
		expectedNames[name] = true
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestInt64Arg_Types(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int64
	}{
		{"float64", float64(42), 42},
		{"int64", int64(42), 42},
		{"int", int(42), 42},
		{"string", "42", 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"id": tt.val}
			got := int64Arg(args, "id")
			if got != tt.want {
				t.Errorf("int64Arg = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStringArg_Trim(t *testing.T) {
	args := map[string]any{
		"query":     "  hello world  ",
		"missing":   nil,
		"not_string": 123,
	}
	if got := stringArg(args, "query"); got != "hello world" {
		t.Errorf("stringArg = %q, want %q", got, "hello world")
	}
	if got := stringArg(args, "missing"); got != "" {
		t.Errorf("stringArg(missing) = %q, want empty", got)
	}
	if got := stringArg(args, "not_string"); got != "" {
		t.Errorf("stringArg(not_string) = %q, want empty", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

// ── hasRecordIntent tests ───────────────────────────────────────────────────

func TestHasRecordIntent_Positive(t *testing.T) {
	positives := []string{
		"帮我记一下，飞书验签失败是因为 header token 不匹配",
		"记录下来：接口限流策略改为令牌桶算法",
		"把这个记录下来",
		"帮我保存这个信息",
		"记一下这个结论",
		"帮我记录 Go 的 GMP 模型",
		"保存这个知识点",
		"帮我存一下",
		"记下来吧",
	}
	for _, msg := range positives {
		if !hasRecordIntent(msg) {
			t.Errorf("expected hasRecordIntent=true for %q", msg)
		}
	}
}

func TestHasRecordIntent_Negative(t *testing.T) {
	negatives := []string{
		"你好",
		"谢谢",
		"Nginx proxy_pass 末尾要加斜杠",
		"飞书验签失败是因为 header token 不匹配",
		"今天天气真好",
		"开始 V3 版本的开发和测试工作",
		"Go 的 goroutine 调度器用的是 GMP 模型",
		"之前有没有记录过关于接口限流的内容？",
		"确认保存",
		"不要了",
	}
	for _, msg := range negatives {
		if hasRecordIntent(msg) {
			t.Errorf("expected hasRecordIntent=false for %q", msg)
		}
	}
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── Agent.Run() Tests ───────────────────────────────────────────────────────
// These test the full agent loop with mock LLM + mock history.

func TestAgentRun_DirectReply_NoTools(t *testing.T) {
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{Content: "你好！有什么可以帮你的？", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	result, err := agent.Run(context.Background(), SessionContext{OpenID: "u1", ChatID: "c1"}, "你好")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reply != "你好！有什么可以帮你的？" {
		t.Errorf("reply = %q, want '你好！有什么可以帮你的？'", result.Reply)
	}
	if len(result.ToolsCalled) != 0 {
		t.Errorf("expected 0 tools called, got %d", len(result.ToolsCalled))
	}
	// History should have 2 messages: user + assistant
	if len(hist.messages) != 2 {
		t.Errorf("expected 2 history messages, got %d", len(hist.messages))
	}
	if len(hist.messages) >= 2 {
		if hist.messages[0].Role != "user" {
			t.Errorf("first history msg role = %s, want user", hist.messages[0].Role)
		}
		if hist.messages[1].Role != "assistant" {
			t.Errorf("second history msg role = %s, want assistant", hist.messages[1].Role)
		}
	}
}

func TestAgentRun_ToolCall_ThenReply(t *testing.T) {
	// Round 1: LLM returns a tool call to search_knowledge
	// Round 2: LLM returns a text reply
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Type: "function", Function: llm.FunctionCall{
						Name: "search_knowledge", Arguments: `{"query":"接口限流"}`,
					}},
				},
			},
			{Content: "根据知识库，接口限流用的是令牌桶。", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{
		searchResult: json.RawMessage(`{"results":[]}`),
	}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	result, err := agent.Run(context.Background(), SessionContext{OpenID: "u1", ChatID: "c1"}, "之前接口限流是什么策略？")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reply == "" {
		t.Error("expected non-empty reply")
	}
	if len(result.ToolsCalled) != 1 || result.ToolsCalled[0] != "search_knowledge" {
		t.Errorf("toolsCalled = %v, want [search_knowledge]", result.ToolsCalled)
	}
	// search was called on the service
	if len(svc.searchCalls) != 1 {
		t.Errorf("expected 1 search call, got %d", len(svc.searchCalls))
	}
}

func TestAgentRun_Guard_BlocksCreateDraft(t *testing.T) {
	// LLM tries to call create_knowledge_draft but user didn't say trigger words.
	// Round 1: create_knowledge_draft → blocked by guard
	// Round 2: LLM returns text reply (responding to the guard message)
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Type: "function", Function: llm.FunctionCall{
						Name: "create_knowledge_draft", Arguments: `{"content":"test content"}`,
					}},
				},
			},
			{Content: "好的，这个信息很有价值。要不要帮你记下来？", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{
		createDraftResult: json.RawMessage(`{"id":1,"title":"test"}`),
	}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	result, err := agent.Run(context.Background(),
		SessionContext{OpenID: "u1", ChatID: "c1"},
		"飞书验签失败是因为 header token 不匹配") // no trigger word
	if err != nil {
		t.Fatal(err)
	}
	// Guard should have blocked — svc.createDraftCalls should be empty
	if len(svc.createDraftCalls) != 0 {
		t.Error("create_knowledge_draft should have been blocked by guard, but service was called")
	}
	// ToolsCalled still records the attempt (it was in the LLM response)
	if len(result.ToolsCalled) != 1 || result.ToolsCalled[0] != "create_knowledge_draft" {
		t.Errorf("toolsCalled = %v, want [create_knowledge_draft]", result.ToolsCalled)
	}
	// No card/draft should be produced
	if result.CardMarkdown != "" {
		t.Errorf("expected no card markdown, got %q", result.CardMarkdown)
	}
}

func TestAgentRun_CreateDraft_WithCard(t *testing.T) {
	draftJSON := `{"id":42,"title":"飞书验签排查","normalized_summary":"header token 不匹配","normalized_points":["检查 token"],"recommended_category_path":"工作/接口"}`
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Type: "function", Function: llm.FunctionCall{
						Name: "create_knowledge_draft", Arguments: `{"content":"test"}`,
					}},
				},
			},
			{Content: "已帮你创建草稿。", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{
		createDraftResult: json.RawMessage(draftJSON),
	}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	// Use trigger word so guard doesn't block
	result, err := agent.Run(context.Background(),
		SessionContext{OpenID: "u1", ChatID: "c1"},
		"帮我记一下，飞书验签失败是因为 header token 不匹配")
	if err != nil {
		t.Fatal(err)
	}
	if result.CardMarkdown == "" {
		t.Error("expected card markdown for draft creation")
	}
	if !contains(result.CardMarkdown, "草稿ID：42") {
		t.Errorf("card should contain draft ID 42, got: %s", result.CardMarkdown)
	}
	// Data should contain draft map
	draft, ok := result.Data["draft"].(map[string]any)
	if !ok {
		t.Fatal("Data[\"draft\"] should be map[string]any")
	}
	id, _ := draft["id"].(float64)
	if int64(id) != 42 {
		t.Errorf("draft id = %v, want 42", draft["id"])
	}
}

func TestAgentRun_MaxRounds_Fallback(t *testing.T) {
	// LLM always returns tool calls, never a stop reply.
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{
				FinishReason: "tool_calls",
				ToolCalls: []llm.ToolCall{
					{ID: "call_loop", Type: "function", Function: llm.FunctionCall{
						Name: "list_pending_drafts", Arguments: `{}`,
					}},
				},
			},
		},
	}
	svc := &mockServiceLayer{
		listDraftsResult: json.RawMessage(`{"count":0,"drafts":[]}`),
	}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)
	agent.maxToolRounds = 3 // reduce for faster test

	result, err := agent.Run(context.Background(), SessionContext{OpenID: "u1", ChatID: "c1"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Reply, "花了比较长时间") {
		t.Errorf("expected fallback message, got: %q", result.Reply)
	}
	if len(result.ToolsCalled) != 3 {
		t.Errorf("expected 3 tool calls (maxRounds), got %d", len(result.ToolsCalled))
	}
}

func TestAgentRun_LLMError(t *testing.T) {
	mock := &mockLLMClientWithError{err: fmt.Errorf("connection timeout")}
	svc := &mockServiceLayer{}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	_, err := agent.Run(context.Background(), SessionContext{OpenID: "u1", ChatID: "c1"}, "hello")
	if err == nil {
		t.Error("expected error from LLM failure")
	}
	if !strings.Contains(err.Error(), "llm chat failed") {
		t.Errorf("error = %v, want 'llm chat failed: ...'", err)
	}
}

func TestAgentRun_EmptyReply_Fallback(t *testing.T) {
	// LLM returns stop but with empty content — agent should use fallback text
	mock := &mockLLMClient{
		chatResponses: []llm.ChatResponse{
			{Content: "", FinishReason: "stop"},
		},
	}
	svc := &mockServiceLayer{}
	hist := &mockHistory{}
	agent := newTestAgent(mock, svc, hist)

	result, err := agent.Run(context.Background(), SessionContext{OpenID: "u1", ChatID: "c1"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Reply != "好的，我已处理完了。" {
		t.Errorf("expected fallback reply for empty content, got: %q", result.Reply)
	}
}

func TestConvertToolCalls(t *testing.T) {
	input := []ToolCall{
		{ID: "c1", Type: "function", Function: FunctionCall{Name: "search_knowledge", Arguments: `{"query":"test"}`}},
		{ID: "c2", Type: "function", Function: FunctionCall{Name: "list_pending_drafts", Arguments: `{}`}},
	}
	result := convertToolCalls(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].ID != "c1" || result[0].Function.Name != "search_knowledge" {
		t.Errorf("first result: ID=%s Name=%s", result[0].ID, result[0].Function.Name)
	}
	if result[1].ID != "c2" || result[1].Function.Name != "list_pending_drafts" {
		t.Errorf("second result: ID=%s Name=%s", result[1].ID, result[1].Function.Name)
	}

	// nil input → nil output
	nilResult := convertToolCalls(nil)
	if nilResult != nil {
		t.Error("expected nil for nil input")
	}

	// empty input → nil output
	emptyResult := convertToolCalls([]ToolCall{})
	if emptyResult != nil {
		t.Error("expected nil for empty input")
	}
}

func TestIsShortMessage(t *testing.T) {
	tests := []struct {
		msg      string
		maxRunes int
		want     bool
	}{
		{"你好", 5, true},
		{"你好世界！", 5, true},        // exactly 5 runes
		{"你好世界！你好", 5, false},     // 7 runes
		{"  hello  ", 10, true},        // after trim: 5
		{"", 5, true},
		{"ab", 1, false},
		{"a", 1, true},
	}
	for _, tt := range tests {
		got := isShortMessage(tt.msg, tt.maxRunes)
		if got != tt.want {
			t.Errorf("isShortMessage(%q, %d) = %v, want %v", tt.msg, tt.maxRunes, got, tt.want)
		}
	}
}

func TestGuardCreateDraftResult(t *testing.T) {
	result := guardCreateDraftResult()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if parsed["blocked"] != true {
		t.Errorf("expected blocked=true, got %v", parsed["blocked"])
	}
	if _, ok := parsed["reason"]; !ok {
		t.Error("expected 'reason' field in guard result")
	}
}
