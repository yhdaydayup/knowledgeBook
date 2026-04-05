package model

import "time"

type APIResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

type User struct {
	ID        int64     `json:"id"`
	OpenID    string    `json:"openId"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Draft struct {
	ID                       int64                  `json:"id"`
	UserID                   int64                  `json:"userId"`
	InputType                string                 `json:"inputType"`
	InputText                string                 `json:"inputText"`
	Title                    string                 `json:"title"`
	Summary                  string                 `json:"summary"`
	ContentMarkdown          string                 `json:"contentMarkdown"`
	Tags                     []string               `json:"tags"`
	RawContent               string                 `json:"rawContent,omitempty"`
	NormalizedTitle          string                 `json:"normalizedTitle,omitempty"`
	NormalizedSummary        string                 `json:"normalizedSummary,omitempty"`
	NormalizedPoints         []string               `json:"normalizedPoints,omitempty"`
	RecommendedCategoryPath  string                 `json:"recommendedCategoryPath"`
	RecommendationConfidence float64                `json:"recommendationConfidence"`
	AutoAcceptedCategory     bool                   `json:"autoAcceptedCategory"`
	LLMConfidence            float64                `json:"llmConfidence,omitempty"`
	ChatID                   string                 `json:"chatId,omitempty"`
	SourceMessageID          string                 `json:"sourceMessageId,omitempty"`
	ReplyToMessageID         string                 `json:"replyToMessageId,omitempty"`
	CardMessageID            string                 `json:"cardMessageId,omitempty"`
	Status                   string                 `json:"status"`
	ReviewedAt               *time.Time             `json:"reviewedAt,omitempty"`
	ExpiresAt                *time.Time             `json:"expiresAt,omitempty"`
	ResolvedAt               *time.Time             `json:"resolvedAt,omitempty"`
	LastRemindedAt           *time.Time             `json:"lastRemindedAt,omitempty"`
	InteractionContext       map[string]interface{} `json:"interactionContext,omitempty"`
	CreatedAt                time.Time              `json:"createdAt"`
	UpdatedAt                time.Time              `json:"updatedAt"`
}

type KnowledgeItem struct {
	ID                     int64      `json:"id"`
	UserID                 int64      `json:"userId"`
	DraftID                *int64     `json:"draftId,omitempty"`
	Title                  string     `json:"title"`
	Summary                string     `json:"summary"`
	ContentMarkdown        string     `json:"contentMarkdown"`
	Tags                   []string   `json:"tags"`
	PrimaryCategoryID      *int64     `json:"primaryCategoryId,omitempty"`
	CategoryPath           string     `json:"categoryPath"`
	Confidence             float64    `json:"confidence"`
	Status                 string     `json:"status"`
	CurrentVersion         int        `json:"currentVersion"`
	AutoClassified         bool       `json:"autoClassified"`
	AutoClassifyConfidence float64    `json:"autoClassifyConfidence"`
	DocLink                string     `json:"docLink"`
	DocAnchorLink          string     `json:"docAnchorLink"`
	RemovedAt              *time.Time `json:"removedAt,omitempty"`
	PurgeAt                *time.Time `json:"purgeAt,omitempty"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type Category struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	Name       string     `json:"name"`
	ParentID   *int64     `json:"parentId,omitempty"`
	Level      int        `json:"level"`
	Path       string     `json:"path"`
	PathKey    string     `json:"pathKey"`
	SortOrder  int        `json:"sortOrder"`
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	DocNodeKey string     `json:"docNodeKey"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	Children   []Category `json:"children,omitempty"`
}

type SearchResult struct {
	KnowledgeID   int64     `json:"knowledgeId"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary,omitempty"`
	CategoryPath  string    `json:"categoryPath"`
	UpdatedAt     time.Time `json:"updatedAt"`
	DocAnchorLink string    `json:"docAnchorLink"`
}

type SearchAnswer struct {
	Query     string             `json:"query"`
	Answer    string             `json:"answer"`
	Evidence  []SearchResult     `json:"evidence"`
	Related   []SimilarityRecord `json:"related"`
	Conflicts []SimilarityRecord `json:"conflicts"`
}

type SimilarityRecord struct {
	DraftID         int64     `json:"draftId,omitempty"`
	KnowledgeID     int64     `json:"knowledgeId"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary,omitempty"`
	CategoryPath    string    `json:"categoryPath"`
	DocAnchorLink   string    `json:"docAnchorLink,omitempty"`
	SimilarityScore float64   `json:"similarityScore"`
	RelationType    string    `json:"relationType"`
	Reason          string    `json:"reason"`
	SuggestedAction string    `json:"suggestedAction"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
}

type IntentResult struct {
	Intent             string         `json:"intent"`
	Confidence         float64        `json:"confidence"`
	NeedsClarification bool           `json:"needs_clarification"`
	Action             string         `json:"action,omitempty"`
	ResponseMode       string         `json:"response_mode,omitempty"`
	Slots              map[string]any `json:"slots"`
}

type BotConversationResult struct {
	Intent       string      `json:"intent"`
	Reply        string      `json:"reply"`
	CardMarkdown string      `json:"cardMarkdown,omitempty"`
	Data         interface{} `json:"data,omitempty"`
}

type Task struct {
	ID         int64     `json:"id"`
	TaskType   string    `json:"taskType"`
	TargetType string    `json:"targetType"`
	TargetID   int64     `json:"targetId"`
	Payload    string    `json:"payload"`
	Status     string    `json:"status"`
	RetryCount int       `json:"retryCount"`
	LastError  string    `json:"lastError"`
	RunAfter   time.Time `json:"runAfter"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
