package api

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"knowledgebook/internal/feishu"
	"knowledgebook/internal/model"
	"knowledgebook/internal/service"
)

type Handler struct {
	Services *service.Services
	DB       *pgxpool.Pool
	Redis    *redis.Client
}

func NewHandler(s *service.Services, db *pgxpool.Pool, r *redis.Client) *Handler {
	return &Handler{Services: s, DB: db, Redis: r}
}

func writeJSON(c *app.RequestContext, status int, payload model.APIResponse) {
	c.JSON(status, payload)
}

func bindJSON(c *app.RequestContext, dest interface{}) error {
	if len(c.Request.Body()) == 0 {
		return nil
	}
	return json.Unmarshal(c.Request.Body(), dest)
}

func requestID(c *app.RequestContext) string {
	return string(c.Request.Header.Peek("X-Request-Id"))
}

func pendingSelectionKey(openID, chatID string) string {
	return "feishu:pending_selection:" + strings.TrimSpace(openID) + ":" + strings.TrimSpace(chatID)
}

func parseSelectionIndex(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	idx, err := strconv.Atoi(trimmed)
	if err != nil || idx <= 0 {
		return 0
	}
	return idx
}

func extractPendingSelectionState(data interface{}) *pendingSelectionState {
	payload, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	state := &pendingSelectionState{}
	switch intent := payload["intent"].(type) {
	case model.IntentResult:
		state.Action = intent.Intent
	case *model.IntentResult:
		if intent != nil {
			state.Action = intent.Intent
		}
	case map[string]any:
		state.Action, _ = intent["intent"].(string)
	}
	switch pending := payload["pending"].(type) {
	case *service.PendingDraftContext:
		for _, item := range pending.Candidates {
			state.CandidateIDs = append(state.CandidateIDs, item.ID)
		}
	case map[string]any:
		candidatesRaw, ok := pending["candidates"].([]any)
		if ok {
			for _, item := range candidatesRaw {
				candidateMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				switch id := candidateMap["id"].(type) {
				case float64:
					state.CandidateIDs = append(state.CandidateIDs, int64(id))
				case int64:
					state.CandidateIDs = append(state.CandidateIDs, id)
				}
			}
		}
	}
	if state.Action == "" || len(state.CandidateIDs) == 0 {
		return nil
	}
	return state
}

