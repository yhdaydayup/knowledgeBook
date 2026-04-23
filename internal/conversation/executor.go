package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ServiceLayer is the subset of service.Services methods the executor needs.
// Using an interface avoids a circular import between conversation and service.
type ServiceLayer interface {
	CreateDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID, title, content, categoryPath string) (json.RawMessage, error)
	ApproveDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error)
	RejectDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64) (json.RawMessage, error)
	UpdateDraftCategoryForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error)
	SearchKnowledgeForAgent(ctx context.Context, openID, userName, query, category string) (json.RawMessage, error)
	CheckSimilarityForAgent(ctx context.Context, openID, userName, text string, draftID int64) (json.RawMessage, error)
	ListPendingDraftsForAgent(ctx context.Context, openID, userName, chatID string) (json.RawMessage, error)
	ReviseDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, title, content, categoryPath string) (json.RawMessage, error)
}

// SessionContext carries per-request identity and chat metadata.
type SessionContext struct {
	OpenID           string
	UserName         string
	ChatID           string
	MessageID        string
	ReplyToMessageID string
	QuotedText       string
}

// ToolExecutor dispatches tool calls from the agent loop to service methods.
type ToolExecutor struct {
	svc ServiceLayer
}

// NewToolExecutor creates an executor backed by the given service layer.
func NewToolExecutor(svc ServiceLayer) *ToolExecutor {
	return &ToolExecutor{svc: svc}
}

// Execute runs a tool by name with JSON-encoded arguments, returning a JSON result string.
func (e *ToolExecutor) Execute(ctx context.Context, session SessionContext, toolName, argsJSON string) (string, error) {
	var args map[string]any
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	var result json.RawMessage
	var err error

	switch toolName {
	case "create_knowledge_draft":
		result, err = e.svc.CreateDraftForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID, session.MessageID, session.ReplyToMessageID,
			stringArg(args, "title"), stringArg(args, "content"), stringArg(args, "categoryPath"))

	case "confirm_knowledge_draft":
		result, err = e.svc.ApproveDraftForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID, session.MessageID, session.ReplyToMessageID,
			int64Arg(args, "draftId"), stringArg(args, "categoryPath"))

	case "reject_knowledge_draft":
		result, err = e.svc.RejectDraftForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID, session.MessageID, session.ReplyToMessageID,
			int64Arg(args, "draftId"))

	case "update_draft_category":
		result, err = e.svc.UpdateDraftCategoryForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID, session.MessageID, session.ReplyToMessageID,
			int64Arg(args, "draftId"), stringArg(args, "categoryPath"))

	case "search_knowledge":
		result, err = e.svc.SearchKnowledgeForAgent(ctx,
			session.OpenID, session.UserName, stringArg(args, "query"), stringArg(args, "category"))

	case "check_similarity":
		result, err = e.svc.CheckSimilarityForAgent(ctx,
			session.OpenID, session.UserName, stringArg(args, "text"), int64Arg(args, "draftId"))

	case "list_pending_drafts":
		result, err = e.svc.ListPendingDraftsForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID)

	case "revise_knowledge_draft":
		result, err = e.svc.ReviseDraftForAgent(ctx,
			session.OpenID, session.UserName, session.ChatID, session.MessageID, session.ReplyToMessageID,
			int64Arg(args, "draftId"), stringArg(args, "title"), stringArg(args, "content"), stringArg(args, "categoryPath"))

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	if err != nil {
		errResult, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(errResult), nil
	}
	return string(result), nil
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func int64Arg(args map[string]any, key string) int64 {
	switch v := args[key].(type) {
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
