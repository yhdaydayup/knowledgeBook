package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Messenger struct {
	httpClient *http.Client
	appID      string
	appSecret  string
}

type CardAction struct {
	Action  string `json:"action"`
	DraftID int64  `json:"draft_id"`
	ChatID  string `json:"chat_id,omitempty"`
}

func NewMessenger(appID, appSecret string) *Messenger {
	return &Messenger{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		appID:      strings.TrimSpace(appID),
		appSecret:  strings.TrimSpace(appSecret),
	}
}

func (m *Messenger) Enabled() bool {
	return m.appID != "" && m.appSecret != "" && !strings.Contains(m.appID, "xxx") && !strings.Contains(m.appSecret, "xxx")
}

func (m *Messenger) ReplyText(ctx context.Context, messageID, text string) error {
	if !m.Enabled() {
		return fmt.Errorf("feishu messenger not configured")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message_id is required")
	}
	token, err := m.fetchTenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{
		"msg_type": "text",
		"content":  string(MustJSON(map[string]string{"text": text})),
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s/reply", messageID)
	if err := m.postJSON(ctx, url, token, body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("reply message failed: %s", resp.Msg)
	}
	return nil
}

func (m *Messenger) ReplyCard(ctx context.Context, messageID, cardJSON string) error {
	if !m.Enabled() {
		return fmt.Errorf("feishu messenger not configured")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message_id is required")
	}
	token, err := m.fetchTenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]string{
		"msg_type": "interactive",
		"content":  cardJSON,
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages/%s/reply", messageID)
	if err := m.postJSON(ctx, url, token, body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("reply card failed: %s", resp.Msg)
	}
	return nil
}

func BuildDraftCardJSON(title, markdown string, actions []CardAction) string {
	elements := []map[string]any{{
		"tag": "markdown",
		"content": markdown,
	}}
	var draftID int64
	var chatID string
	for _, action := range actions {
		if draftID == 0 && action.DraftID > 0 {
			draftID = action.DraftID
		}
		if chatID == "" && strings.TrimSpace(action.ChatID) != "" {
			chatID = strings.TrimSpace(action.ChatID)
		}
		switch action.Action {
		case "confirm", "reject":
			label := map[string]string{"confirm": "确认保存", "reject": "拒绝保存"}[action.Action]
			buttonType := map[string]string{"confirm": "primary", "reject": "default"}[action.Action]
			elements = append(elements, map[string]any{
				"tag":  "button",
				"type": buttonType,
				"text": map[string]any{"tag": "plain_text", "content": label},
				"behaviors": []map[string]any{{
					"type": "callback",
					"value": map[string]any{
						"action":   action.Action,
						"draft_id": action.DraftID,
						"chat_id":  action.ChatID,
					},
				}},
			})
		}
	}
	if draftID > 0 {
		elements = append(elements, map[string]any{
			"tag": "form",
			"name": fmt.Sprintf("f%d", draftID),
			"direction": "horizontal",
			"horizontal_spacing": "8px",
			"vertical_spacing": "8px",
			"elements": []map[string]any{
				{
					"tag": "input",
					"name": fmt.Sprintf("cat%d", draftID),
					"placeholder": map[string]any{"tag": "plain_text", "content": "输入新的完整分类路径，例如：软件开发/接口治理"},
					"width": "fill",
				},
				{
					"tag": "button",
					"name": fmt.Sprintf("chg%d", draftID),
					"type": "primary",
					"text": map[string]any{"tag": "plain_text", "content": "提交分类修改"},
					"behaviors": []map[string]any{{
						"type": "callback",
						"value": map[string]any{
							"action":   "change_category",
							"draft_id": draftID,
							"chat_id":  chatID,
						},
					}},
					"form_action_type": "submit",
				},
			},
		})
	}
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{"width_mode": "fill"},
		"header": map[string]any{
			"title": map[string]any{"tag": "plain_text", "content": title},
		},
		"body": map[string]any{"elements": elements},
	}
	return string(MustJSON(card))
}

func (m *Messenger) fetchTenantAccessToken(ctx context.Context) (string, error) {
	body := map[string]string{"app_id": m.appID, "app_secret": m.appSecret}
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := m.postJSON(ctx, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", "", body, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 || strings.TrimSpace(resp.TenantAccessToken) == "" {
		return "", fmt.Errorf("fetch tenant_access_token failed: %s", resp.Msg)
	}
	return resp.TenantAccessToken, nil
}

func (m *Messenger) postJSON(ctx context.Context, url, token string, reqBody interface{}, out interface{}) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("feishu api status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode feishu response failed: %w body=%s", err, strings.TrimSpace(string(body)))
	}
	return nil
}

func MustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