func parseID(c *app.RequestContext, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

type feishuEventMessage struct {
	OpenID           string
	UserName         string
	Text             string
	MessageID        string
	ChatID           string
	ReplyToMessageID string
	QuotedText       string
}

type pendingSelectionState struct {
	Action     string  `json:"action"`
	CandidateIDs []int64 `json:"candidateIds"`
}

func feishuToken(payload map[string]any) string {
	if token, ok := payload["token"].(string); ok {
		return strings.TrimSpace(token)
	}
	header, _ := payload["header"].(map[string]any)
	if token, ok := header["token"].(string); ok {
		return strings.TrimSpace(token)
	}
	return ""
}

func validateFeishuRequest(c *app.RequestContext, rawBody []byte, payload map[string]any, verificationToken string) error {
	verificationToken = strings.TrimSpace(verificationToken)
	if verificationToken == "" {
		return nil
	}
	bodyToken := feishuToken(payload)
	if bodyToken != "" {
		if bodyToken != verificationToken {
			return fmt.Errorf("invalid feishu verification token")
		}
		return nil
	}
	timestamp := strings.TrimSpace(string(c.Request.Header.Peek("X-Lark-Request-Timestamp")))
	nonce := strings.TrimSpace(string(c.Request.Header.Peek("X-Lark-Request-Nonce")))
	signature := strings.TrimSpace(string(c.Request.Header.Peek("X-Lark-Signature")))
	if signature == "" {
		return fmt.Errorf("missing feishu token or signature")
	}
	if timestamp == "" || nonce == "" {
		return fmt.Errorf("missing feishu signature headers")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid feishu timestamp")
	}
	if delta := time.Now().Unix() - ts; delta > 300 || delta < -300 {
		return fmt.Errorf("expired feishu request timestamp")
	}
	sum := sha256.Sum256([]byte(timestamp + nonce + verificationToken + string(rawBody)))
	if hex.EncodeToString(sum[:]) != signature {
		return fmt.Errorf("invalid feishu signature")
	}
	return nil
}

func extractFeishuEventMessage(payload map[string]any) (*feishuEventMessage, error) {
	event, _ := payload["event"].(map[string]any)
	if len(event) == 0 {
		return nil, fmt.Errorf("missing event body")
	}
	message, _ := event["message"].(map[string]any)
	if len(message) == 0 {
		return nil, fmt.Errorf("missing message body")
	}
	if msgType, _ := message["message_type"].(string); msgType != "" && msgType != "text" {
		return nil, fmt.Errorf("unsupported message type: %s", msgType)
	}
	messageID, _ := message["message_id"].(string)
	chatID, _ := message["chat_id"].(string)
	replyToMessageID, _ := message["parent_id"].(string)
	contentText := ""
	quotedText := ""
	if rawContent, ok := message["content"].(string); ok && rawContent != "" {
		var content map[string]any
		if err := json.Unmarshal([]byte(rawContent), &content); err == nil {
			contentText, _ = content["text"].(string)
			quotedText, _ = content["quote_text"].(string)
		}
		if contentText == "" {
			contentText = rawContent
		}
	}
	sender, _ := event["sender"].(map[string]any)
	senderID, _ := sender["sender_id"].(map[string]any)
	openID, _ := senderID["open_id"].(string)
	senderName, _ := sender["sender_name"].(string)
	if senderName == "" {
		if name, _ := sender["name"].(string); name != "" {
			senderName = name
		}
	}
	return &feishuEventMessage{OpenID: strings.TrimSpace(openID), UserName: strings.TrimSpace(senderName), Text: strings.TrimSpace(contentText), MessageID: strings.TrimSpace(messageID), ChatID: strings.TrimSpace(chatID), ReplyToMessageID: strings.TrimSpace(replyToMessageID), QuotedText: strings.TrimSpace(quotedText)}, nil
}

func (h *Handler) Healthz(ctx stdctx.Context, c *app.RequestContext) {
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "ok"})
}

