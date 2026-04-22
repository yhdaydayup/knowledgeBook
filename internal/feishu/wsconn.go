package feishu

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// FeishuEventMessage is a parsed bot message event shared between HTTP and WebSocket paths.
type FeishuEventMessage struct {
	OpenID           string
	UserName         string
	Text             string
	MessageID        string
	ChatID           string
	ReplyToMessageID string
	QuotedText       string
}

// MessageEventCallback is called when a message event arrives via WebSocket.
type MessageEventCallback func(ctx context.Context, eventID string, msg FeishuEventMessage)

// CardActionCallback is called when a card action arrives via WebSocket.
// Returns the toast response map.
type CardActionCallback func(ctx context.Context, action, chatID string, draftID int64, openID, userName string, formValue map[string]any, openMessageID string) map[string]any

// WSClient wraps the Feishu WebSocket long connection client.
type WSClient struct {
	appID, appSecret string
	onMessage        MessageEventCallback
	onCardAction     CardActionCallback
}

// NewWSClient creates a new WebSocket client for Feishu long connection events.
func NewWSClient(appID, appSecret string, onMessage MessageEventCallback, onCardAction CardActionCallback) *WSClient {
	return &WSClient{appID: appID, appSecret: appSecret, onMessage: onMessage, onCardAction: onCardAction}
}

// Start connects to Feishu and blocks, processing events. It handles reconnection internally.
func (c *WSClient) Start(ctx context.Context) error {
	eventDispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			msg, eventID := extractWSMessage(event)
			if msg.Text == "" {
				log.Printf("[ws_event_ignored] event_id=%s reason=empty_text", eventID)
				return nil
			}
			log.Printf("[ws_event_received] event_id=%s message_id=%s chat_id=%s open_id=%s text=%q",
				eventID, msg.MessageID, msg.ChatID, msg.OpenID, msg.Text)
			c.onMessage(ctx, eventID, msg)
			return nil
		}).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			action, chatID, draftID, openID, userName, formValue, openMessageID := extractWSCardAction(event)
			log.Printf("[ws_card_action] action=%s draft_id=%d open_id=%s chat_id=%s open_message_id=%s",
				action, draftID, openID, chatID, openMessageID)
			response := c.onCardAction(ctx, action, chatID, draftID, openID, userName, formValue, openMessageID)
			return toCardResponse(response), nil
		})

	wsClient := larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)
	return wsClient.Start(ctx)
}

func extractWSMessage(event *larkim.P2MessageReceiveV1) (FeishuEventMessage, string) {
	var msg FeishuEventMessage
	var eventID string
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}
	if event.Event == nil {
		return msg, eventID
	}
	if m := event.Event.Message; m != nil {
		if m.MessageType != nil && *m.MessageType != "text" {
			return msg, eventID
		}
		if m.MessageId != nil {
			msg.MessageID = strings.TrimSpace(*m.MessageId)
		}
		if m.ChatId != nil {
			msg.ChatID = strings.TrimSpace(*m.ChatId)
		}
		if m.ParentId != nil {
			msg.ReplyToMessageID = strings.TrimSpace(*m.ParentId)
		}
		if m.Content != nil {
			var content map[string]any
			if err := json.Unmarshal([]byte(*m.Content), &content); err == nil {
				if text, ok := content["text"].(string); ok {
					msg.Text = strings.TrimSpace(text)
				}
				if qt, ok := content["quote_text"].(string); ok {
					msg.QuotedText = strings.TrimSpace(qt)
				}
			}
		}
	}
	if s := event.Event.Sender; s != nil {
		if s.SenderId != nil && s.SenderId.OpenId != nil {
			msg.OpenID = strings.TrimSpace(*s.SenderId.OpenId)
		}
	}
	return msg, eventID
}

func extractWSCardAction(event *callback.CardActionTriggerEvent) (action, chatID string, draftID int64, openID, userName string, formValue map[string]any, openMessageID string) {
	formValue = map[string]any{}
	if event.Event == nil {
		return
	}
	if event.Event.Operator != nil {
		openID = strings.TrimSpace(event.Event.Operator.OpenID)
	}
	if event.Event.Context != nil {
		openMessageID = strings.TrimSpace(event.Event.Context.OpenMessageID)
		chatID = strings.TrimSpace(event.Event.Context.OpenChatID)
	}
	if event.Event.Action != nil {
		if a, ok := event.Event.Action.Value["action"].(string); ok {
			action = strings.TrimSpace(a)
		}
		if cid, ok := event.Event.Action.Value["chat_id"].(string); ok && chatID == "" {
			chatID = strings.TrimSpace(cid)
		}
		if did, ok := event.Event.Action.Value["draft_id"]; ok {
			switch v := did.(type) {
			case float64:
				draftID = int64(v)
			case int64:
				draftID = v
			}
		}
		if event.Event.Action.FormValue != nil {
			formValue = event.Event.Action.FormValue
		}
	}
	return
}

func toCardResponse(response map[string]any) *callback.CardActionTriggerResponse {
	toast, _ := response["toast"].(map[string]any)
	if toast == nil {
		return nil
	}
	toastType, _ := toast["type"].(string)
	content, _ := toast["content"].(string)
	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    toastType,
			Content: content,
		},
	}
}
