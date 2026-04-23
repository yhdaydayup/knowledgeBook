package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"knowledgebook/internal/model"
)

// CreateDraftParams carries the normalized draft fields used by controlled draft creation.
type CreateDraftParams struct {
	UserID                   int64
	InputType                string
	InputText                string
	Title                    string
	Summary                  string
	ContentMarkdown          string
	Tags                     []string
	RecommendedCategoryPath  string
	RecommendationConfidence float64
	AutoAcceptedCategory     bool
	RawContent               string
	NormalizedTitle          string
	NormalizedSummary        string
	NormalizedPoints         []string
	LLMConfidence            float64
	ChatID                   string
	SourceMessageID          string
	ReplyToMessageID         string
	CardMessageID            string
	ExpiresAt                *time.Time
	ResolvedAt               *time.Time
	LastRemindedAt           *time.Time
	InteractionContext       map[string]interface{}
}

func marshalPoints(points []string) string {
	if len(points) == 0 {
		return "[]"
	}
	buf, _ := json.Marshal(points)
	return string(buf)
}

func marshalInteractionContext(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return "{}"
	}
	buf, _ := json.Marshal(ctx)
	return string(buf)
}

func scanPoints(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var points []string
	if err := json.Unmarshal(raw, &points); err != nil {
		return []string{}
	}
	return points
}