func (h *Handler) Readyz(ctx stdctx.Context, c *app.RequestContext) {
	if err := h.DB.Ping(ctx); err != nil {
		writeJSON(c, http.StatusServiceUnavailable, model.APIResponse{Code: 5000, Message: "database not ready", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if err := h.Redis.Ping(ctx).Err(); err != nil {
		writeJSON(c, http.StatusServiceUnavailable, model.APIResponse{Code: 5000, Message: "redis not ready", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "ready", RequestID: requestID(c)})
}

func (h *Handler) FeishuEvents(ctx stdctx.Context, c *app.RequestContext) {
	start := time.Now()
	rawBody := c.Request.Body()
	var payload map[string]any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			log.Printf("[feishu_event_invalid_payload] request_id=%s error=%v", requestID(c), err)
			writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid payload", Details: err.Error(), RequestID: requestID(c)})
			return
		}
	}
	eventType := ""
	if header, ok := payload["header"].(map[string]any); ok {
		eventType, _ = header["event_type"].(string)
	}
	log.Printf("[feishu_event_received] request_id=%s event_type=%s body_size=%d", requestID(c), eventType, len(rawBody))
	if err := validateFeishuRequest(c, rawBody, payload, h.Services.Cfg.FeishuVerificationToken); err != nil {
		log.Printf("[feishu_event_validate_failed] request_id=%s event_type=%s error=%v", requestID(c), eventType, err)
		writeJSON(c, http.StatusForbidden, model.APIResponse{Code: 4003, Message: "invalid feishu request", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if challenge, ok := payload["challenge"].(string); ok && challenge != "" {
		log.Printf("[feishu_event_challenge] request_id=%s event_type=%s", requestID(c), eventType)
		c.JSON(http.StatusOK, map[string]string{"challenge": challenge})
		return
	}
	msg, err := extractFeishuEventMessage(payload)
	if err != nil {
		log.Printf("[feishu_event_extract_failed] request_id=%s event_type=%s error=%v", requestID(c), eventType, err)
		writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "event accepted", Data: map[string]any{"handled": false, "reason": err.Error()}, RequestID: requestID(c)})
		return
	}
	eventID := ""
	if header, ok := payload["header"].(map[string]any); ok {
		eventID, _ = header["event_id"].(string)
	}
	log.Printf("[feishu_event_parsed] request_id=%s event_id=%s event_type=%s message_id=%s chat_id=%s open_id=%s reply_to_message_id=%s text=%q", requestID(c), eventID, eventType, msg.MessageID, msg.ChatID, msg.OpenID, msg.ReplyToMessageID, msg.Text)
	if msg.Text == "" {
		log.Printf("[feishu_event_ignored] request_id=%s event_id=%s reason=empty_text", requestID(c), eventID)
		writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "event accepted", Data: map[string]any{"handled": false, "reason": "empty message text"}, RequestID: requestID(c)})
		return
	}
	dedupID := strings.TrimSpace(eventID)
	if dedupID == "" {
		dedupID = strings.TrimSpace(msg.MessageID)
	}
	if dedupID != "" {
		ok, err := h.Redis.SetNX(ctx, "feishu:event:"+dedupID, "1", 10*time.Minute).Result()
		if err != nil {
			log.Printf("[feishu_event_dedup_check_failed] request_id=%s event_id=%s message_id=%s error=%v", requestID(c), eventID, msg.MessageID, err)
		} else if !ok {
			log.Printf("[feishu_event_duplicate] request_id=%s event_id=%s message_id=%s", requestID(c), eventID, msg.MessageID)
			writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "event accepted", Data: map[string]any{"handled": false, "reason": "duplicate event"}, RequestID: requestID(c)})
			return
		}
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "event accepted", Data: map[string]any{"handled": true, "messageId": msg.MessageID, "eventId": eventID}, RequestID: requestID(c)})
	log.Printf("[feishu_event_acked] request_id=%s event_id=%s message_id=%s ack_duration_ms=%d", requestID(c), eventID, msg.MessageID, time.Since(start).Milliseconds())

	go func(msg feishuEventMessage, eventID, requestID string) {
		backgroundCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 60*time.Second)
		defer cancel()
		runtimeCtx := map[string]any{"chat_id": msg.ChatID, "message_id": msg.MessageID, "reply_to_message_id": msg.ReplyToMessageID, "quoted_text": msg.QuotedText}
		if idx := parseSelectionIndex(msg.Text); idx > 0 && msg.OpenID != "" && msg.ChatID != "" {
			stateJSON, err := h.Redis.Get(backgroundCtx, pendingSelectionKey(msg.OpenID, msg.ChatID)).Result()
			if err == nil && strings.TrimSpace(stateJSON) != "" {
				var state pendingSelectionState
				if err := json.Unmarshal([]byte(stateJSON), &state); err == nil && idx <= len(state.CandidateIDs) {
					runtimeCtx["selected_action"] = state.Action
					runtimeCtx["selected_draft_id"] = state.CandidateIDs[idx-1]
					log.Printf("[feishu_pending_selection_resolved] request_id=%s event_id=%s message_id=%s selected_index=%d selected_draft_id=%d action=%s", requestID, eventID, msg.MessageID, idx, state.CandidateIDs[idx-1], state.Action)
				}
			}
		}
		result, err := h.Services.ExecuteBotMessageWithContext(backgroundCtx, msg.OpenID, msg.UserName, msg.Text, runtimeCtx)
		if err != nil {
			log.Printf("[feishu_event_handle_failed] request_id=%s event_id=%s message_id=%s error=%v", requestID, eventID, msg.MessageID, err)
			return
		}
		log.Printf("[conversation_result] request_id=%s event_id=%s message_id=%s intent=%s reply_len=%d card_present=%t duration_ms=%d", requestID, eventID, msg.MessageID, result.Command, len(result.Reply), strings.TrimSpace(result.CardMarkdown) != "", time.Since(start).Milliseconds())
		if msg.OpenID != "" && msg.ChatID != "" {
			selectionKey := pendingSelectionKey(msg.OpenID, msg.ChatID)
			if state := extractPendingSelectionState(result.Data); state != nil {
				if payload, err := json.Marshal(state); err == nil {
					if err := h.Redis.Set(backgroundCtx, selectionKey, payload, 10*time.Minute).Err(); err != nil {
						log.Printf("[feishu_pending_selection_store_failed] request_id=%s event_id=%s message_id=%s error=%v", requestID, eventID, msg.MessageID, err)
					} else {
						log.Printf("[feishu_pending_selection_stored] request_id=%s event_id=%s message_id=%s action=%s candidate_count=%d", requestID, eventID, msg.MessageID, state.Action, len(state.CandidateIDs))
					}
				}
			} else {
				_ = h.Redis.Del(backgroundCtx, selectionKey).Err()
			}
		}
		if msg.MessageID == "" {
			return
		}
		messenger := feishu.NewMessenger(h.Services.Cfg.FeishuAppID, h.Services.Cfg.FeishuAppSecret)
		if !messenger.Enabled() {
			log.Printf("[feishu_reply_skipped] request_id=%s event_id=%s message_id=%s reason=messenger_disabled", requestID, eventID, msg.MessageID)
			return
		}
		draftID := extractDraftID(result.Data)
		replyType := map[bool]string{true: "card", false: "text"}[strings.TrimSpace(result.CardMarkdown) != ""]
		log.Printf("[feishu_reply_attempt] request_id=%s event_id=%s message_id=%s chat_id=%s reply_type=%s draft_id=%d", requestID, eventID, msg.MessageID, msg.ChatID, replyType, draftID)
		if strings.TrimSpace(result.CardMarkdown) != "" {
			cardJSON := feishu.BuildDraftCardJSON("知识沉淀助手", result.CardMarkdown, []feishu.CardAction{{Action: "confirm", DraftID: draftID, ChatID: msg.ChatID}, {Action: "reject", DraftID: draftID, ChatID: msg.ChatID}, {Action: "change_category", DraftID: draftID, ChatID: msg.ChatID}})
			if err := messenger.ReplyCard(backgroundCtx, msg.MessageID, cardJSON); err != nil {
				log.Printf("[feishu_reply_card_failed] request_id=%s event_id=%s message_id=%s draft_id=%d error=%v card_json=%s", requestID, eventID, msg.MessageID, draftID, err, cardJSON)
				if err := messenger.ReplyText(backgroundCtx, msg.MessageID, result.Reply); err != nil {
					log.Printf("[feishu_reply_text_fallback_failed] request_id=%s event_id=%s message_id=%s draft_id=%d error=%v", requestID, eventID, msg.MessageID, draftID, err)
				} else {
					log.Printf("[feishu_reply_text_fallback_success] request_id=%s event_id=%s message_id=%s draft_id=%d", requestID, eventID, msg.MessageID, draftID)
				}
			} else {
				log.Printf("[feishu_reply_card_success] request_id=%s event_id=%s message_id=%s draft_id=%d", requestID, eventID, msg.MessageID, draftID)
			}
			return
		}
		if err := messenger.ReplyText(backgroundCtx, msg.MessageID, result.Reply); err != nil {
			log.Printf("[feishu_reply_text_failed] request_id=%s event_id=%s message_id=%s draft_id=%d error=%v", requestID, eventID, msg.MessageID, draftID, err)
		} else {
			log.Printf("[feishu_reply_text_success] request_id=%s event_id=%s message_id=%s draft_id=%d", requestID, eventID, msg.MessageID, draftID)
		}
	}(*msg, eventID, requestID(c))
}

