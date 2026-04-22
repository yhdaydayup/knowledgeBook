package api

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"knowledgebook/internal/service"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func (h *Handler) HandleMCP(ctx stdctx.Context, c *app.RequestContext) {
	var req mcpRequest
	if err := bindJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "invalid JSON"}})
		return
	}
	id := parseMCPID(req.ID)
	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo":      map[string]any{"name": "knowledgebook-embedded-mcp", "version": "0.1.0"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}})
	case "notifications/initialized":
		c.JSON(http.StatusOK, map[string]any{"ok": true})
	case "tools/list":
		c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": mcpTools()}})
	case "tools/call":
		var call mcpToolCallRequest
		if err := json.Unmarshal(req.Params, &call); err != nil {
			c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32602, Message: "invalid tool call params"}})
			return
		}
		result, err := h.callMCPTool(ctx, call)
		if err != nil {
			c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: err.Error()}})
			return
		}
		c.JSON(http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
	default:
		c.JSON(http.StatusBadRequest, mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32601, Message: "method not found"}})
	}
}

func parseMCPID(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func (h *Handler) callMCPTool(ctx stdctx.Context, call mcpToolCallRequest) (map[string]any, error) {
	args := call.Arguments
	switch call.Name {
	case "create_knowledge_draft":
		draft, err := h.Services.CreateDraft(ctx, serviceCreateRequest(args))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(draft, "draft created"), nil
	case "reject_knowledge_draft":
		if err := h.Services.RejectDraft(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "draftId")); err != nil {
			return nil, err
		}
		return mcpStructuredResult(map[string]any{"draftId": int64Arg(args, "draftId"), "status": "REJECTED"}, "draft rejected"), nil
	case "update_draft_category":
		draft, err := h.Services.UpdateDraftCategory(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "draftId"), stringArg(args, "categoryPath"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(draft, "draft category updated"), nil
	case "get_pending_draft_context":
		result, err := h.Services.ResolvePendingDraftContext(ctx, stringArg(args, "openId"), stringArg(args, "userName"), stringArg(args, "chatId"), stringArg(args, "messageId"), stringArg(args, "replyToMessageId"), int64Arg(args, "draftId"), stringArg(args, "quotedText"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(result, "pending context resolved"), nil
	case "list_pending_drafts":
		result, err := h.Services.ListPendingDrafts(ctx, stringArg(args, "openId"), stringArg(args, "userName"), stringArg(args, "chatId"), int(int64Arg(args, "limit")))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(result, "pending drafts loaded"), nil
	case "expire_pending_drafts":
		result, err := h.Services.ExpirePendingDrafts(ctx, int(int64Arg(args, "limit")))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(result, "pending drafts expired"), nil
	case "get_knowledge_draft":
		draft, err := h.Services.GetDraft(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "draftId"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(draft, "draft loaded"), nil
	case "confirm_knowledge_draft":
		item, err := h.Services.ApproveDraft(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "draftId"), stringArg(args, "categoryPath"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(item, "draft confirmed"), nil
	case "check_similarity":
		records, err := h.Services.CheckSimilarity(ctx, stringArg(args, "openId"), stringArg(args, "userName"), stringArg(args, "text"), int64Arg(args, "draftId"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(records, "similarity checked"), nil
	case "get_similarity_candidates":
		records, err := h.Services.ListSimilarityCandidates(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "draftId"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(records, "similarity candidates loaded"), nil
	case "search_knowledge":
		result, err := h.Services.SearchAnswer(ctx, stringArg(args, "openId"), stringArg(args, "userName"), stringArg(args, "query"), stringArg(args, "category"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(result, "search completed"), nil
	case "get_knowledge":
		item, err := h.Services.GetKnowledge(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "knowledgeId"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(item, "knowledge loaded"), nil
	case "get_related_knowledge":
		items, err := h.Services.GetRelatedKnowledge(ctx, stringArg(args, "openId"), stringArg(args, "userName"), int64Arg(args, "knowledgeId"))
		if err != nil {
			return nil, err
		}
		return mcpStructuredResult(items, "related knowledge loaded"), nil
	default:
		return nil, http.ErrNotSupported
	}
}

func mcpStructuredResult(data interface{}, text string) map[string]any {
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": text}},
		"structuredContent": data,
	}
}

func stringArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func int64Arg(args map[string]interface{}, key string) int64 {
	value, ok := args[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func serviceCreateRequest(args map[string]interface{}) service.CreateKnowledgeRequest {
	request := service.CreateKnowledgeRequest{
		OpenID:          stringArg(args, "openId"),
		UserName:        stringArg(args, "userName"),
		Title:           stringArg(args, "title"),
		Content:         stringArg(args, "content"),
		CategoryPath:    stringArg(args, "categoryPath"),
		Source:          defaultMCPSource(stringArg(args, "source")),
		ChatID:          stringArg(args, "chatId"),
		SourceMessageID: stringArg(args, "sourceMessageId"),
		ReplyMessageID:  stringArg(args, "replyToMessageId"),
		QuotedText:      stringArg(args, "quotedText"),
	}
	if rawTags, ok := args["tags"].([]interface{}); ok {
		request.Tags = make([]string, 0, len(rawTags))
		for _, tag := range rawTags {
			if text, ok := tag.(string); ok && strings.TrimSpace(text) != "" {
				request.Tags = append(request.Tags, strings.TrimSpace(text))
			}
		}
	}
	return request
}

func defaultMCPSource(source string) string {
	if source == "" {
		return "EMBEDDED_MCP"
	}
	return source
}

func mcpTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "create_knowledge_draft",
			Description: "Create a structured knowledge draft from raw text.",
			InputSchema: objectSchema(requiredFields("openId", "content"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("title", "string"), fieldSchema("content", "string"), fieldSchema("categoryPath", "string"), arrayStringSchema("tags"), fieldSchema("source", "string"), fieldSchema("chatId", "string"), fieldSchema("sourceMessageId", "string"), fieldSchema("replyToMessageId", "string"), fieldSchema("quotedText", "string")),
		},
		{
			Name:        "reject_knowledge_draft",
			Description: "Reject a pending knowledge draft.",
			InputSchema: objectSchema(requiredFields("openId", "draftId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("draftId", "integer")),
		},
		{
			Name:        "update_draft_category",
			Description: "Update category for a pending draft.",
			InputSchema: objectSchema(requiredFields("openId", "draftId", "categoryPath"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("draftId", "integer"), fieldSchema("categoryPath", "string")),
		},
		{
			Name:        "get_pending_draft_context",
			Description: "Resolve the current pending draft from chat/message context.",
			InputSchema: objectSchema(requiredFields("openId", "chatId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("chatId", "string"), fieldSchema("messageId", "string"), fieldSchema("replyToMessageId", "string")),
		},
		{
			Name:        "list_pending_drafts",
			Description: "List pending drafts in one chat.",
			InputSchema: objectSchema(requiredFields("openId", "chatId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("chatId", "string"), fieldSchema("limit", "integer")),
		},
		{
			Name:        "expire_pending_drafts",
			Description: "Expire pending drafts by TTL batch job.",
			InputSchema: objectSchema([]string{}, fieldSchema("limit", "integer")),
		},
		{
			Name:        "get_knowledge_draft",
			Description: "Load a draft by ID.",
			InputSchema: objectSchema(requiredFields("openId", "draftId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("draftId", "integer")),
		},
		{
			Name:        "confirm_knowledge_draft",
			Description: "Confirm a draft and create knowledge.",
			InputSchema: objectSchema(requiredFields("openId", "draftId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("draftId", "integer"), fieldSchema("categoryPath", "string")),
		},
		{
			Name:        "check_similarity",
			Description: "Check if text or draft is similar to existing knowledge.",
			InputSchema: objectSchema([]string{}, fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("text", "string"), fieldSchema("draftId", "integer")),
		},
		{
			Name:        "get_similarity_candidates",
			Description: "Load stored similarity candidates for a draft.",
			InputSchema: objectSchema(requiredFields("openId", "draftId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("draftId", "integer")),
		},
		{
			Name:        "search_knowledge",
			Description: "Search knowledge and return answer plus evidence.",
			InputSchema: objectSchema(requiredFields("openId", "query"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("query", "string"), fieldSchema("category", "string")),
		},
		{
			Name:        "get_knowledge",
			Description: "Load a knowledge item by ID.",
			InputSchema: objectSchema(requiredFields("openId", "knowledgeId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("knowledgeId", "integer")),
		},
		{
			Name:        "get_related_knowledge",
			Description: "Load related knowledge suggestions for a knowledge item.",
			InputSchema: objectSchema(requiredFields("openId", "knowledgeId"), fieldSchema("openId", "string"), fieldSchema("userName", "string"), fieldSchema("knowledgeId", "integer")),
		},
	}
}

func objectSchema(required []string, properties ...map[string]any) map[string]any {
	props := map[string]any{}
	for _, property := range properties {
		for key, value := range property {
			props[key] = value
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func fieldSchema(name, fieldType string) map[string]any {
	return map[string]any{name: map[string]any{"type": fieldType}}
}

func arrayStringSchema(name string) map[string]any {
	return map[string]any{name: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}
}

func requiredFields(fields ...string) []string {
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			items = append(items, field)
		}
	}
	return items
}
