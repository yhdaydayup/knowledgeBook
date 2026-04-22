package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"knowledgebook/internal/model"
	"knowledgebook/internal/repository"
)

// extractedDraft is the normalized draft shape consumed by the conversation flow.
type extractedDraft struct {
	Title        string
	Summary      string
	Points       []string
	Tags         []string
	CategoryHint string
	Confidence   float64
}

// ExecuteBotMessage dispatches bot input to command mode or conversation mode.
func (s *Services) ExecuteBotMessage(ctx context.Context, openID, userName, text string) (*BotCommandResult, error) {
	return s.ExecuteBotMessageWithContext(ctx, openID, userName, text, nil)
}

func (s *Services) ExecuteBotMessageWithContext(ctx context.Context, openID, userName, text string, runtimeCtx map[string]any) (*BotCommandResult, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/kb") {
		return s.ExecuteBotCommand(ctx, openID, userName, text)
	}
	result, err := s.HandleConversationWithContext(ctx, openID, userName, text, runtimeCtx)
	if err != nil {
		return nil, err
	}
	return &BotCommandResult{Command: result.Intent, Reply: result.Reply, CardMarkdown: result.CardMarkdown, Data: result.Data}, nil
}

// CreateDraft builds and stores a structured draft before final confirmation.
func (s *Services) CreateDraft(ctx context.Context, req CreateKnowledgeRequest) (*model.Draft, error) {
	user, err := s.ensureUser(ctx, req.OpenID, req.UserName)
	if err != nil {
		return nil, err
	}
	extracted := s.extractDraftWithFallback(ctx, req.Title, req.Content)
	recPath, conf, auto := recommendCategory(req.Content, defaultString(req.CategoryPath, extracted.CategoryHint))
	tags := req.Tags
	if len(tags) == 0 {
		tags = extracted.Tags
	}
	llmConfidence := extracted.Confidence
	if llmConfidence < conf {
		llmConfidence = conf
	}
	expiresAt := time.Now().Add(time.Hour)
	topPaths, _ := s.Store.ListTopCategoryPaths(ctx, user.ID, 5)
	candidates := recommendCandidateCategories(req.Content, extracted.CategoryHint, topPaths)
	interactionContext := map[string]interface{}{
		"quoted_text":           strings.TrimSpace(req.QuotedText),
		"candidate_categories":  candidates,
		"similarity_candidates": []interface{}{},
		"source_channel":        "feishu",
	}
	return s.Store.CreateStructuredDraft(ctx, repository.CreateDraftParams{
		UserID:                   user.ID,
		InputType:                defaultString(req.Source, "BOT_MESSAGE"),
		InputText:                req.Content,
		Title:                    extracted.Title,
		Summary:                  extracted.Summary,
		ContentMarkdown:          req.Content,
		Tags:                     tags,
		RecommendedCategoryPath:  recPath,
		RecommendationConfidence: conf,
		AutoAcceptedCategory:     auto,
		RawContent:               req.Content,
		NormalizedTitle:          extracted.Title,
		NormalizedSummary:        extracted.Summary,
		NormalizedPoints:         extracted.Points,
		LLMConfidence:            llmConfidence,
		ChatID:                   strings.TrimSpace(req.ChatID),
		SourceMessageID:          strings.TrimSpace(req.SourceMessageID),
		ReplyToMessageID:         strings.TrimSpace(req.ReplyMessageID),
		ExpiresAt:                &expiresAt,
		InteractionContext:       interactionContext,
	})
}

func (s *Services) GetDraft(ctx context.Context, openID, userName string, draftID int64) (*model.Draft, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.GetStructuredDraft(ctx, user.ID, draftID)
}

// SearchAnswer returns answer plus evidence for one user query.
func (s *Services) SearchAnswer(ctx context.Context, openID, userName, query, category string) (*model.SearchAnswer, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	evidence, err := s.Store.SearchKnowledgeEvidence(ctx, user.ID, query, category, 5)
	if err != nil {
		return nil, err
	}
	return s.composeAnswerWithFallback(ctx, query, evidence), nil
}