func extractDraftID(data interface{}) int64 {
	payload, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	if draft, ok := payload["draft"].(*model.Draft); ok && draft != nil {
		return draft.ID
	}
	if draftMap, ok := payload["draft"].(map[string]any); ok {
		switch id := draftMap["id"].(type) {
		case int64:
			return id
		case float64:
			return int64(id)
		}
	}
	return 0
}

func (h *Handler) FeishuOAuthCallback(ctx stdctx.Context, c *app.RequestContext) {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	errorCode := strings.TrimSpace(c.Query("error"))
	errorDescription := strings.TrimSpace(c.Query("error_description"))
	if code != "" {
		if err := h.Redis.Set(ctx, "feishu:oauth:last_code", code, 30*time.Minute).Err(); err != nil {
			log.Printf("[feishu_oauth_callback_store_failed] code=%s state=%s error=%v", code, state, err)
		} else {
			log.Printf("[feishu_oauth_callback_received] code=%s state=%s", code, state)
		}
	}
	if errorCode != "" || errorDescription != "" {
		log.Printf("[feishu_oauth_callback_error] error=%s error_description=%s state=%s", errorCode, errorDescription, state)
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "oauth callback received", Data: map[string]any{"code": code, "state": state, "error": errorCode, "errorDescription": errorDescription}, RequestID: requestID(c)})
}

