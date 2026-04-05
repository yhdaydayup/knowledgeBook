package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"knowledgebook/internal/agent"
	"knowledgebook/internal/config"
	"knowledgebook/internal/feishu"
	"knowledgebook/internal/llm"
	"knowledgebook/internal/model"
	"knowledgebook/internal/repository"
)

// Services wires repository access, runtime agent resources and optional LLM integration.
type Services struct {
	Store *repository.Store
	Cfg   config.Config
	Agent *agent.Runtime
	LLM   llm.Client
}

// New constructs the service layer with repository access, runtime prompts and optional LLM support.
func New(store *repository.Store, cfg config.Config, runtimeAgent *agent.Runtime, llmClient llm.Client) *Services {
	return &Services{Store: store, Cfg: cfg, Agent: runtimeAgent, LLM: llmClient}
}

type BotCommandResult struct {
	Command      string      `json:"command"`
	Reply        string      `json:"reply"`
	CardMarkdown string      `json:"cardMarkdown,omitempty"`
	Data         interface{} `json:"data,omitempty"`
}

type CreateKnowledgeRequest struct {
	OpenID           string   `json:"openId"`
	UserName         string   `json:"userName"`
	Title            string   `json:"title"`
	Content          string   `json:"content"`
	CategoryPath     string   `json:"categoryPath"`
	Tags             []string `json:"tags"`
	Source           string   `json:"source"`
	ChatID           string   `json:"chatId"`
	SourceMessageID  string   `json:"sourceMessageId"`
	ReplyMessageID   string   `json:"replyToMessageId"`
	QuotedText       string   `json:"quotedText"`
}