// CheckSimilarity compares new text or a draft against recalled knowledge candidates.
func (s *Services) CheckSimilarity(ctx context.Context, openID, userName, text string, draftID int64) ([]model.SimilarityRecord, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	baseText := strings.TrimSpace(text)
	if draftID > 0 {
		draft, err := s.Store.GetStructuredDraft(ctx, user.ID, draftID)
		if err != nil {
			return nil, err
		}
		baseText = draft.RawContent
		if strings.TrimSpace(baseText) == "" {
			baseText = draft.ContentMarkdown
		}
	}
	candidates, err := s.Store.FindKnowledgeCandidates(ctx, user.ID, baseText, 5)
	if err != nil {
		return nil, err
	}
	records := s.judgeSimilarityWithFallback(ctx, baseText, candidates)
	if draftID > 0 {
		for i := range records {
			records[i].DraftID = draftID
		}
		if err := s.Store.ReplaceDraftSimilarities(ctx, draftID, records); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (s *Services) ListSimilarityCandidates(ctx context.Context, openID, userName string, draftID int64) ([]model.SimilarityRecord, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetStructuredDraft(ctx, user.ID, draftID); err != nil {
		return nil, err
	}
	return s.Store.ListDraftSimilarities(ctx, draftID)
}

func (s *Services) GetRelatedKnowledge(ctx context.Context, openID, userName string, knowledgeID int64) ([]model.SimilarityRecord, error) {
	item, err := s.GetKnowledge(ctx, openID, userName, knowledgeID)
	if err != nil {
		return nil, err
	}
	return s.CheckSimilarity(ctx, openID, userName, item.Title+"\n"+item.ContentMarkdown, 0)
}

// HandleConversation is the orchestrator entry for natural-language bot interaction.
func (s *Services) HandleConversation(ctx context.Context, openID, userName, text string) (*model.BotConversationResult, error) {
	return s.HandleConversationWithContext(ctx, openID, userName, text, nil)
}

func (s *Services) HandleConversationWithContext(ctx context.Context, openID, userName, text string, runtimeCtx map[string]any) (*model.BotConversationResult, error) {
	text = strings.TrimSpace(text)
	selectedAction := contextString(runtimeCtx, "selected_action")
	selectedDraftID := contextInt64(runtimeCtx, "selected_draft_id")
	if selectedDraftID > 0 && isDraftSelectionReply(text) {
		text = selectionActionText(selectedAction)
	}
	chatID := contextString(runtimeCtx, "chat_id")
	messageID := contextString(runtimeCtx, "message_id")
	replyToMessageID := contextString(runtimeCtx, "reply_to_message_id")
	quotedText := contextString(runtimeCtx, "quoted_text")
	var convCtx *ConversationContext
	if s.llmAvailable() && strings.TrimSpace(chatID) != "" {
		user, _ := s.ensureUser(ctx, openID, userName)
		if user != nil {
			count, _ := s.Store.CountPendingDraftsByChat(ctx, user.ID, chatID)
			var lastTitle string
			if count > 0 {
				if drafts, err := s.Store.ListPendingDraftsByChat(ctx, user.ID, chatID, 1); err == nil && len(drafts) > 0 {
					lastTitle = drafts[0].Title
				}
			}
			convCtx = &ConversationContext{PendingDraftCount: count, LastDraftTitle: lastTitle, ChatID: chatID}
		}
	}
	intent := s.parseIntentWithContext(ctx, text, convCtx)
	if intent.NeedsClarification {
		return &model.BotConversationResult{Intent: intent.Intent, Reply: "我还没完全理解你的意思。你可以直接告诉我：想记录什么、想查什么、是否确认保存，或者要改到哪个分类。我也能继续承接上一条草稿操作。", Data: intent}, nil
	}
	switch intent.Intent {
	case "create_knowledge":
		draft, err := s.CreateDraft(ctx, CreateKnowledgeRequest{
			OpenID:          openID,
			UserName:        userName,
			Title:           stringSlot(intent.Slots, "title"),
			Content:         stringSlot(intent.Slots, "raw_text"),
			CategoryPath:    stringSlot(intent.Slots, "category_hint"),
			Source:          "FEISHU_BOT_NL",
			ChatID:          chatID,
			SourceMessageID: messageID,
			ReplyMessageID:  replyToMessageID,
			QuotedText:      quotedText,
		})
		if err != nil {
			return nil, err
		}
		records, err := s.CheckSimilarity(ctx, openID, userName, draft.RawContent, draft.ID)
		if err != nil {
			return nil, err
		}
		data := map[string]any{"intent": intent, "draft": draft, "similarities": records}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: composeCreateReply(draft, records), CardMarkdown: composeDraftCard(draft, records), Data: data}, nil
	case "approve_pending_draft":
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, selectedDraftID, quotedText)
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return &model.BotConversationResult{Intent: intent.Intent, Reply: resolved.ClarifyReply, Data: map[string]any{"intent": intent, "pending": resolved}}, nil
		}
		item, err := s.ApproveDraft(ctx, openID, userName, resolved.Draft.ID, stringSlot(intent.Slots, "category_path"))
		if err != nil {
			return nil, err
		}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: fmt.Sprintf("已确认草稿 #%d，生成知识 #%d：%s", resolved.Draft.ID, item.ID, item.Title), Data: map[string]any{"intent": intent, "draft": resolved.Draft, "item": item}}, nil
	case "reject_pending_draft":
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, selectedDraftID, quotedText)
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return &model.BotConversationResult{Intent: intent.Intent, Reply: resolved.ClarifyReply, Data: map[string]any{"intent": intent, "pending": resolved}}, nil
		}
		if err := s.RejectDraft(ctx, openID, userName, resolved.Draft.ID); err != nil {
			return nil, err
		}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: fmt.Sprintf("已丢弃草稿 #%d：%s", resolved.Draft.ID, resolved.Draft.Title), Data: map[string]any{"intent": intent, "draft": resolved.Draft}}, nil
	case "change_pending_draft_category":
		resolved, err := s.ResolvePendingDraftContext(ctx, openID, userName, chatID, messageID, replyToMessageID, selectedDraftID, quotedText)
		if err != nil {
			return nil, err
		}
		if resolved.NeedsClarification {
			return &model.BotConversationResult{Intent: intent.Intent, Reply: resolved.ClarifyReply, Data: map[string]any{"intent": intent, "pending": resolved}}, nil
		}
		categoryPath := stringSlot(intent.Slots, "category_path")
		if categoryPath == "" {
			return &model.BotConversationResult{Intent: intent.Intent, Reply: "请直接告诉我你希望修改到哪个完整分类路径，例如：改到 软件开发/接口治理。", Data: map[string]any{"intent": intent, "pending": resolved}}, nil
		}
		updatedDraft, err := s.UpdateDraftCategory(ctx, openID, userName, resolved.Draft.ID, categoryPath)
		if err != nil {
			return nil, err
		}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: fmt.Sprintf("已将草稿 #%d 的推荐分类改为：%s", updatedDraft.ID, updatedDraft.RecommendedCategoryPath), CardMarkdown: composeDraftCard(updatedDraft, nil), Data: map[string]any{"intent": intent, "draft": updatedDraft}}, nil
	case "search_knowledge":
		answer, err := s.SearchAnswer(ctx, openID, userName, stringSlot(intent.Slots, "query"), stringSlot(intent.Slots, "category_hint"))
		if err != nil {
			return nil, err
		}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: formatSearchAnswerReply(answer), Data: map[string]any{"intent": intent, "result": answer}}, nil
	case "check_similarity":
		records, err := s.CheckSimilarity(ctx, openID, userName, stringSlot(intent.Slots, "raw_text"), 0)
		if err != nil {
			return nil, err
		}
		return &model.BotConversationResult{Intent: intent.Intent, Reply: composeSimilarityReply(records), Data: map[string]any{"intent": intent, "similarities": records}}, nil
	default:
		return &model.BotConversationResult{Intent: "clarify", Reply: "我现在支持新增知识、确认保存、拒绝保存、修改分类、查询知识和检查相似，也可以继续使用 /kb 命令。", Data: intent}, nil
	}
}