func (h *Handler) FeishuCardCallback(ctx stdctx.Context, c *app.RequestContext) {
	rawBody := c.Request.Body()
	var payload map[string]any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid payload", Details: err.Error(), RequestID: requestID(c)})
			return
		}
	}
	if challenge, ok := payload["challenge"].(string); ok && challenge != "" {
		c.JSON(http.StatusOK, map[string]string{"challenge": challenge})
		return
	}
	actionMap, _ := payload["action"].(map[string]any)
	valueMap, _ := actionMap["value"].(map[string]any)
	action, _ := valueMap["action"].(string)
	chatID, _ := valueMap["chat_id"].(string)
	draftIDRaw := valueMap["draft_id"]
	var draftID int64
	switch v := draftIDRaw.(type) {
	case float64:
		draftID = int64(v)
	case int64:
		draftID = v
	}
	openID := ""
	userName := ""
	if operator, ok := payload["operator"].(map[string]any); ok {
		if operatorID, ok := operator["open_id"].(string); ok {
			openID = strings.TrimSpace(operatorID)
		}
		if name, ok := operator["name"].(string); ok {
			userName = strings.TrimSpace(name)
		}
	}
	formValue := map[string]any{}
	if fv, ok := actionMap["form_value"].(map[string]any); ok {
		formValue = fv
	}
	toastType := "info"
	result := "暂不支持的卡片操作"
	switch action {
	case "confirm":
		item, err := h.Services.ApproveDraft(ctx, openID, userName, draftID, "")
		if err != nil {
			if strings.Contains(err.Error(), "already resolved") {
				toastType = "warning"
				result = fmt.Sprintf("草稿 #%d 已处理，请刷新查看最新状态。", draftID)
				break
			}
			writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "confirm draft failed", Details: err.Error(), RequestID: requestID(c)})
			return
		}
		toastType = "success"
		result = fmt.Sprintf("已确认草稿 #%d，生成知识 #%d：%s", draftID, item.ID, item.Title)
	case "reject":
		if err := h.Services.RejectDraft(ctx, openID, userName, draftID); err != nil {
			if strings.Contains(err.Error(), "already resolved") {
				toastType = "warning"
				result = fmt.Sprintf("草稿 #%d 已处理，请刷新查看最新状态。", draftID)
				break
			}
			writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "reject draft failed", Details: err.Error(), RequestID: requestID(c)})
			return
		}
		toastType = "success"
		result = fmt.Sprintf("已丢弃草稿 #%d", draftID)
	case "change_category":
		categoryPath := ""
		for _, key := range []string{fmt.Sprintf("cat%d", draftID), "category_path"} {
			if value, ok := formValue[key].(string); ok && strings.TrimSpace(value) != "" {
				categoryPath = strings.TrimSpace(value)
				break
			}
		}
		if categoryPath == "" {
			toastType = "warning"
			result = "请先在输入框里填写新的完整分类路径，再提交。"
			break
		}
		updatedDraft, err := h.Services.UpdateDraftCategory(ctx, openID, userName, draftID, categoryPath)
		if err != nil {
			if strings.Contains(err.Error(), "no rows") || strings.Contains(err.Error(), "already resolved") {
				toastType = "warning"
				result = fmt.Sprintf("草稿 #%d 已处理，不能再修改分类。", draftID)
				break
			}
			writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "change category failed", Details: err.Error(), RequestID: requestID(c)})
			return
		}
		toastType = "success"
		result = fmt.Sprintf("已将草稿 #%d 的分类改为：%s", draftID, updatedDraft.RecommendedCategoryPath)
	default:
		result = "暂不支持的卡片操作"
	}
	c.JSON(http.StatusOK, map[string]any{
		"toast": map[string]any{
			"type":    toastType,
			"content": result,
		},
		"data": map[string]any{
			"action":  action,
			"draftId": draftID,
			"chatId":  chatID,
			"result":  result,
		},
	})
}

