package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Message represents a single turn in a conversation history.
type Message struct {
	Role       string     `json:"role"`                  // "system", "user", "assistant", "tool"
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Timestamp  int64      `json:"ts"`
}

// ToolCall represents a tool invocation requested by the assistant.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// HistoryStore abstracts conversation history operations so that
// Agent can be tested with an in-memory mock instead of Redis.
type HistoryStore interface {
	Load(ctx context.Context, userID, chatID string) ([]Message, error)
	Append(ctx context.Context, userID, chatID string, msgs ...Message) error
	Clear(ctx context.Context, userID, chatID string) error
}

// History manages per-conversation message history in Redis.
type History struct {
	redis       *redis.Client
	maxMessages int
	ttl         time.Duration
}

// NewHistory creates a History backed by the given Redis client.
func NewHistory(redisClient *redis.Client, maxMessages int, ttl time.Duration) *History {
	if maxMessages <= 0 {
		maxMessages = 20
	}
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	return &History{redis: redisClient, maxMessages: maxMessages, ttl: ttl}
}

func convKey(userID, chatID string) string {
	if chatID == "" {
		chatID = "direct"
	}
	return fmt.Sprintf("kb:conv:%s:%s", userID, chatID)
}

// Append adds one or more messages to the conversation and trims to maxMessages.
func (h *History) Append(ctx context.Context, userID, chatID string, msgs ...Message) error {
	if len(msgs) == 0 {
		return nil
	}
	key := convKey(userID, chatID)
	pipe := h.redis.Pipeline()
	for _, msg := range msgs {
		if msg.Timestamp == 0 {
			msg.Timestamp = time.Now().Unix()
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		pipe.RPush(ctx, key, data)
	}
	pipe.LTrim(ctx, key, int64(-h.maxMessages), -1)
	pipe.Expire(ctx, key, h.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// Load retrieves the recent conversation messages.
func (h *History) Load(ctx context.Context, userID, chatID string) ([]Message, error) {
	key := convKey(userID, chatID)
	vals, err := h.redis.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(vals))
	for _, raw := range vals {
		var msg Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Clear removes all conversation history for the given scope.
func (h *History) Clear(ctx context.Context, userID, chatID string) error {
	return h.redis.Del(ctx, convKey(userID, chatID)).Err()
}
