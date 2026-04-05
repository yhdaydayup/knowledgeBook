package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"knowledgebook/internal/llm"
	"knowledgebook/internal/model"
)

// llmExtractedDraft mirrors the constrained JSON shape returned by the extractor prompt.
type llmExtractedDraft struct {
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	KeyPoints    []string `json:"key_points"`
	Tags         []string `json:"tags"`
	CategoryHint string   `json:"category_hint"`
	Confidence   float64  `json:"confidence"`
}

// llmSimilarityJudgement mirrors the constrained JSON shape returned by the similarity prompt.
type llmSimilarityJudgement struct {
	RelationType    string  `json:"relation_type"`
	Score           float64 `json:"score"`
	Reason          string  `json:"reason"`
	SuggestedAction string  `json:"suggested_action"`
}

// parseIntentWithFallback prefers the external LLM but safely falls back to rule-based intent parsing.
func (s *Services) parseIntentWithFallback(ctx context.Context, text string) model.IntentResult {
	fallback := normalizeIntentResult(parseIntent(text), text)
	if !s.llmAvailable() {
		return fallback
	}
	payload, err := s.LLM.GenerateJSON(ctx, llm.GenerateJSONRequest{
		TaskName:       "intent_parser",
		SystemPrompt:   s.Agent.Prompt("intent"),
		UserPrompt:     fmt.Sprintf("请分析下面的用户输入，并按 schema 返回。\n\n用户输入：%s", text),
		Temperature:    0,
		MaxTokens:      500,
		ResponseSchema: s.Agent.Schema("intent"),
	})
	if err != nil {
		return handleLLMError(s, "intent_parser", err, fallback)
	}
	var result model.IntentResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return handleLLMError(s, "intent_parser_decode", err, fallback)
	}
	if !isValidIntent(result.Intent) {
		return handleLLMError(s, "intent_parser_validate", fmt.Errorf("invalid intent: %s", result.Intent), fallback)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return handleLLMError(s, "intent_parser_validate", fmt.Errorf("invalid confidence"), fallback)
	}
	result = normalizeIntentResult(result, text)
	if shouldPreferIntentFallback(result, fallback) {
		log.Printf("llm task intent_parser fallback_preferred: llm_intent=%s llm_confidence=%.2f fallback_intent=%s fallback_confidence=%.2f", result.Intent, result.Confidence, fallback.Intent, fallback.Confidence)
		return fallback
	}
	return result
}

// extractDraftWithFallback prefers the external LLM but falls back to deterministic local extraction.
func (s *Services) extractDraftWithFallback(ctx context.Context, explicitTitle, raw string) extractedDraft {
	fallback := extractDraft(explicitTitle, raw)
	if !s.llmAvailable() {
		return fallback
	}
	payload, err := s.LLM.GenerateJSON(ctx, llm.GenerateJSONRequest{
		TaskName:       "knowledge_extractor",
		SystemPrompt:   s.Agent.Prompt("draft"),
		UserPrompt:     fmt.Sprintf("请把下面的原始输入提取成结构化知识草稿。\n\n显式标题：%s\n原始输入：%s", explicitTitle, raw),
		Temperature:    0.2,
		MaxTokens:      700,
		ResponseSchema: s.Agent.Schema("draft"),
	})
	if err != nil {
		return handleLLMError(s, "knowledge_extractor", err, fallback)
	}
	var result llmExtractedDraft
	if err := json.Unmarshal(payload, &result); err != nil {
		return handleLLMError(s, "knowledge_extractor_decode", err, fallback)
	}
	if err := validateExtractedDraft(result); err != nil {
		return handleLLMError(s, "knowledge_extractor_validate", err, fallback)
	}
	if strings.TrimSpace(explicitTitle) != "" {
		result.Title = strings.TrimSpace(explicitTitle)
	}
	return extractedDraft{
		Title:        strings.TrimSpace(result.Title),
		Summary:      strings.TrimSpace(result.Summary),
		Points:       normalizeStringSlice(result.KeyPoints),
		Tags:         normalizeStringSlice(result.Tags),
		CategoryHint: strings.TrimSpace(result.CategoryHint),
		Confidence:   result.Confidence,
	}
}

