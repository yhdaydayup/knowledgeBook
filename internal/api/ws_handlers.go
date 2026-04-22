package api

import (
	stdctx "context"
	"log"
	"strings"
	"time"

	"knowledgebook/internal/feishu"
)

// OnWSMessageEvent adapts a WebSocket message event to the shared processMessageEvent logic.
func (h *Handler) OnWSMessageEvent(ctx stdctx.Context, eventID string, msg feishu.FeishuEventMessage) {
	internal := feishuEventMessage{
		OpenID:           msg.OpenID,
		UserName:         msg.UserName,
		Text:             msg.Text,
		MessageID:        msg.MessageID,
		ChatID:           msg.ChatID,
		ReplyToMessageID: msg.ReplyToMessageID,
		QuotedText:       msg.QuotedText,
	}
	log.Printf("[ws_event_parsed] event_id=%s message_id=%s chat_id=%s open_id=%s text=%q",
		eventID, internal.MessageID, internal.ChatID, internal.OpenID, internal.Text)

	dedupID := strings.TrimSpace(eventID)
	if dedupID == "" {
		dedupID = strings.TrimSpace(internal.MessageID)
	}
	if dedupID != "" {
		ok, err := h.Redis.SetNX(ctx, "feishu:event:"+dedupID, "1", 10*time.Minute).Result()
		if err != nil {
			log.Printf("[ws_event_dedup_check_failed] event_id=%s message_id=%s error=%v", eventID, internal.MessageID, err)
		} else if !ok {
			log.Printf("[ws_event_duplicate] event_id=%s message_id=%s", eventID, internal.MessageID)
			return
		}
	}

	go h.processMessageEvent(internal, eventID, "ws:"+eventID)
}

// OnWSCardAction adapts a WebSocket card action to the shared processCardAction logic.
func (h *Handler) OnWSCardAction(ctx stdctx.Context, action, chatID string, draftID int64, openID, userName string, formValue map[string]any, openMessageID string) map[string]any {
	log.Printf("[ws_card_action_dispatch] action=%s draft_id=%d open_id=%s chat_id=%s open_message_id=%s",
		action, draftID, openID, chatID, openMessageID)
	return h.processCardAction(ctx, action, chatID, draftID, openID, userName, formValue, openMessageID)
}
