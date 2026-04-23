package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"knowledgebook/internal/feishu"
)

// Agent adapter methods — these implement conversation.ServiceLayer so the
// tool executor can call service logic without knowing internal types.

func (s *Services) CreateDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID, title, content, categoryPath string) (json.RawMessage, error) {
	draft, err := s.CreateDraft(ctx, CreateKnowledgeRequest{
		OpenID:          openID,
		UserName:        userName,
		Title:           title,
		Content:         content,
		CategoryPath:    categoryPath,
		Source:          "AGENT_TOOL",
		ChatID:          chatID,
		SourceMessageID: messageID,
		ReplyMessageID:  replyToMessageID,
	})
	if err != nil {
		return nil, err
	}
	// Also run similarity check
	records, _ := s.CheckSimilarity(ctx, openID, userName, draft.RawContent, draft.ID)
	result := map[string]any{
		"id":                       draft.ID,
		"title":                    draft.Title,
		"normalized_summary":       draft.NormalizedSummary,
		"normalized_points":        draft.NormalizedPoints,
		"recommended_category_path": draft.RecommendedCategoryPath,
		"status":                   draft.Status,
		"similarities":            records,
	}
	return json.Marshal(result)
}

func (s *Services) ApproveDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error) {
	// Resolve draft context if draftID is not specified
	if draftID == 0 {
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, 0, "")
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return json.Marshal(map[string]any{
				"needs_clarification": true,
				"message":             resolved.ClarifyReply,
				"pending_count":       resolved.PendingCount,
			})
		}
		draftID = resolved.Draft.ID
	}
	item, err := s.ApproveDraft(ctx, openID, userName, draftID, categoryPath)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"draft_id":      draftID,
		"knowledge_id":  item.ID,
		"title":         item.Title,
		"category_path": item.CategoryPath,
		"status":        "APPROVED",
	})
}

func (s *Services) RejectDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64) (json.RawMessage, error) {
	if draftID == 0 {
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, 0, "")
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return json.Marshal(map[string]any{
				"needs_clarification": true,
				"message":             resolved.ClarifyReply,
				"pending_count":       resolved.PendingCount,
			})
		}
		draftID = resolved.Draft.ID
	}
	draft, err := s.GetDraft(ctx, openID, userName, draftID)
	if err != nil {
		return nil, err
	}
	if err := s.RejectDraft(ctx, openID, userName, draftID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"draft_id": draftID,
		"title":    draft.Title,
		"status":   "REJECTED",
	})
}

func (s *Services) UpdateDraftCategoryForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, categoryPath string) (json.RawMessage, error) {
	if strings.TrimSpace(categoryPath) == "" {
		return nil, fmt.Errorf("categoryPath is required")
	}
	if draftID == 0 {
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, 0, "")
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return json.Marshal(map[string]any{
				"needs_clarification": true,
				"message":             resolved.ClarifyReply,
				"pending_count":       resolved.PendingCount,
			})
		}
		draftID = resolved.Draft.ID
	}
	draft, err := s.UpdateDraftCategory(ctx, openID, userName, draftID, categoryPath)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"draft_id":                  draft.ID,
		"title":                     draft.Title,
		"recommended_category_path": draft.RecommendedCategoryPath,
		"status":                    draft.Status,
	})
}

func (s *Services) SearchKnowledgeForAgent(ctx context.Context, openID, userName, query, category string) (json.RawMessage, error) {
	answer, err := s.SearchAnswer(ctx, openID, userName, query, category)
	if err != nil {
		return nil, err
	}
	return json.Marshal(answer)
}

func (s *Services) CheckSimilarityForAgent(ctx context.Context, openID, userName, text string, draftID int64) (json.RawMessage, error) {
	records, err := s.CheckSimilarity(ctx, openID, userName, text, draftID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(records)
}

func (s *Services) ListPendingDraftsForAgent(ctx context.Context, openID, userName, chatID string) (json.RawMessage, error) {
	drafts, err := s.ListPendingDrafts(ctx, openID, userName, chatID, 10)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(drafts))
	for _, d := range drafts {
		items = append(items, map[string]any{
			"draft_id": d.ID,
			"title":    d.Title,
			"summary":  d.NormalizedSummary,
			"status":   d.Status,
		})
	}
	return json.Marshal(map[string]any{
		"count":  len(items),
		"drafts": items,
	})
}

func (s *Services) ReviseDraftForAgent(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, draftID int64, title, content, categoryPath string) (json.RawMessage, error) {
	// 1. Resolve draft context if draftID is not specified
	if draftID == 0 {
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, 0, "")
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return json.Marshal(map[string]any{
				"needs_clarification": true,
				"message":             resolved.ClarifyReply,
				"pending_count":       resolved.PendingCount,
			})
		}
		draftID = resolved.Draft.ID
	}

	// 2. Get the old draft
	oldDraft, err := s.GetDraft(ctx, openID, userName, draftID)
	if err != nil {
		return nil, fmt.Errorf("获取原草稿失败: %w", err)
	}

	// 3. Reject the old draft
	if err := s.RejectDraft(ctx, openID, userName, draftID); err != nil {
		return nil, fmt.Errorf("废弃旧草稿失败: %w", err)
	}

	// 4. Patch the old card to show "已修订"
	if strings.TrimSpace(oldDraft.CardMessageID) != "" && s.Messenger != nil && s.Messenger.Enabled() {
		md := fmt.Sprintf("# 草稿 #%d\n- 标题：%s\n- 已修订为新草稿", draftID, oldDraft.Title)
		card := feishu.BuildResolvedCardJSON("知识沉淀助手", md, "revised")
		if patchErr := s.Messenger.PatchCard(ctx, oldDraft.CardMessageID, card); patchErr != nil {
			log.Printf("[revise_card_patch_failed] draft_id=%d card_message_id=%s error=%v", draftID, oldDraft.CardMessageID, patchErr)
		}
	}

	// 5. Use old draft fields as defaults where new values are empty
	if strings.TrimSpace(title) == "" {
		title = oldDraft.Title
	}
	if strings.TrimSpace(categoryPath) == "" {
		categoryPath = oldDraft.RecommendedCategoryPath
	}

	// 6. Create a new draft with revised content
	newDraft, err := s.CreateDraft(ctx, CreateKnowledgeRequest{
		OpenID:          openID,
		UserName:        userName,
		Title:           title,
		Content:         content,
		CategoryPath:    categoryPath,
		Source:          "AGENT_TOOL",
		ChatID:          chatID,
		SourceMessageID: messageID,
		ReplyMessageID:  replyToMessageID,
	})
	if err != nil {
		return nil, fmt.Errorf("创建修订草稿失败: %w", err)
	}

	return json.Marshal(map[string]any{
		"old_draft_id":               draftID,
		"old_draft_status":           "REJECTED",
		"id":                         newDraft.ID,
		"title":                      newDraft.Title,
		"normalized_summary":         newDraft.NormalizedSummary,
		"normalized_points":          newDraft.NormalizedPoints,
		"recommended_category_path":  newDraft.RecommendedCategoryPath,
		"status":                     newDraft.Status,
	})
}