// judgeSimilarityWithFallback asks the external LLM for relation judgement after deterministic recall.
func (s *Services) judgeSimilarityWithFallback(ctx context.Context, baseText string, candidates []model.KnowledgeItem) []model.SimilarityRecord {
	fallback := buildSimilarityRecords(baseText, candidates)
	if !s.llmAvailable() || len(candidates) == 0 {
		return fallback
	}
	items := make([]model.SimilarityRecord, 0, len(candidates))
	for _, candidate := range candidates {
		payload, err := s.LLM.GenerateJSON(ctx, llm.GenerateJSONRequest{
			TaskName:       "similarity_judge",
			SystemPrompt:   s.Agent.Prompt("similarity"),
			UserPrompt:     similarityPrompt(baseText, candidate),
			Temperature:    0,
			MaxTokens:      400,
			ResponseSchema: s.Agent.Schema("similarity"),
		})
		if err != nil {
			return handleLLMError(s, "similarity_judge", err, fallback)
		}
		var judgement llmSimilarityJudgement
		if err := json.Unmarshal(payload, &judgement); err != nil {
			return handleLLMError(s, "similarity_judge_decode", err, fallback)
		}
		if err := validateSimilarityJudgement(judgement); err != nil {
			return handleLLMError(s, "similarity_judge_validate", err, fallback)
		}
		items = append(items, model.SimilarityRecord{
			KnowledgeID:     candidate.ID,
			Title:           candidate.Title,
			Summary:         candidate.Summary,
			CategoryPath:    candidate.CategoryPath,
			DocAnchorLink:   candidate.DocAnchorLink,
			SimilarityScore: judgement.Score,
			RelationType:    judgement.RelationType,
			Reason:          strings.TrimSpace(judgement.Reason),
			SuggestedAction: strings.TrimSpace(judgement.SuggestedAction),
		})
	}
	items = filterSimilarityRecords(items)
	if len(items) == 0 {
		return fallback
	}
	return items
}

// composeAnswerWithFallback prefers the external LLM while preserving evidence-bound fallback output.
func (s *Services) composeAnswerWithFallback(ctx context.Context, query string, evidence []model.SearchResult) *model.SearchAnswer {
	fallbackAnswer := composeSearchAnswer(query, evidence)
	fallback := &model.SearchAnswer{Query: query, Answer: fallbackAnswer, Evidence: evidence, Related: []model.SimilarityRecord{}, Conflicts: []model.SimilarityRecord{}}
	if !s.llmAvailable() {
		return fallback
	}
	evidenceJSON, _ := json.Marshal(evidence)
	payload, err := s.LLM.GenerateJSON(ctx, llm.GenerateJSONRequest{
		TaskName:       "answer_composer",
		SystemPrompt:   s.Agent.Prompt("answer"),
		UserPrompt:     fmt.Sprintf("请基于以下 query 和 evidence 生成最终回答。不得编造 evidence 外的事实。\n\nquery: %s\n\nevidence: %s", query, string(evidenceJSON)),
		Temperature:    0.2,
		MaxTokens:      800,
		ResponseSchema: s.Agent.Schema("answer"),
	})
	if err != nil {
		return handleLLMError(s, "answer_composer", err, fallback)
	}
	var result model.SearchAnswer
	if err := json.Unmarshal(payload, &result); err != nil {
		return handleLLMError(s, "answer_composer_decode", err, fallback)
	}
	if strings.TrimSpace(result.Answer) == "" {
		return handleLLMError(s, "answer_composer_validate", fmt.Errorf("empty answer"), fallback)
	}
	result.Query = query
	result.Evidence = evidence
	result.Related = normalizeSimilaritySlice(result.Related)
	result.Conflicts = normalizeSimilaritySlice(result.Conflicts)
	return &result
}

func (s *Services) llmAvailable() bool {
	return s != nil && s.Cfg.LLMEnabled && s.Agent != nil && s.LLM != nil && s.LLM.Enabled()
}

func handleLLMError[T any](s *Services, task string, err error, fallback T) T {
	_ = s
	log.Printf("llm task %s fallback: %v", task, err)
	return fallback
}

