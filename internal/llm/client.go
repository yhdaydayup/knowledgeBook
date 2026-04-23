package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"knowledgebook/internal/config"
)

// HTTPClient calls an external OpenAI-compatible chat completion endpoint.
type HTTPClient struct {
	enabled    bool
	baseURL    string
	apiKey     string
	model      string
	timeout    time.Duration
	maxTokens  int
	httpClient *http.Client
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	Tools          []ToolDefinition `json:"tools,omitempty"`
	ToolChoice     any              `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewHTTPClient builds the external LLM client from environment-backed config.
func NewHTTPClient(cfg config.Config) Client {
	timeout := time.Duration(cfg.LLMTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	enabled := cfg.LLMEnabled && strings.TrimSpace(cfg.LLMBaseURL) != "" && strings.TrimSpace(cfg.LLMAPIKey) != "" && strings.TrimSpace(cfg.LLMModel) != ""
	return &HTTPClient{
		enabled:    enabled,
		baseURL:    strings.TrimSpace(cfg.LLMBaseURL),
		apiKey:     strings.TrimSpace(cfg.LLMAPIKey),
		model:      strings.TrimSpace(cfg.LLMModel),
		timeout:    timeout,
		maxTokens:  cfg.LLMMaxTokens,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Enabled() bool {
	return c != nil && c.enabled
}

// GenerateJSON sends one structured request and returns the first valid JSON object from the model response.
func (c *HTTPClient) GenerateJSON(ctx context.Context, req GenerateJSONRequest) ([]byte, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("llm client disabled")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	body := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt + schemaHint(req.ResponseSchema)},
		},
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
		ResponseFormat: map[string]any{
			"type": "json_object",
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completionEndpoint(c.baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm api error: %s", strings.TrimSpace(string(respBody)))
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode llm response failed: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("llm api error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm api returned no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("llm api returned empty content")
	}
	return extractJSONObject(content)
}

// Chat sends a multi-turn conversation with optional tool definitions and returns
// the model's reply or tool-call requests.
func (c *HTTPClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("llm client disabled")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	messages := make([]chatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, chatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  m.ToolCalls,
		})
	}
	body := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completionEndpoint(c.baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm api error: %s", strings.TrimSpace(string(respBody)))
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode llm response failed: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("llm api error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm api returned no choices")
	}
	choice := parsed.Choices[0]
	result := &ChatResponse{
		Content:      strings.TrimSpace(choice.Message.Content),
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
	}
	if len(result.ToolCalls) > 0 && result.FinishReason == "" {
		result.FinishReason = "tool_calls"
	}
	return result, nil
}

func completionEndpoint(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

func schemaHint(schema []byte) string {
	if len(schema) == 0 {
		return "\n\n请只返回一个 JSON 对象，不要输出 Markdown 代码块。"
	}
	return "\n\n请只返回一个 JSON 对象，不要输出 Markdown 代码块。输出必须满足以下 schema：\n" + string(schema)
}

func extractJSONObject(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if json.Valid([]byte(content)) {
		return []byte(content), nil
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(content[start : end+1])
		if json.Valid([]byte(candidate)) {
			return []byte(candidate), nil
		}
	}
	return nil, fmt.Errorf("llm content is not valid json")
}
