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

type DocSyncInput struct {
	KnowledgeID int64
	Version     int
	Title       string
	Content     string
	UserID      int64
	CategoryID  int64
	BaseURL     string
	AppID       string
	AppSecret   string
}

type DocSyncResult struct {
	TargetDocID   string
	TargetBlockID string
	ParentBlockID string
	ExternalKey   string
	AnchorKey     string
	DocLink       string
	AnchorLink    string
	SyncStatus    string
	Synthetic     bool
}

type DocSyncClient interface {
	SyncKnowledge(ctx context.Context, in DocSyncInput) (*DocSyncResult, error)
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 15 * time.Second}}
}

func categoryDocID(userID, categoryID int64) string {
	return fmt.Sprintf("kb-u%d-c%d", userID, categoryID)
}

func knowledgeExternalKey(knowledgeID int64) string {
	return fmt.Sprintf("knowledge-%d", knowledgeID)
}

func knowledgeBlockID(knowledgeID int64) string {
	return fmt.Sprintf("kb-item-%d", knowledgeID)
}

func (c *Client) SyncKnowledge(ctx context.Context, in DocSyncInput) (*DocSyncResult, error) {
	baseURL := strings.TrimRight(in.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://www.feishu.cn/docx"
	}
	targetDocID := categoryDocID(in.UserID, in.CategoryID)
	parentBlockID := targetDocID
	targetBlockID := knowledgeBlockID(in.KnowledgeID)
	externalKey := knowledgeExternalKey(in.KnowledgeID)
	anchorKey := SyntheticAnchor(in.KnowledgeID, in.Version)
	if strings.TrimSpace(in.AppID) == "" || strings.TrimSpace(in.AppSecret) == "" || strings.Contains(in.AppID, "xxx") || strings.Contains(in.AppSecret, "xxx") {
		return c.simulatedResult(in, baseURL, targetDocID, parentBlockID, targetBlockID, externalKey, anchorKey), nil
	}
	token, err := c.fetchTenantAccessToken(ctx, in.AppID, in.AppSecret)
	if err != nil {
		return nil, err
	}
	docID, err := c.ensureCategoryDocument(ctx, token, in.Title, targetDocID)
	if err != nil {
		return nil, err
	}
	if err := c.upsertKnowledgeBlock(ctx, token, docID, parentBlockID, targetBlockID, in.Title, in.Content); err != nil {
		return nil, err
	}
	docLink := fmt.Sprintf("%s/%s", baseURL, docID)
	return &DocSyncResult{
		TargetDocID:   docID,
		TargetBlockID: targetBlockID,
		ParentBlockID: parentBlockID,
		ExternalKey:   externalKey,
		AnchorKey:     anchorKey,
		DocLink:       docLink,
		AnchorLink:    docLink + "#" + anchorKey,
		SyncStatus:    "SUCCESS",
		Synthetic:     false,
	}, nil
}

func (c *Client) simulatedResult(in DocSyncInput, baseURL, targetDocID, parentBlockID, targetBlockID, externalKey, anchorKey string) *DocSyncResult {
	docLink := fmt.Sprintf("%s/%s", baseURL, targetDocID)
	return &DocSyncResult{
		TargetDocID:   targetDocID,
		TargetBlockID: targetBlockID,
		ParentBlockID: parentBlockID,
		ExternalKey:   externalKey,
		AnchorKey:     anchorKey,
		DocLink:       docLink,
		AnchorLink:    docLink + "#" + anchorKey,
		SyncStatus:    "SIMULATED",
		Synthetic:     true,
	}
}

func (c *Client) fetchTenantAccessToken(ctx context.Context, appID, appSecret string) (string, error) {
	body := map[string]string{"app_id": appID, "app_secret": appSecret}
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := c.postJSON(ctx, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", "", body, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 || strings.TrimSpace(resp.TenantAccessToken) == "" {
		return "", fmt.Errorf("fetch tenant_access_token failed: %s", resp.Msg)
	}
	return resp.TenantAccessToken, nil
}

func (c *Client) ensureCategoryDocument(ctx context.Context, token, title, preferredDocID string) (string, error) {
	if strings.TrimSpace(preferredDocID) != "" {
		return preferredDocID, nil
	}
	body := map[string]string{"title": defaultTitle(title)}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Document struct {
				DocumentID string `json:"document_id"`
			} `json:"document"`
		} `json:"data"`
	}
	if err := c.postJSON(ctx, "https://open.feishu.cn/open-apis/docx/v1/documents", token, body, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 || strings.TrimSpace(resp.Data.Document.DocumentID) == "" {
		return "", fmt.Errorf("create doc failed: %s", resp.Msg)
	}
	return resp.Data.Document.DocumentID, nil
}

func (c *Client) upsertKnowledgeBlock(ctx context.Context, token, documentID, parentBlockID, targetBlockID, title, content string) error {
	_ = targetBlockID
	body := map[string]interface{}{
		"children": []map[string]interface{}{
			{
				"block_type": 2,
				"heading1": map[string]interface{}{
					"elements": []map[string]interface{}{{
						"text_run": map[string]string{"content": defaultTitle(title)},
					}},
				},
			},
			{
				"block_type": 3,
				"text": map[string]interface{}{
					"elements": []map[string]interface{}{{
						"text_run": map[string]string{"content": strings.TrimSpace(content)},
					}},
				},
			},
		},
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	url := fmt.Sprintf("https://open.feishu.cn/open-apis/docx/v1/documents/%s/blocks/%s/children", documentID, parentBlockID)
	if err := c.postJSON(ctx, url, token, body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("append doc block failed: %s", resp.Msg)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, url, token string, reqBody interface{}, out interface{}) error {
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
	resp, err := c.httpClient.Do(req)
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

func defaultTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "knowledgeBook Sync"
	}
	return title
}

func SyntheticAnchor(knowledgeID int64, version int) string {
	return fmt.Sprintf("k-%d-v-%d", knowledgeID, version)
}