func (h *Handler) CreateKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	var req service.CreateKnowledgeRequest
	if err := bindJSON(c, &req); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid request", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	draft, err := h.Services.CreateDraft(ctx, req)
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5004, Message: "create draft failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: draft, RequestID: requestID(c)})
}

func (h *Handler) ApproveDraft(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid draft id", RequestID: requestID(c)})
		return
	}
	var body struct {
		OpenID       string `json:"openId"`
		UserName     string `json:"userName"`
		CategoryPath string `json:"categoryPath"`
	}
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	item, err := h.Services.ApproveDraft(ctx, body.OpenID, body.UserName, id, body.CategoryPath)
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "approve draft failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: item, RequestID: requestID(c)})
}

func (h *Handler) IgnoreDraft(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid draft id", RequestID: requestID(c)})
		return
	}
	var body struct{ OpenID, UserName string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if err := h.Services.IgnoreDraft(ctx, body.OpenID, body.UserName, id); err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "ignore draft failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "ignored", RequestID: requestID(c)})
}

func (h *Handler) LaterDraft(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid draft id", RequestID: requestID(c)})
		return
	}
	var body struct{ OpenID, UserName string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if err := h.Services.LaterDraft(ctx, body.OpenID, body.UserName, id); err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "move draft later failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "moved to later list", RequestID: requestID(c)})
}

func (h *Handler) ListLaterDrafts(ctx stdctx.Context, c *app.RequestContext) {
	items, err := h.Services.ListLater(ctx, c.Query("openId"), c.Query("userName"))
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "list later failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: map[string]any{"items": items}, RequestID: requestID(c)})
}

func (h *Handler) SearchKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	result, err := h.Services.SearchAnswer(ctx, c.Query("openId"), c.Query("userName"), c.Query("query"), c.Query("category"))
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "search failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: result, RequestID: requestID(c)})
}

func (h *Handler) GetKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	item, err := h.Services.GetKnowledge(ctx, c.Query("openId"), c.Query("userName"), id)
	if err != nil {
		writeJSON(c, http.StatusNotFound, model.APIResponse{Code: 4001, Message: "knowledge not found", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: item, RequestID: requestID(c)})
}

func (h *Handler) UpdateKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	var req service.UpdateKnowledgeRequest
	if err := bindJSON(c, &req); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid request", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	item, err := h.Services.UpdateKnowledge(ctx, id, req)
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "update knowledge failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "updated", Data: item, RequestID: requestID(c)})
}

func (h *Handler) MoveKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	var body struct{ OpenID, UserName, CategoryPath string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	item, err := h.Services.MoveKnowledge(ctx, body.OpenID, body.UserName, id, body.CategoryPath)
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "move category failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "moved", Data: item, RequestID: requestID(c)})
}