func normalizeIntentResult(result model.IntentResult, text string) model.IntentResult {
	if result.Slots == nil {
		result.Slots = map[string]any{}
	}
	if _, ok := result.Slots["raw_text"]; !ok {
		result.Slots["raw_text"] = text
	}
	switch result.Intent {
	case "create_knowledge":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "create_draft"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text_and_card"
		}
	case "approve_pending_draft":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "resolve_pending_then_confirm"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text"
		}
	case "reject_pending_draft":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "resolve_pending_then_reject"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text"
		}
	case "change_pending_draft_category":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "resolve_pending_then_change_category"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text_and_card"
		}
		if categoryPath := stringSlot(result.Slots, "category_path"); categoryPath == "" {
			if extracted := extractCategoryPath(text); extracted != "" {
				result.Slots["category_path"] = extracted
			}
		}
	case "search_knowledge":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "search_knowledge"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text"
		}
		if stringSlot(result.Slots, "query") == "" {
			result.Slots["query"] = text
		}
	case "check_similarity":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "check_similarity"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text"
		}
	case "clarify":
		if strings.TrimSpace(result.Action) == "" {
			result.Action = "clarify"
		}
		if strings.TrimSpace(result.ResponseMode) == "" {
			result.ResponseMode = "text"
		}
	}
	return result
}

func shouldPreferIntentFallback(llmResult, fallback model.IntentResult) bool {
	if llmResult.Intent == "clarify" && fallback.Intent != "clarify" {
		return true
	}
	if llmResult.Confidence < 0.6 && fallback.Intent != "clarify" {
		return true
	}
	if llmResult.Intent == "search_knowledge" && fallback.Intent == "create_knowledge" && llmResult.Confidence < 0.75 {
		return true
	}
	if llmResult.Intent == "clarify" && utf8.RuneCountInString(stringSlot(fallback.Slots, "raw_text")) >= 10 {
		return true
	}
	return false
}

func isValidIntent(intent string) bool {
	switch intent {
	case "create_knowledge", "approve_pending_draft", "reject_pending_draft", "change_pending_draft_category", "search_knowledge", "check_similarity", "clarify":
		return true
	default:
		return false
	}
}

func validateExtractedDraft(d llmExtractedDraft) error {
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("empty title")
	}
	if strings.TrimSpace(d.Summary) == "" {
		return fmt.Errorf("empty summary")
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("invalid confidence")
	}
	return nil
}

func similarityPrompt(baseText string, candidate model.KnowledgeItem) string {
	candidateJSON, _ := json.Marshal(map[string]any{
		"knowledge_id":    candidate.ID,
		"title":           candidate.Title,
		"summary":         candidate.Summary,
		"content":         candidate.ContentMarkdown,
		"category_path":   candidate.CategoryPath,
		"doc_anchor_link": candidate.DocAnchorLink,
	})
	return fmt.Sprintf("请判断下面的新输入与候选知识的关系。\n\n新输入：%s\n\n候选知识：%s", baseText, string(candidateJSON))
}

func validateSimilarityJudgement(j llmSimilarityJudgement) error {
	switch j.RelationType {
	case "merge_candidate", "supplement_candidate", "conflict_candidate", "new_knowledge":
	default:
		return fmt.Errorf("invalid relation_type: %s", j.RelationType)
	}
	if j.Score < 0 || j.Score > 1 {
		return fmt.Errorf("invalid score")
	}
	if strings.TrimSpace(j.Reason) == "" {
		return fmt.Errorf("empty reason")
	}
	return nil
}

func normalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return uniqueStrings(items)
}

func filterSimilarityRecords(items []model.SimilarityRecord) []model.SimilarityRecord {
	filtered := make([]model.SimilarityRecord, 0, len(items))
	for _, item := range items {
		if item.RelationType == "new_knowledge" && item.SimilarityScore < 0.18 {
			continue
		}
		if strings.TrimSpace(item.SuggestedAction) == "" {
			item.SuggestedAction = similarityAction(item.RelationType)
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return filtered
	}
	for i := 0; i < len(filtered)-1; i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[j].SimilarityScore > filtered[i].SimilarityScore {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}
	if len(filtered) > 3 {
		filtered = filtered[:3]
	}
	return filtered
}

func normalizeSimilaritySlice(items []model.SimilarityRecord) []model.SimilarityRecord {
	if len(items) == 0 {
		return []model.SimilarityRecord{}
	}
	for i := range items {
		items[i].Title = strings.TrimSpace(items[i].Title)
		items[i].Summary = strings.TrimSpace(items[i].Summary)
		items[i].CategoryPath = strings.TrimSpace(items[i].CategoryPath)
		items[i].Reason = strings.TrimSpace(items[i].Reason)
		items[i].SuggestedAction = strings.TrimSpace(items[i].SuggestedAction)
	}
	return items
}