// CreateStructuredDraft persists a normalized draft with the extra fields used by LLM-assisted flows.
func (s *Store) CreateStructuredDraft(ctx context.Context, params CreateDraftParams) (*model.Draft, error) {
	var d model.Draft
	var normalizedPoints []byte
	var interactionContext []byte
	err := s.DB.QueryRow(ctx, `
INSERT INTO knowledge_drafts (
  user_id,
  input_type,
  input_text,
  title,
  summary,
  content_markdown,
  tags,
  recommended_category_path,
  recommendation_confidence,
  auto_accepted_category,
  raw_content,
  normalized_title,
  normalized_summary,
  normalized_points,
  llm_confidence,
  chat_id,
  source_message_id,
  reply_to_message_id,
  card_message_id,
  expires_at,
  resolved_at,
  last_reminded_at,
  interaction_context,
  status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17,$18,$19,$20,$21,$22,$23::jsonb,$24)
RETURNING id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags,
  COALESCE(raw_content,''), COALESCE(normalized_title,''), COALESCE(normalized_summary,''), COALESCE(normalized_points,'[]'::jsonb),
  COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, COALESCE(llm_confidence,0),
  COALESCE(chat_id,''), COALESCE(source_message_id,''), COALESCE(reply_to_message_id,''), COALESCE(card_message_id,''),
  status, reviewed_at, expires_at, resolved_at, last_reminded_at, COALESCE(interaction_context,'{}'::jsonb), created_at, updated_at
`, params.UserID, params.InputType, params.InputText, params.Title, params.Summary, params.ContentMarkdown, params.Tags, params.RecommendedCategoryPath, params.RecommendationConfidence, params.AutoAcceptedCategory, params.RawContent, params.NormalizedTitle, params.NormalizedSummary, marshalPoints(params.NormalizedPoints), params.LLMConfidence, params.ChatID, params.SourceMessageID, params.ReplyToMessageID, params.CardMessageID, params.ExpiresAt, params.ResolvedAt, params.LastRemindedAt, marshalInteractionContext(params.InteractionContext), "PENDING_CONFIRMATION").Scan(
		&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags,
		&d.RawContent, &d.NormalizedTitle, &d.NormalizedSummary, &normalizedPoints,
		&d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.LLMConfidence,
		&d.ChatID, &d.SourceMessageID, &d.ReplyToMessageID, &d.CardMessageID,
		&d.Status, &d.ReviewedAt, &d.ExpiresAt, &d.ResolvedAt, &d.LastRemindedAt, &interactionContext, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.NormalizedPoints = scanPoints(normalizedPoints)
	_ = json.Unmarshal(interactionContext, &d.InteractionContext)
	return &d, nil
}

// GetStructuredDraft loads a draft together with the normalized fields used by the conversation flow.
func (s *Store) GetStructuredDraft(ctx context.Context, userID, draftID int64) (*model.Draft, error) {
	var d model.Draft
	var normalizedPoints []byte
	var interactionContext []byte
	err := s.DB.QueryRow(ctx, `
SELECT id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags,
  COALESCE(raw_content,''), COALESCE(normalized_title,''), COALESCE(normalized_summary,''), COALESCE(normalized_points,'[]'::jsonb),
  COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, COALESCE(llm_confidence,0),
  COALESCE(chat_id,''), COALESCE(source_message_id,''), COALESCE(reply_to_message_id,''), COALESCE(card_message_id,''),
  status, reviewed_at, expires_at, resolved_at, last_reminded_at, COALESCE(interaction_context,'{}'::jsonb), created_at, updated_at
FROM knowledge_drafts
WHERE id=$1 AND user_id=$2
`, draftID, userID).Scan(
		&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags,
		&d.RawContent, &d.NormalizedTitle, &d.NormalizedSummary, &normalizedPoints,
		&d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.LLMConfidence,
		&d.ChatID, &d.SourceMessageID, &d.ReplyToMessageID, &d.CardMessageID,
		&d.Status, &d.ReviewedAt, &d.ExpiresAt, &d.ResolvedAt, &d.LastRemindedAt, &interactionContext, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.NormalizedPoints = scanPoints(normalizedPoints)
	_ = json.Unmarshal(interactionContext, &d.InteractionContext)
	return &d, nil
}

// SearchKnowledgeEvidence returns the evidence set used by answer composition.
func (s *Store) SearchKnowledgeEvidence(ctx context.Context, userID int64, query, category string, limit int) ([]model.SearchResult, error) {
	query = strings.TrimSpace(query)
	category = normalizePath(category)
	terms := strings.Fields(query)
	if len(terms) == 0 {
		terms = []string{query}
	}
	likeTerms := make([]string, len(terms))
	for i, t := range terms {
		likeTerms[i] = "%" + t + "%"
	}
	rows, err := s.DB.Query(ctx, `
WITH ranked AS (
  SELECT id, title, COALESCE(summary,'') AS summary, category_path, updated_at, COALESCE(doc_anchor_link,'') AS doc_anchor_link,
    CASE WHEN $4 = '' THEN 0 ELSE ts_rank_cd(search_vector, plainto_tsquery('simple', $4)) END AS rank
  FROM knowledge_items
  WHERE user_id=$1
    AND status='ACTIVE'
    AND ($2 = '' OR category_path ILIKE $3)
    AND (
      $4 = ''
      OR search_vector @@ plainto_tsquery('simple', $4)
      OR title ILIKE ANY($5)
      OR summary ILIKE ANY($5)
      OR content_markdown ILIKE ANY($5)
      OR array_to_string(tags, ',') ILIKE ANY($5)
    )
)
SELECT id, title, summary, category_path, updated_at, doc_anchor_link
FROM ranked
ORDER BY rank DESC, updated_at DESC
LIMIT $6
`, userID, category, category+"%", query, likeTerms, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.SearchResult, 0, limit)
	for rows.Next() {
		var it model.SearchResult
		if err := rows.Scan(&it.KnowledgeID, &it.Title, &it.Summary, &it.CategoryPath, &it.UpdatedAt, &it.DocAnchorLink); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// FindKnowledgeCandidates performs deterministic recall before similarity judgement.
func (s *Store) FindKnowledgeCandidates(ctx context.Context, userID int64, text string, limit int) ([]model.KnowledgeItem, error) {
	text = strings.TrimSpace(text)
	like := "%" + text + "%"
	rows, err := s.DB.Query(ctx, `
SELECT id, user_id, draft_id, title, COALESCE(summary,''), content_markdown, tags, primary_category_id, category_path,
  COALESCE(confidence,0), status, current_version, auto_classified, COALESCE(auto_classify_confidence,0),
  COALESCE(doc_link,''), COALESCE(doc_anchor_link,''), removed_at, purge_at, created_at, updated_at
FROM knowledge_items
WHERE user_id=$1
  AND status='ACTIVE'
  AND (
    $2 = ''
    OR search_vector @@ plainto_tsquery('simple', $2)
    OR title ILIKE $3
    OR summary ILIKE $3
    OR content_markdown ILIKE $3
    OR array_to_string(tags, ',') ILIKE $3
  )
ORDER BY updated_at DESC
LIMIT $4
`, userID, text, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.KnowledgeItem, 0, limit)
	for rows.Next() {
		var item model.KnowledgeItem
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.DraftID, &item.Title, &item.Summary, &item.ContentMarkdown, &item.Tags, &item.PrimaryCategoryID, &item.CategoryPath,
			&item.Confidence, &item.Status, &item.CurrentVersion, &item.AutoClassified, &item.AutoClassifyConfidence,
			&item.DocLink, &item.DocAnchorLink, &item.RemovedAt, &item.PurgeAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ReplaceDraftSimilarities refreshes the stored similarity suggestions for one draft.
func (s *Store) ReplaceDraftSimilarities(ctx context.Context, draftID int64, records []model.SimilarityRecord) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_similarity WHERE draft_id=$1`, draftID); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := tx.Exec(ctx, `
INSERT INTO knowledge_similarity (draft_id, knowledge_id, similarity_score, relation_type, reason)
VALUES ($1,$2,$3,$4,$5)
`, draftID, record.KnowledgeID, record.SimilarityScore, record.RelationType, record.Reason); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListDraftSimilarities returns the persisted similarity suggestions for one draft.
func (s *Store) ListDraftSimilarities(ctx context.Context, draftID int64) ([]model.SimilarityRecord, error) {
	rows, err := s.DB.Query(ctx, `
SELECT ks.draft_id, ks.knowledge_id, ki.title, COALESCE(ki.summary,''), ki.category_path, COALESCE(ki.doc_anchor_link,''),
  COALESCE(ks.similarity_score,0), ks.relation_type, COALESCE(ks.reason,''), ks.created_at
FROM knowledge_similarity ks
JOIN knowledge_items ki ON ki.id = ks.knowledge_id
WHERE ks.draft_id=$1
ORDER BY ks.similarity_score DESC, ks.id ASC
`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.SimilarityRecord{}
	for rows.Next() {
		var item model.SimilarityRecord
		if err := rows.Scan(&item.DraftID, &item.KnowledgeID, &item.Title, &item.Summary, &item.CategoryPath, &item.DocAnchorLink, &item.SimilarityScore, &item.RelationType, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.SuggestedAction = suggestedAction(item.RelationType)
		items = append(items, item)
	}
	return items, rows.Err()
}

func suggestedAction(relationType string) string {
	switch relationType {
	case "merge_candidate":
		return "优先检查是否补充到已有知识"
	case "conflict_candidate":
		return "请人工确认是否存在结论冲突"
	case "supplement_candidate":
		return "建议作为补充信息保存"
	default:
		return "可以作为新知识保存"
	}
}