func extractDraft(explicitTitle, raw string) extractedDraft {
	raw = strings.TrimSpace(raw)
	title := strings.TrimSpace(explicitTitle)
	lines := splitNonEmptyLines(raw)
	if title == "" && len(lines) > 0 {
		title = truncateRunes(lines[0], 28)
	}
	if title == "" {
		title = truncateRunes(raw, 28)
	}
	points := extractPoints(raw)
	summary := truncateRunes(strings.Join(points, "；"), 120)
	if summary == "" {
		summary = truncateRunes(raw, 120)
	}
	tags := inferTags(raw)
	categoryHint, _, _ := recommendCategory(raw, "")
	confidence := 0.62
	if len(points) >= 2 {
		confidence += 0.08
	}
	if len(tags) > 0 {
		confidence += 0.05
	}
	if title != "" && summary != "" {
		confidence += 0.08
	}
	if confidence > 0.95 {
		confidence = 0.95
	}
	return extractedDraft{Title: title, Summary: summary, Points: points, Tags: tags, CategoryHint: categoryHint, Confidence: confidence}
}

func parseIntent(text string) model.IntentResult {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if text == "" {
		return model.IntentResult{Intent: "clarify", Confidence: 0.1, NeedsClarification: true, Action: "clarify", ResponseMode: "text", Slots: map[string]any{}}
	}
	if containsAnyText(lower, "不要保存", "先不存", "先别存", "不用保存", "不记了", "不需要保存", "算了", "放弃", "丢弃", "拒绝保存") {
		return model.IntentResult{Intent: "reject_pending_draft", Confidence: 0.94, NeedsClarification: false, Action: "resolve_pending_then_reject", ResponseMode: "text", Slots: map[string]any{"raw_text": text}}
	}
	if containsAnyText(lower, "改到", "改成", "换到", "换成", "挪到", "放到", "归到", "分类不对", "分类改", "修改分类", "换个分类") {
		categoryPath := extractCategoryPath(text)
		return model.IntentResult{Intent: "change_pending_draft_category", Confidence: 0.9, NeedsClarification: false, Action: "resolve_pending_then_change_category", ResponseMode: "text_and_card", Slots: map[string]any{"raw_text": text, "category_path": categoryPath}}
	}
	if containsAnyText(lower, "确认保存", "确认一下", "就按这个存", "按这个保存", "存吧", "保存吧", "可以保存", "就这么定", "没问题，保存", "好，保存") {
		return model.IntentResult{Intent: "approve_pending_draft", Confidence: 0.94, NeedsClarification: false, Action: "resolve_pending_then_confirm", ResponseMode: "text", Slots: map[string]any{"raw_text": text}}
	}
	if containsAnyText(lower, "重复", "相似", "是不是同一", "是不是一回事", "一样吗", "是否重复", "像不像", "冲突吗", "有没有冲突", "有点像") {
		return model.IntentResult{Intent: "check_similarity", Confidence: 0.9, NeedsClarification: false, Action: "check_similarity", ResponseMode: "text", Slots: map[string]any{"raw_text": text}}
	}
	if containsAnyText(lower, "查一下", "看一下", "搜一下", "搜搜", "搜索", "查询", "找一下", "帮我找", "之前", "是什么", "怎么说", "有没有", "记得吗", "提过吗") {
		return model.IntentResult{Intent: "search_knowledge", Confidence: 0.86, NeedsClarification: false, Action: "search_knowledge", ResponseMode: "text", Slots: map[string]any{"query": text, "raw_text": text}}
	}
	if containsAnyText(lower, "记一下", "记住", "帮我记", "记录一下", "记录下", "存一下", "存起来", "整理一下", "沉淀一下", "新增知识", "整理成知识", "同步一下这个结论") {
		return model.IntentResult{Intent: "create_knowledge", Confidence: 0.9, NeedsClarification: false, Action: "create_draft", ResponseMode: "text_and_card", Slots: map[string]any{"raw_text": text}}
	}
	if seemsKnowledgeStatement(text, lower) {
		return model.IntentResult{Intent: "create_knowledge", Confidence: 0.7, NeedsClarification: false, Action: "create_draft", ResponseMode: "text_and_card", Slots: map[string]any{"raw_text": text}}
	}
	if strings.ContainsAny(text, "?？") || containsAnyText(lower, "想知道", "想确认", "想问", "想查") {
		return model.IntentResult{Intent: "search_knowledge", Confidence: 0.58, NeedsClarification: false, Action: "search_knowledge", ResponseMode: "text", Slots: map[string]any{"query": text, "raw_text": text}}
	}
	return model.IntentResult{Intent: "clarify", Confidence: 0.3, NeedsClarification: true, Action: "clarify", ResponseMode: "text", Slots: map[string]any{"raw_text": text}}
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func seemsKnowledgeStatement(text, lower string) bool {
	if utf8.RuneCountInString(strings.TrimSpace(text)) < 8 {
		return false
	}
	if strings.ContainsAny(text, "?？") {
		return false
	}
	if containsAnyText(lower, "怎么", "为何", "为什么", "哪里", "谁", "吗", "么") {
		return false
	}
	if containsAnyText(lower, "结论", "原因", "背景", "方案", "规则", "约束", "要求", "流程", "处理方式", "注意", "风险", "最佳实践", "排查", "经验", "已确认") {
		return true
	}
	return utf8.RuneCountInString(text) >= 18
}

func extractCategoryPath(text string) string {
	markers := []string{"改到", "改成", "放到", "分类改到", "分类改成", "换到", "换成", "归到", "挪到"}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			return strings.TrimSpace(strings.Trim(text[idx+len(marker):], "：:，,。 "))
		}
	}
	return ""
}