type UpdateKnowledgeRequest struct {
	OpenID       string `json:"openId"`
	UserName     string `json:"userName"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	CategoryPath string `json:"categoryPath"`
	ChangeReason string `json:"changeReason"`
}

type SyncFromDocRequest struct {
	OpenID       string `json:"openId"`
	UserName     string `json:"userName"`
	Content      string `json:"content"`
	ChangeReason string `json:"changeReason"`
}

func summarize(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= 60 {
		return text
	}
	r := []rune(text)
	return string(r[:60]) + "..."
}

func recommendCategory(content, explicit string) (string, float64, bool) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit, 1, true
	}
	content = strings.ToLower(content)
	switch {
	case strings.Contains(content, "接口") || strings.Contains(content, "api") || strings.Contains(content, "登录"):
		return "工作/默认项目/接口设计", 0.91, true
	case strings.Contains(content, "需求") || strings.Contains(content, "prd"):
		return "工作/默认项目/需求讨论", 0.82, false
	default:
		return "默认/待分类", 0.5, false
	}
}

func (s *Services) ensureUser(ctx context.Context, openID, userName string) (*model.User, error) {
	return s.Store.EnsureUser(ctx, openID, userName)
}

func (s *Services) CreateDraftLegacy(ctx context.Context, req CreateKnowledgeRequest) (*model.Draft, error) {
	user, err := s.ensureUser(ctx, req.OpenID, req.UserName)
	if err != nil {
		return nil, err
	}
	recPath, conf, auto := recommendCategory(req.Content, req.CategoryPath)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = summarize(req.Content)
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	return s.Store.CreateDraft(ctx, user.ID, defaultString(req.Source, "BOT_MESSAGE"), req.Content, title, summarize(req.Content), req.Content, tags, recPath, conf, auto)
}

func (s *Services) ApproveDraft(ctx context.Context, openID, userName string, draftID int64, categoryPath string) (*model.KnowledgeItem, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	draft, err := s.Store.GetStructuredDraft(ctx, user.ID, draftID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(draft.Status, "PENDING_CONFIRMATION") {
		return nil, fmt.Errorf("draft already resolved")
	}
	if categoryPath == "" {
		categoryPath = draft.RecommendedCategoryPath
	}
	return s.Store.CreateKnowledgeFromDraft(ctx, user.ID, draftID, categoryPath)
}

func (s *Services) IgnoreDraft(ctx context.Context, openID, userName string, draftID int64) error {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return err
	}
	return s.Store.UpdateDraftStatus(ctx, user.ID, draftID, "IGNORED")
}

func (s *Services) LaterDraft(ctx context.Context, openID, userName string, draftID int64) error {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return err
	}
	return s.Store.UpdateDraftStatus(ctx, user.ID, draftID, "LATER")
}

func (s *Services) ListLater(ctx context.Context, openID, userName string) ([]model.Draft, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.ListLaterDrafts(ctx, user.ID)
}

func (s *Services) Search(ctx context.Context, openID, userName, query, category string) ([]model.SearchResult, string, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, "", err
	}
	return s.Store.SearchKnowledge(ctx, user.ID, query, category)
}

func (s *Services) GetKnowledge(ctx context.Context, openID, userName string, id int64) (*model.KnowledgeItem, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.GetKnowledge(ctx, user.ID, id)
}

func (s *Services) UpdateKnowledge(ctx context.Context, id int64, req UpdateKnowledgeRequest) (*model.KnowledgeItem, error) {
	user, err := s.ensureUser(ctx, req.OpenID, req.UserName)
	if err != nil {
		return nil, err
	}
	return s.Store.UpdateKnowledge(ctx, user.ID, id, req.Title, req.Content, req.CategoryPath, defaultString(req.ChangeReason, "manual update"))
}

func (s *Services) MoveKnowledge(ctx context.Context, openID, userName string, id int64, categoryPath string) (*model.KnowledgeItem, error) {
	return s.UpdateKnowledge(ctx, id, UpdateKnowledgeRequest{OpenID: openID, UserName: userName, CategoryPath: categoryPath, ChangeReason: "move category"})
}

func (s *Services) SoftDeleteKnowledge(ctx context.Context, openID, userName string, id int64) error {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return err
	}
	return s.Store.SoftDeleteKnowledge(ctx, user.ID, id, s.Cfg.SoftDeleteRetentionDays)
}

func (s *Services) RestoreKnowledge(ctx context.Context, openID, userName string, id int64) error {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return err
	}
	return s.Store.RestoreKnowledge(ctx, user.ID, id)
}

func (s *Services) SyncFromDoc(ctx context.Context, id int64, req SyncFromDocRequest) (*model.KnowledgeItem, error) {
	user, err := s.ensureUser(ctx, req.OpenID, req.UserName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("DOC_BACKFILL_NO_DIFF")
	}
	return s.Store.BackfillFromDoc(ctx, user.ID, id, req.Content, defaultString(req.ChangeReason, "doc sync backfill"))
}

func (s *Services) CreateCategory(ctx context.Context, openID, userName, path string) (int64, string, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return 0, "", err
	}
	return s.Store.EnsureCategoryPath(ctx, user.ID, path, "manual")
}

func (s *Services) ListCategories(ctx context.Context, openID, userName string) ([]model.Category, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.ListCategoryTree(ctx, user.ID)
}

func formatSearchReply(answer string, items []model.SearchResult) string {
	if len(items) == 0 {
		return answer
	}
	lines := []string{answer}
	for i, item := range items {
		line := fmt.Sprintf("%d. #%d %s [%s]", i+1, item.KnowledgeID, item.Title, item.CategoryPath)
		if item.DocAnchorLink != "" {
			line += " " + item.DocAnchorLink
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func splitTitleAndContent(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) == 1 {
		return "", strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func (s *Services) ExecuteBotCommand(ctx context.Context, openID, userName, text string) (*BotCommandResult, error) {
	parsed := feishu.ParseCommand(text)
	if parsed.Namespace != "kb" {
		return nil, fmt.Errorf("unsupported command namespace")
	}
	if userName == "" {
		userName = openID
	}
	usage := "支持命令：/kb add 标题 | 内容；/kb search 关键词；/kb approve 草稿ID [分类路径]"
	switch parsed.Name {
	case "help", "":
		return &BotCommandResult{Command: "help", Reply: usage}, nil
	case "add", "create":
		title, content := splitTitleAndContent(strings.Join(parsed.Args, " "))
		if content == "" {
			return nil, fmt.Errorf("usage: /kb add 标题 | 内容")
		}
		draft, err := s.CreateDraft(ctx, CreateKnowledgeRequest{
			OpenID:       openID,
			UserName:     userName,
			Title:        title,
			Content:      content,
			Source:       "FEISHU_BOT_COMMAND",
			CategoryPath: "",
		})
		if err != nil {
			return nil, err
		}
		reply := fmt.Sprintf("已创建草稿 #%d：%s。推荐分类：%s。可继续执行 /kb approve %d", draft.ID, draft.Title, defaultString(draft.RecommendedCategoryPath, "默认/待分类"), draft.ID)
		return &BotCommandResult{Command: "add", Reply: reply, Data: draft}, nil
	case "search", "find", "query":
		query := strings.TrimSpace(strings.Join(parsed.Args, " "))
		if query == "" {
			return nil, fmt.Errorf("usage: /kb search 关键词")
		}
		result, err := s.SearchAnswer(ctx, openID, userName, query, "")
		if err != nil {
			return nil, err
		}
		return &BotCommandResult{Command: "search", Reply: formatSearchAnswerReply(result), Data: result}, nil
	case "approve":
		if len(parsed.Args) == 0 {
			return nil, fmt.Errorf("usage: /kb approve 草稿ID [分类路径]")
		}
		draftID, err := feishu.ParseInt64(parsed.Args[0])
		if err != nil {
			return nil, fmt.Errorf("invalid draft id")
		}
		categoryPath := strings.TrimSpace(strings.Join(parsed.Args[1:], " "))
		item, err := s.ApproveDraft(ctx, openID, userName, draftID, categoryPath)
		if err != nil {
			return nil, err
		}
		reply := fmt.Sprintf("已确认草稿 #%d，生成知识 #%d：%s", draftID, item.ID, item.Title)
		return &BotCommandResult{Command: "approve", Reply: reply, Data: item}, nil
	default:
		return nil, fmt.Errorf("unsupported command: %s", parsed.Name)
	}
}

func (s *Services) TriggerSync(ctx context.Context, taskType string, targetID int64) error {
	return s.Store.CreateDocSyncTask(ctx, taskType, "knowledge", targetID, `{"reason":"manual_trigger"}`)
}

func (s *Services) ProcessTasks(ctx context.Context) error {
	tasks, err := s.Store.ClaimRunnableTasks(ctx, 10)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := s.processTask(ctx, task); err != nil {
			_ = s.Store.MarkTaskFailure(ctx, task.ID, task.RetryCount+1, err.Error())
			continue
		}
		_ = s.Store.MarkTaskSuccess(ctx, task.ID)
	}
	_, _ = s.ExpirePendingDrafts(ctx, 100)
	_, _ = s.RemindPendingDrafts(ctx, 15*time.Minute, 20)
	_, _ = s.Store.PurgeExpiredKnowledge(ctx)
	return nil
}

func (s *Services) processTask(ctx context.Context, task model.Task) error {
	switch task.TaskType {
	case "DOC_SYNC_KNOWLEDGE":
		item, err := s.Store.GetKnowledgeByIDAnyState(ctx, task.TargetID)
		if err != nil {
			return err
		}
		if item.PrimaryCategoryID == nil {
			return fmt.Errorf("category missing")
		}
		client := feishu.NewClient()
		result, err := client.SyncKnowledge(ctx, feishu.DocSyncInput{
			KnowledgeID: item.ID,
			Version:     item.CurrentVersion,
			Title:       item.Title,
			Content:     item.ContentMarkdown,
			UserID:      item.UserID,
			CategoryID:  *item.PrimaryCategoryID,
			BaseURL:     s.Cfg.FeishuDocBaseURL,
			AppID:       s.Cfg.FeishuAppID,
			AppSecret:   s.Cfg.FeishuAppSecret,
		})
		if err != nil {
			return err
		}
		if err := s.Store.UpsertDocMapping(ctx, item.ID, item.PrimaryCategoryID, result.TargetDocID, result.TargetBlockID, result.ParentBlockID, result.ExternalKey, result.AnchorKey, result.DocLink, result.AnchorLink, result.SyncStatus, item.CurrentVersion); err != nil {
			return err
		}
		_, err = s.Store.DB.Exec(ctx, `UPDATE knowledge_items SET doc_link=$1, doc_anchor_link=$2, updated_at=NOW() WHERE id=$3`, result.DocLink, result.AnchorLink, item.ID)
		return err
	case "DOC_SYNC_CATEGORY", "DOC_REBUILD_ALL", "PURGE_SOFT_DELETED":
		return nil
	default:
		return nil
	}
}

func (s *Services) ListPendingDrafts(ctx context.Context, openID, userName, chatID string, limit int) ([]model.Draft, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.ListPendingDraftsByChat(ctx, user.ID, chatID, limit)
}

func (s *Services) ExpirePendingDrafts(ctx context.Context, limit int) (map[string]any, error) {
	ids, err := s.Store.ExpirePendingDrafts(ctx, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"expiredCount": len(ids), "draftIds": ids}, nil
}

func (s *Services) RemindPendingDrafts(ctx context.Context, before time.Duration, limit int) (int, error) {
	drafts, err := s.Store.ListDraftsNeedingReminder(ctx, before, limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, draft := range drafts {
		if err := s.Store.MarkDraftReminded(ctx, draft.ID); err == nil {
			count++
		}
	}
	return count, nil
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func SleepUntilNextTick() time.Duration { return 30 * time.Second }