func (h *Handler) SoftDeleteKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	var body struct{ OpenID, UserName string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if err := h.Services.SoftDeleteKnowledge(ctx, body.OpenID, body.UserName, id); err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "soft delete failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "soft deleted", RequestID: requestID(c)})
}

func (h *Handler) RestoreKnowledge(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	var body struct{ OpenID, UserName string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	if err := h.Services.RestoreKnowledge(ctx, body.OpenID, body.UserName, id); err != nil {
		code := 5000
		if strings.Contains(err.Error(), "expired") {
			code = 4005
		}
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: code, Message: "restore failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "restored", RequestID: requestID(c)})
}

func (h *Handler) SyncFromDoc(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	var req service.SyncFromDocRequest
	if err := bindJSON(c, &req); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	item, err := h.Services.SyncFromDoc(ctx, id, req)
	if err != nil {
		code := 5000
		msg := "sync from doc failed"
		if strings.Contains(err.Error(), "DOC_BACKFILL_NO_DIFF") {
			code = 5003
			msg = "no diff detected"
		}
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: code, Message: msg, Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "synced from doc", Data: item, RequestID: requestID(c)})
}

func (h *Handler) ListCategories(ctx stdctx.Context, c *app.RequestContext) {
	items, err := h.Services.ListCategories(ctx, c.Query("openId"), c.Query("userName"))
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "list categories failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: map[string]any{"items": items}, RequestID: requestID(c)})
}

func (h *Handler) CreateCategory(ctx stdctx.Context, c *app.RequestContext) {
	var body struct{ OpenID, UserName, Path string }
	if err := bindJSON(c, &body); err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid body", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	id, path, err := h.Services.CreateCategory(ctx, body.OpenID, body.UserName, body.Path)
	if err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5000, Message: "create category failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: map[string]any{"id": id, "path": path}, RequestID: requestID(c)})
}

func (h *Handler) TriggerKnowledgeSync(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid knowledge id", RequestID: requestID(c)})
		return
	}
	if err := h.Services.TriggerSync(ctx, "DOC_SYNC_KNOWLEDGE", id); err != nil {
		writeJSON(c, http.StatusInternalServerError, model.APIResponse{Code: 5001, Message: "enqueue sync failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "sync queued", RequestID: requestID(c)})
}

func (h *Handler) GetTask(ctx stdctx.Context, c *app.RequestContext) {
	id, err := parseID(c, "id")
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "invalid task id", RequestID: requestID(c)})
		return
	}
	task, err := h.Services.Store.GetTask(ctx, id)
	if err != nil {
		writeJSON(c, http.StatusNotFound, model.APIResponse{Code: 4001, Message: "task not found", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "success", Data: task, RequestID: requestID(c)})
}

func (h *Handler) HandleBotCommand(ctx stdctx.Context, c *app.RequestContext) {
	text := strings.TrimSpace(c.Query("text"))
	if text == "" {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "text is required", RequestID: requestID(c)})
		return
	}
	if !strings.HasPrefix(text, "/") {
		text = "/" + text
	}
	parsed := feishu.ParseCommand(text)
	if parsed.Namespace != "kb" {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "only /kb commands are supported", RequestID: requestID(c)})
		return
	}
	result, err := h.Services.ExecuteBotCommand(ctx, c.Query("openId"), c.Query("userName"), text)
	if err != nil {
		writeJSON(c, http.StatusBadRequest, model.APIResponse{Code: 4003, Message: "bot command failed", Details: err.Error(), RequestID: requestID(c)})
		return
	}
	writeJSON(c, http.StatusOK, model.APIResponse{Code: 0, Message: "bot command handled", Data: result, RequestID: requestID(c)})
}

func ErrorResponse(code int, message string, details interface{}, reqID string) model.APIResponse {
	return model.APIResponse{Code: code, Message: message, Details: details, RequestID: reqID}
}

func MustJSONString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func HumanMessage(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