func stringSlot(slots map[string]any, key string) string {
	if slots == nil {
		return ""
	}
	value, _ := slots[key].(string)
	return strings.TrimSpace(value)
}

func splitNonEmptyLines(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimLeft(part, "-•*0123456789.、"))
		if part != "" {
			lines = append(lines, part)
		}
	}
	return lines
}

func extractPoints(text string) []string {
	lines := splitNonEmptyLines(text)
	points := make([]string, 0, 3)
	for _, line := range lines {
		points = append(points, truncateRunes(line, 40))
		if len(points) == 3 {
			return points
		}
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '。', '！', '？', ';', '；':
			return true
		default:
			return false
		}
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		points = append(points, truncateRunes(part, 40))
		if len(points) == 3 {
			break
		}
	}
	if len(points) == 0 && strings.TrimSpace(text) != "" {
		return []string{truncateRunes(strings.TrimSpace(text), 40)}
	}
	return uniqueStrings(points)
}

func inferTags(text string) []string {
	keywords := map[string]string{
		"feishu":   "feishu",
		"飞书":       "feishu",
		"fts":      "fts",
		"搜索":       "search",
		"检索":       "search",
		"缓存":       "cache",
		"登录":       "auth",
		"鉴权":       "auth",
		"权限":       "auth",
		"同步":       "sync",
		"数据库":      "database",
		"postgres": "postgres",
		"mcp":      "mcp",
	}
	set := map[string]struct{}{}
	lower := strings.ToLower(text)
	for needle, tag := range keywords {
		if strings.Contains(lower, strings.ToLower(needle)) {
			set[tag] = struct{}{}
		}
	}
	items := make([]string, 0, len(set))
	for tag := range set {
		items = append(items, tag)
	}
	sort.Strings(items)
	if len(items) > 5 {
		items = items[:5]
	}
	return items
}

