package llm

import "context"

type GenerateJSONRequest struct {
	TaskName       string
	SystemPrompt   string
	UserPrompt     string
	Temperature    float64
	MaxTokens      int
	ResponseSchema []byte
}

type Client interface {
	Enabled() bool
	GenerateJSON(ctx context.Context, req GenerateJSONRequest) ([]byte, error)
}