func buildSimilarityRecords(baseText string, candidates []model.KnowledgeItem) []model.SimilarityRecord {
	items := make([]model.SimilarityRecord, 0, len(candidates))
	base := normalizeText(baseText)
	for _, candidate := range candidates {
		target := normalizeText(candidate.Title + " " + candidate.Summary + " " + candidate.ContentMarkdown)
		score := ngramSimilarity(base, target)
		relationType := classifyRelation(base, target, score)
		if relationType == "new_knowledge" && score < 0.18 {
			continue
		}
		items = append(items, model.SimilarityRecord{
			KnowledgeID:     candidate.ID,
			Title:           candidate.Title,
			Summary:         candidate.Summary,
			CategoryPath:    candidate.CategoryPath,
			DocAnchorLink:   candidate.DocAnchorLink,
			SimilarityScore: score,
			RelationType:    relationType,
			Reason:          similarityReason(relationType, score, candidate.Title),
			SuggestedAction: similarityAction(relationType),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SimilarityScore > items[j].SimilarityScore
	})
	if len(items) > 3 {
		items = items[:3]
	}
	return items
}

func normalizeText(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func ngramSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	tagsA := bigrams(a)
	tagsB := bigrams(b)
	if len(tagsA) == 0 || len(tagsB) == 0 {
		if a == b {
			return 1
		}
		return 0
	}
	intersect := 0
	for gram := range tagsA {
		if _, ok := tagsB[gram]; ok {
			intersect++
		}
	}
	union := len(tagsA) + len(tagsB) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func bigrams(text string) map[string]struct{} {
	runes := []rune(text)
	grams := map[string]struct{}{}
	if len(runes) < 2 {
		if len(runes) == 1 {
			grams[string(runes[0])] = struct{}{}
		}
		return grams
	}
	for i := 0; i < len(runes)-1; i++ {
		grams[string(runes[i:i+2])] = struct{}{}
	}
	return grams
}

func classifyRelation(base, target string, score float64) string {
	if score >= 0.58 {
		return "merge_candidate"
	}
	if score >= 0.32 {
		if hasConflictSignal(base, target) {
			return "conflict_candidate"
		}
		return "supplement_candidate"
	}
	return "new_knowledge"
}

func hasConflictSignal(base, target string) bool {
	negative := []string{"未", "失败", "不支持", "禁止", "回滚", "异常"}
	positive := []string{"已", "成功", "支持", "完成", "开启", "通过"}
	baseNegative := containsAny(base, negative)
	targetNegative := containsAny(target, negative)
	basePositive := containsAny(base, positive)
	targetPositive := containsAny(target, positive)
	return (baseNegative && targetPositive) || (basePositive && targetNegative)
}

func containsAny(text string, items []string) bool {
	for _, item := range items {
		if strings.Contains(text, normalizeText(item)) {
			return true
		}
	}
	return false
}

func similarityReason(relationType string, score float64, title string) string {
	switch relationType {
	case "merge_candidate":
		return fmt.Sprintf("与《%s》的内容高度重合，建议先检查是否应合并或补充。", title)
	case "conflict_candidate":
		return fmt.Sprintf("与《%s》存在部分重合，但结论信号可能不一致，需要人工确认。", title)
	case "supplement_candidate":
		return fmt.Sprintf("与《%s》主题接近，适合作为补充信息。", title)
	default:
		return fmt.Sprintf("当前仅发现弱相关结果（相似度 %.2f），更像一条新知识。", score)
	}
}

func similarityAction(relationType string) string {
	switch relationType {
	case "merge_candidate":
		return "建议先查看已有知识再决定是否新建"
	case "conflict_candidate":
		return "建议人工确认冲突结论"
	case "supplement_candidate":
		return "建议保存为补充知识"
	default:
		return "可以直接创建为新知识"
	}
}

func composeSearchAnswer(query string, evidence []model.SearchResult) string {
	if len(evidence) == 0 {
		return fmt.Sprintf("没有找到和“%s”直接相关的知识。你可以换个说法，或者改用 /kb search 关键词。", query)
	}
	top := evidence[0]
	answer := fmt.Sprintf("根据当前知识库，最相关的是《%s》。", top.Title)
	if top.Summary != "" {
		answer += " 摘要：" + top.Summary
	}
	if len(evidence) > 1 {
		answer += fmt.Sprintf(" 另外还有 %d 条可作为补充证据。", len(evidence)-1)
	}
	return answer
}

func formatSearchAnswerReply(answer *model.SearchAnswer) string {
	lines := []string{answer.Answer}
	for i, item := range answer.Evidence {
		line := fmt.Sprintf("%d. #%d %s [%s]", i+1, item.KnowledgeID, item.Title, item.CategoryPath)
		if item.DocAnchorLink != "" {
			line += " " + item.DocAnchorLink
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func composeCreateReply(draft *model.Draft, records []model.SimilarityRecord) string {
	lines := []string{fmt.Sprintf("已生成知识草稿 #%d：%s", draft.ID, draft.Title)}
	if draft.NormalizedSummary != "" {
		lines = append(lines, "摘要："+draft.NormalizedSummary)
	}
	if len(draft.NormalizedPoints) > 0 {
		lines = append(lines, "要点："+strings.Join(draft.NormalizedPoints, "；"))
	}
	if draft.RecommendedCategoryPath != "" {
		lines = append(lines, "推荐分类："+draft.RecommendedCategoryPath)
	}
	if draft.ExpiresAt != nil {
		lines = append(lines, "有效期：1 小时内确认，否则自动失效")
	}
	if len(records) > 0 {
		lines = append(lines, "发现相似知识：")
		for i, item := range records {
			lines = append(lines, fmt.Sprintf("%d. #%d %s（%s，%.2f）", i+1, item.KnowledgeID, item.Title, item.RelationType, item.SimilarityScore))
		}
	}
	lines = append(lines, "你可以直接回复“确认保存”“不要保存”或“改到 软件开发/接口治理”。")
	lines = append(lines, fmt.Sprintf("兼容命令：/kb approve %d", draft.ID))
	return strings.Join(lines, "\n")
}

func composeDraftCard(draft *model.Draft, records []model.SimilarityRecord) string {
	lines := []string{"# 待确认知识草稿", fmt.Sprintf("- 草稿ID：%d", draft.ID), fmt.Sprintf("- 标题：%s", draft.Title)}
	if draft.NormalizedSummary != "" {
		lines = append(lines, fmt.Sprintf("- 摘要：%s", draft.NormalizedSummary))
	}
	if len(draft.NormalizedPoints) > 0 {
		lines = append(lines, fmt.Sprintf("- 要点：%s", strings.Join(draft.NormalizedPoints, "；")))
	}
	if draft.RecommendedCategoryPath != "" {
		lines = append(lines, fmt.Sprintf("- 推荐分类：%s", draft.RecommendedCategoryPath))
	}
	if draft.InteractionContext != nil {
		if raw, ok := draft.InteractionContext["candidate_categories"]; ok {
			if arr, ok := raw.([]interface{}); ok && len(arr) > 0 {
				var paths []string
				for _, v := range arr {
					if s, ok := v.(string); ok && s != "" {
						paths = append(paths, s)
					}
				}
				if len(paths) > 0 {
					lines = append(lines, fmt.Sprintf("- 候选分类：%s", strings.Join(paths, " | ")))
				}
			}
		}
	}
	if len(records) > 0 {
		lines = append(lines, "- 相似知识：")
		for i, item := range records {
			lines = append(lines, fmt.Sprintf("  %d. #%d %s（%s，%.2f）", i+1, item.KnowledgeID, item.Title, item.RelationType, item.SimilarityScore))
		}
	}
	lines = append(lines, "- 操作：确认保存 / 拒绝保存 / 修改分类")
	lines = append(lines, "- 提示：1 小时内确认，否则自动失效")
	return strings.Join(lines, "\n")
}

func contextString(runtimeCtx map[string]any, key string) string {
	if runtimeCtx == nil {
		return ""
	}
	value, _ := runtimeCtx[key].(string)
	return strings.TrimSpace(value)
}

func contextInt64(runtimeCtx map[string]any, key string) int64 {
	if runtimeCtx == nil {
		return 0
	}
	switch value := runtimeCtx[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return id
	default:
		return 0
	}
}

func isDraftSelectionReply(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func selectionActionText(action string) string {
	switch strings.TrimSpace(action) {
	case "reject_pending_draft":
		return "不要保存"
	case "change_pending_draft_category":
		return "修改分类"
	default:
		return "确认保存"
	}
}

type PendingDraftContext struct {
	Draft              *model.Draft   `json:"draft,omitempty"`
	Candidates         []model.Draft  `json:"candidates,omitempty"`
	PendingCount       int            `json:"pendingCount"`
	MatchMode          string         `json:"matchMode,omitempty"`
	NeedsClarification bool           `json:"needsClarification"`
	ClarifyReply       string         `json:"clarifyReply,omitempty"`
}

func (s *Services) ResolvePendingDraftContext(ctx context.Context, openID, userName, chatID, messageID, replyToMessageID string, selectedDraftID int64, quotedText string) (*PendingDraftContext, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	if selectedDraftID > 0 {
		draft, err := s.Store.GetStructuredDraft(ctx, user.ID, selectedDraftID)
		if err == nil && strings.EqualFold(draft.Status, "PENDING_CONFIRMATION") {
			return &PendingDraftContext{Draft: draft, PendingCount: 1, MatchMode: "selection_memory", NeedsClarification: false}, nil
		}
	}
	for _, candidateMessageID := range []string{replyToMessageID, messageID} {
		if candidateMessageID == "" {
			continue
		}
		draft, err := s.Store.GetPendingDraftBySourceMessage(ctx, user.ID, candidateMessageID)
		if err == nil {
			return &PendingDraftContext{Draft: draft, PendingCount: 1, MatchMode: "message_binding", NeedsClarification: false}, nil
		}
	}
	quotedText = strings.TrimSpace(quotedText)
	if quotedText != "" && strings.TrimSpace(chatID) != "" {
		drafts, err := s.Store.ListPendingDraftsByChat(ctx, user.ID, chatID, 10)
		if err == nil {
			normalizedQuote := strings.ToLower(quotedText)
			for i, draft := range drafts {
				normalizedTitle := strings.ToLower(draft.Title)
				normalizedContent := strings.ToLower(draft.RawContent)
				if strings.Contains(normalizedContent, normalizedQuote) || strings.Contains(normalizedTitle, normalizedQuote) || ngramSimilarity(normalizedQuote, normalizedTitle) > 0.5 {
					return &PendingDraftContext{Draft: &drafts[i], PendingCount: 1, MatchMode: "quoted_text_binding", NeedsClarification: false}, nil
				}
			}
		}
	}
	if strings.TrimSpace(chatID) == "" {
		return &PendingDraftContext{NeedsClarification: true, ClarifyReply: "我还没拿到足够上下文来定位你要操作的草稿。你可以直接引用草稿消息，或者重新说一次“确认保存/不要保存/改到 某个分类”。"}, nil
	}
	drafts, err := s.Store.ListPendingDraftsByChat(ctx, user.ID, chatID, 5)
	if err != nil {
		return nil, err
	}
	if len(drafts) == 0 {
		return &PendingDraftContext{PendingCount: 0, NeedsClarification: true, ClarifyReply: "这个会话里现在没有待确认草稿。你可以先告诉我一条新结论，我先帮你整理成草稿。"}, nil
	}
	if len(drafts) == 1 {
		return &PendingDraftContext{Draft: &drafts[0], PendingCount: 1, MatchMode: "unique_pending_in_chat", NeedsClarification: false}, nil
	}
	lines := []string{"当前会话里有多条待确认草稿，我先帮你列出来。你可以直接回复序号，或者点卡片按钮操作："}
	for i, draft := range drafts {
		lines = append(lines, fmt.Sprintf("%d. 草稿 #%d：%s", i+1, draft.ID, draft.Title))
	}
	return &PendingDraftContext{Candidates: drafts, PendingCount: len(drafts), MatchMode: "multiple_pending_in_chat", NeedsClarification: true, ClarifyReply: strings.Join(lines, "\n")}, nil
}

func (s *Services) RejectDraft(ctx context.Context, openID, userName string, draftID int64) error {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return err
	}
	return s.Store.UpdateDraftStatus(ctx, user.ID, draftID, "REJECTED")
}

func (s *Services) UpdateDraftCategory(ctx context.Context, openID, userName string, draftID int64, categoryPath string) (*model.Draft, error) {
	user, err := s.ensureUser(ctx, openID, userName)
	if err != nil {
		return nil, err
	}
	return s.Store.UpdateDraftCategory(ctx, user.ID, draftID, categoryPath)
}

func composeSimilarityReply(records []model.SimilarityRecord) string {
	if len(records) == 0 {
		return "暂时没有发现明显相似的知识，可以按新知识处理。"
	}
	lines := []string{"发现以下相似知识："}
	for i, item := range records {
		line := fmt.Sprintf("%d. #%d %s（%s，%.2f）", i+1, item.KnowledgeID, item.Title, item.RelationType, item.SimilarityScore)
		if item.DocAnchorLink != "" {
			line += " " + item.DocAnchorLink
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func truncateRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}
