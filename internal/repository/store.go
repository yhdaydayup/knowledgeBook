package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"knowledgebook/internal/model"
)

type Store struct {
	DB *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{DB: db} }

func (s *Store) EnsureUser(ctx context.Context, openID, name string) (*model.User, error) {
	if openID == "" {
		openID = "local-default-user"
	}
	if name == "" {
		name = "default-user"
	}
	var u model.User
	err := s.DB.QueryRow(ctx, `
INSERT INTO users (open_id, name)
VALUES ($1, $2)
ON CONFLICT (open_id)
DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
RETURNING id, open_id, name, role, status, created_at, updated_at
`, openID, name).Scan(&u.ID, &u.OpenID, &u.Name, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

func (s *Store) EnsureCategoryPath(ctx context.Context, userID int64, path string, source string) (int64, string, error) {
	path = normalizePath(path)
	if path == "" {
		path = "默认/待分类"
	}
	parts := strings.Split(path, "/")
	var parentID *int64
	currentPath := ""
	var lastID int64
	for i, name := range parts {
		if currentPath == "" {
			currentPath = name
		} else {
			currentPath = currentPath + "/" + name
		}
		pathKey := strings.ToLower(currentPath)
		var id int64
		err := s.DB.QueryRow(ctx, `
INSERT INTO categories (user_id, name, parent_id, level, path, path_key, source)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, path_key)
DO UPDATE SET updated_at = NOW()
RETURNING id
`, userID, name, parentID, i+1, currentPath, pathKey, source).Scan(&id)
		if err != nil {
			return 0, "", err
		}
		lastID = id
		parentID = &lastID
	}
	return lastID, currentPath, nil
}

func (s *Store) ListCategoryTree(ctx context.Context, userID int64) ([]model.Category, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, user_id, name, parent_id, level, path, path_key, sort_order, source, status, COALESCE(doc_node_key, ''), created_at, updated_at FROM categories WHERE user_id=$1 ORDER BY level, sort_order, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := make([]model.Category, 0)
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.ParentID, &c.Level, &c.Path, &c.PathKey, &c.SortOrder, &c.Source, &c.Status, &c.DocNodeKey, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	idx := map[int64]*model.Category{}
	for i := range all {
		idx[all[i].ID] = &all[i]
	}
	for i := range all {
		c := &all[i]
		if c.ParentID == nil {
			continue
		}
		if p, ok := idx[*c.ParentID]; ok {
			p.Children = append(p.Children, *c)
		}
	}
	// rebuild roots with nested children from idx
	finalRoots := make([]model.Category, 0)
	for _, c := range all {
		if c.ParentID == nil {
			finalRoots = append(finalRoots, *idx[c.ID])
		}
	}
	return finalRoots, nil
}

func (s *Store) CreateDraft(ctx context.Context, userID int64, inputType, inputText, title, summary, content string, tags []string, recPath string, conf float64, autoAccepted bool) (*model.Draft, error) {
	var d model.Draft
	err := s.DB.QueryRow(ctx, `
INSERT INTO knowledge_drafts (user_id, input_type, input_text, title, summary, content_markdown, tags, recommended_category_path, recommendation_confidence, auto_accepted_category)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags, COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, status, reviewed_at, created_at, updated_at
`, userID, inputType, inputText, title, summary, content, tags, recPath, conf, autoAccepted).Scan(
		&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags, &d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.Status, &d.ReviewedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDraft(ctx context.Context, userID, draftID int64) (*model.Draft, error) {
	var d model.Draft
	err := s.DB.QueryRow(ctx, `SELECT id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags, COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, status, reviewed_at, created_at, updated_at FROM knowledge_drafts WHERE id=$1 AND user_id=$2`, draftID, userID).Scan(
		&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags, &d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.Status, &d.ReviewedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) UpdateDraftStatus(ctx context.Context, userID, draftID int64, status string) error {
	cmd, err := s.DB.Exec(ctx, `UPDATE knowledge_drafts SET status=$1, reviewed_at=NOW(), resolved_at=NOW(), updated_at=NOW() WHERE id=$2 AND user_id=$3 AND status='PENDING_CONFIRMATION'`, status, draftID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListLaterDrafts(ctx context.Context, userID int64) ([]model.Draft, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags, COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, status, reviewed_at, created_at, updated_at FROM knowledge_drafts WHERE user_id=$1 AND status='DEFERRED' ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Draft{}
	for rows.Next() {
		var d model.Draft
		if err := rows.Scan(&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags, &d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.Status, &d.ReviewedAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, nil
}

func (s *Store) ListPendingDraftsByChat(ctx context.Context, userID int64, chatID string, limit int) ([]model.Draft, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.DB.Query(ctx, `
SELECT id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags,
  COALESCE(raw_content,''), COALESCE(normalized_title,''), COALESCE(normalized_summary,''), COALESCE(normalized_points,'[]'::jsonb),
  COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, COALESCE(llm_confidence,0),
  COALESCE(chat_id,''), COALESCE(source_message_id,''), COALESCE(reply_to_message_id,''), COALESCE(card_message_id,''),
  status, reviewed_at, expires_at, resolved_at, last_reminded_at, COALESCE(interaction_context,'{}'::jsonb), created_at, updated_at
FROM knowledge_drafts
WHERE user_id=$1 AND chat_id=$2 AND status='PENDING_CONFIRMATION'
ORDER BY created_at DESC
LIMIT $3
`, userID, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Draft{}
	for rows.Next() {
		var d model.Draft
		var normalizedPoints []byte
		var interactionContext []byte
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags,
			&d.RawContent, &d.NormalizedTitle, &d.NormalizedSummary, &normalizedPoints,
			&d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.LLMConfidence,
			&d.ChatID, &d.SourceMessageID, &d.ReplyToMessageID, &d.CardMessageID,
			&d.Status, &d.ReviewedAt, &d.ExpiresAt, &d.ResolvedAt, &d.LastRemindedAt, &interactionContext, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		d.NormalizedPoints = scanPoints(normalizedPoints)
		_ = json.Unmarshal(interactionContext, &d.InteractionContext)
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s *Store) GetPendingDraftBySourceMessage(ctx context.Context, userID int64, messageID string) (*model.Draft, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, pgx.ErrNoRows
	}
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
WHERE user_id=$1 AND status='PENDING_CONFIRMATION' AND (source_message_id=$2 OR reply_to_message_id=$2 OR card_message_id=$2)
ORDER BY created_at DESC
LIMIT 1
`, userID, messageID).Scan(
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

func (s *Store) UpdateDraftCategory(ctx context.Context, userID, draftID int64, categoryPath string) (*model.Draft, error) {
	var d model.Draft
	var normalizedPoints []byte
	var interactionContext []byte
	err := s.DB.QueryRow(ctx, `
UPDATE knowledge_drafts
SET recommended_category_path=$1, updated_at=NOW()
WHERE id=$2 AND user_id=$3 AND status='PENDING_CONFIRMATION'
RETURNING id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags,
  COALESCE(raw_content,''), COALESCE(normalized_title,''), COALESCE(normalized_summary,''), COALESCE(normalized_points,'[]'::jsonb),
  COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, COALESCE(llm_confidence,0),
  COALESCE(chat_id,''), COALESCE(source_message_id,''), COALESCE(reply_to_message_id,''), COALESCE(card_message_id,''),
  status, reviewed_at, expires_at, resolved_at, last_reminded_at, COALESCE(interaction_context,'{}'::jsonb), created_at, updated_at
`, categoryPath, draftID, userID).Scan(
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

// UpdateDraftCardMessageID stores the Feishu card message ID after sending.
func (s *Store) UpdateDraftCardMessageID(ctx context.Context, draftID int64, cardMessageID string) error {
	_, err := s.DB.Exec(ctx, `UPDATE knowledge_drafts SET card_message_id=$1, updated_at=NOW() WHERE id=$2`, cardMessageID, draftID)
	return err
}

// ListTopCategoryPaths returns the user's most-used category paths.
func (s *Store) ListTopCategoryPaths(ctx context.Context, userID int64, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.DB.Query(ctx, `
SELECT category_path, COUNT(*) AS cnt
FROM knowledge_items
WHERE user_id=$1 AND status='ACTIVE' AND COALESCE(category_path,'') != ''
GROUP BY category_path
ORDER BY cnt DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		var cnt int
		if err := rows.Scan(&path, &cnt); err != nil {
			continue
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// CountPendingDraftsByChat returns the number of pending drafts in a chat.
func (s *Store) CountPendingDraftsByChat(ctx context.Context, userID int64, chatID string) (int, error) {
	var count int
	err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM knowledge_drafts WHERE user_id=$1 AND chat_id=$2 AND status='PENDING_CONFIRMATION'`, userID, chatID).Scan(&count)
	return count, err
}

// ExpiredDraftInfo holds the minimal fields needed to update a card after expiration.
type ExpiredDraftInfo struct {
	ID            int64
	Title         string
	Summary       string
	Category      string
	CardMessageID string
}

func (s *Store) ExpirePendingDrafts(ctx context.Context, limit int) ([]ExpiredDraftInfo, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.Query(ctx, `
UPDATE knowledge_drafts
SET status='EXPIRED', reviewed_at=NOW(), resolved_at=NOW(), updated_at=NOW()
WHERE id IN (
  SELECT id FROM knowledge_drafts
  WHERE status='PENDING_CONFIRMATION' AND expires_at IS NOT NULL AND expires_at <= NOW()
  ORDER BY expires_at ASC
  LIMIT $1
)
RETURNING id, COALESCE(title,''), COALESCE(normalized_summary,''), COALESCE(recommended_category_path,''), COALESCE(card_message_id,'')
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []ExpiredDraftInfo
	for rows.Next() {
		var info ExpiredDraftInfo
		if err := rows.Scan(&info.ID, &info.Title, &info.Summary, &info.Category, &info.CardMessageID); err != nil {
			return nil, err
		}
		results = append(results, info)
	}
	return results, rows.Err()
}

func (s *Store) ListDraftsNeedingReminder(ctx context.Context, reminderBefore time.Duration, limit int) ([]model.Draft, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.Query(ctx, `
SELECT id, user_id, input_type, input_text, COALESCE(title,''), COALESCE(summary,''), COALESCE(content_markdown,''), tags,
  COALESCE(raw_content,''), COALESCE(normalized_title,''), COALESCE(normalized_summary,''), COALESCE(normalized_points,'[]'::jsonb),
  COALESCE(recommended_category_path,''), COALESCE(recommendation_confidence,0), auto_accepted_category, COALESCE(llm_confidence,0),
  COALESCE(chat_id,''), COALESCE(source_message_id,''), COALESCE(reply_to_message_id,''), COALESCE(card_message_id,''),
  status, reviewed_at, expires_at, resolved_at, last_reminded_at, COALESCE(interaction_context,'{}'::jsonb), created_at, updated_at
FROM knowledge_drafts
WHERE status='PENDING_CONFIRMATION'
  AND expires_at IS NOT NULL
  AND expires_at <= NOW() + ($1 * interval '1 second')
  AND (last_reminded_at IS NULL)
ORDER BY expires_at ASC
LIMIT $2
`, int(reminderBefore.Seconds()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Draft{}
	for rows.Next() {
		var d model.Draft
		var normalizedPoints []byte
		var interactionContext []byte
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.InputType, &d.InputText, &d.Title, &d.Summary, &d.ContentMarkdown, &d.Tags,
			&d.RawContent, &d.NormalizedTitle, &d.NormalizedSummary, &normalizedPoints,
			&d.RecommendedCategoryPath, &d.RecommendationConfidence, &d.AutoAcceptedCategory, &d.LLMConfidence,
			&d.ChatID, &d.SourceMessageID, &d.ReplyToMessageID, &d.CardMessageID,
			&d.Status, &d.ReviewedAt, &d.ExpiresAt, &d.ResolvedAt, &d.LastRemindedAt, &interactionContext, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		d.NormalizedPoints = scanPoints(normalizedPoints)
		_ = json.Unmarshal(interactionContext, &d.InteractionContext)
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s *Store) MarkDraftReminded(ctx context.Context, draftID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE knowledge_drafts SET last_reminded_at=NOW(), updated_at=NOW() WHERE id=$1`, draftID)
	return err
}

func (s *Store) CreateKnowledgeFromDraft(ctx context.Context, userID, draftID int64, categoryPath string) (*model.KnowledgeItem, error) {
	draft, err := s.GetDraft(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}
	catID, normalizedPath, err := s.EnsureCategoryPath(ctx, userID, categoryPath, "draft")
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var item model.KnowledgeItem
	err = tx.QueryRow(ctx, `
INSERT INTO knowledge_items (user_id, draft_id, title, summary, content_markdown, tags, primary_category_id, category_path, confidence, current_version, auto_classified, search_vector)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10,
  setweight(to_tsvector('simple', COALESCE($11::text, '')), 'A') ||
  setweight(to_tsvector('simple', COALESCE($12::text, '')), 'B') ||
  setweight(to_tsvector('simple', COALESCE($13::text, '')), 'C') ||
  setweight(to_tsvector('simple', array_to_string(COALESCE($14::text[], '{}'::text[]), ' ')), 'B')
)
RETURNING id, user_id, draft_id, title, summary, content_markdown, tags, primary_category_id, category_path, confidence, status, current_version, auto_classified, COALESCE(auto_classify_confidence,0), COALESCE(doc_link,''), COALESCE(doc_anchor_link,''), removed_at, purge_at, created_at, updated_at
`, userID, draft.ID, draft.Title, draft.Summary, draft.ContentMarkdown, draft.Tags, catID, normalizedPath, draft.RecommendationConfidence, draft.AutoAcceptedCategory, draft.Title, draft.Summary, draft.ContentMarkdown, draft.Tags).Scan(
		&item.ID, &item.UserID, &item.DraftID, &item.Title, &item.Summary, &item.ContentMarkdown, &item.Tags, &item.PrimaryCategoryID, &item.CategoryPath, &item.Confidence, &item.Status, &item.CurrentVersion, &item.AutoClassified, &item.AutoClassifyConfidence, &item.DocLink, &item.DocAnchorLink, &item.RemovedAt, &item.PurgeAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_versions (knowledge_id, version_no, source, title, summary, content_markdown, tags, category_path, editor_user_id, change_reason) VALUES ($1,1,'DRAFT_APPROVE',$2,$3,$4,$5,$6,$7,$8)`, item.ID, item.Title, item.Summary, item.ContentMarkdown, item.Tags, item.CategoryPath, userID, "draft approved")
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE knowledge_drafts SET status='APPROVED', reviewed_at=NOW(), resolved_at=NOW(), updated_at=NOW() WHERE id=$1`, draftID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ('DOC_SYNC_KNOWLEDGE','knowledge',$1,$2)`, item.ID, `{"reason":"draft_approved"}`)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) GetKnowledge(ctx context.Context, userID, knowledgeID int64) (*model.KnowledgeItem, error) {
	var item model.KnowledgeItem
	err := s.DB.QueryRow(ctx, `SELECT id, user_id, draft_id, title, COALESCE(summary,''), content_markdown, tags, primary_category_id, category_path, COALESCE(confidence,0), status, current_version, auto_classified, COALESCE(auto_classify_confidence,0), COALESCE(doc_link,''), COALESCE(doc_anchor_link,''), removed_at, purge_at, created_at, updated_at FROM knowledge_items WHERE id=$1 AND user_id=$2`, knowledgeID, userID).Scan(
		&item.ID, &item.UserID, &item.DraftID, &item.Title, &item.Summary, &item.ContentMarkdown, &item.Tags, &item.PrimaryCategoryID, &item.CategoryPath, &item.Confidence, &item.Status, &item.CurrentVersion, &item.AutoClassified, &item.AutoClassifyConfidence, &item.DocLink, &item.DocAnchorLink, &item.RemovedAt, &item.PurgeAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) UpdateKnowledge(ctx context.Context, userID, knowledgeID int64, title, content, categoryPath, reason string) (*model.KnowledgeItem, error) {
	item, err := s.GetKnowledge(ctx, userID, knowledgeID)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = item.Title
	}
	if content == "" {
		content = item.ContentMarkdown
	}
	if categoryPath == "" {
		categoryPath = item.CategoryPath
	}
	catID, normalizedPath, err := s.EnsureCategoryPath(ctx, userID, categoryPath, "manual")
	if err != nil {
		return nil, err
	}
	nextVersion := item.CurrentVersion + 1
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE knowledge_items SET title=$1, content_markdown=$2, primary_category_id=$3, category_path=$4, current_version=$5, search_vector = setweight(to_tsvector('simple', COALESCE($8::text, '')), 'A') || setweight(to_tsvector('simple', COALESCE(summary, '')), 'B') || setweight(to_tsvector('simple', COALESCE($9::text, '')), 'C') || setweight(to_tsvector('simple', array_to_string(COALESCE(tags, '{}'::text[]), ' ')), 'B'), updated_at=NOW() WHERE id=$6 AND user_id=$7`, title, content, catID, normalizedPath, nextVersion, knowledgeID, userID, title, content)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_versions (knowledge_id, version_no, source, title, summary, content_markdown, tags, category_path, editor_user_id, change_reason) VALUES ($1,$2,'MANUAL_UPDATE',$3,$4,$5,$6,$7,$8,$9)`, knowledgeID, nextVersion, title, item.Summary, content, item.Tags, normalizedPath, userID, reason)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ('DOC_SYNC_KNOWLEDGE','knowledge',$1,$2)`, knowledgeID, `{"reason":"knowledge_updated"}`)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, userID, knowledgeID)
}

func (s *Store) SoftDeleteKnowledge(ctx context.Context, userID, knowledgeID int64, retentionDays int) error {
	cmd, err := s.DB.Exec(ctx, `UPDATE knowledge_items SET status='REMOVED_SOFT', removed_at=NOW(), purge_at=NOW() + ($1 * interval '1 day'), updated_at=NOW() WHERE id=$2 AND user_id=$3 AND status='ACTIVE'`, retentionDays, knowledgeID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, _ = s.DB.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ('DOC_SYNC_KNOWLEDGE','knowledge',$1,$2)`, knowledgeID, `{"reason":"knowledge_soft_deleted"}`)
	return nil
}

func (s *Store) RestoreKnowledge(ctx context.Context, userID, knowledgeID int64) error {
	item, err := s.GetKnowledge(ctx, userID, knowledgeID)
	if err != nil {
		return err
	}
	if item.Status != "REMOVED_SOFT" {
		return fmt.Errorf("knowledge not in removed state")
	}
	if item.PurgeAt != nil && item.PurgeAt.Before(time.Now()) {
		return fmt.Errorf("knowledge expired")
	}
	nextVersion := item.CurrentVersion + 1
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE knowledge_items SET status='ACTIVE', removed_at=NULL, purge_at=NULL, current_version=$1, updated_at=NOW() WHERE id=$2 AND user_id=$3`, nextVersion, knowledgeID, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_versions (knowledge_id, version_no, source, title, summary, content_markdown, tags, category_path, editor_user_id, change_reason) VALUES ($1,$2,'RESTORE',$3,$4,$5,$6,$7,$8,$9)`, knowledgeID, nextVersion, item.Title, item.Summary, item.ContentMarkdown, item.Tags, item.CategoryPath, userID, "restore knowledge")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ('DOC_SYNC_KNOWLEDGE','knowledge',$1,$2)`, knowledgeID, `{"reason":"knowledge_restored"}`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SearchKnowledge(ctx context.Context, userID int64, query, category string) ([]model.SearchResult, string, error) {
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
	SELECT id, title, category_path, updated_at, COALESCE(doc_anchor_link,'') AS doc_anchor_link,
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
SELECT id, title, category_path, updated_at, doc_anchor_link
FROM ranked
ORDER BY rank DESC, updated_at DESC
LIMIT 10
`, userID, category, category+"%", query, likeTerms)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []model.SearchResult{}
	for rows.Next() {
		var it model.SearchResult
		if err := rows.Scan(&it.KnowledgeID, &it.Title, &it.CategoryPath, &it.UpdatedAt, &it.DocAnchorLink); err != nil {
			return nil, "", err
		}
		items = append(items, it)
	}
	answer := "未找到相关知识。"
	if len(items) > 0 {
		answer = fmt.Sprintf("共找到 %d 条相关知识，优先返回相关度更高且最近更新的结果。", len(items))
	}
	return items, answer, nil
}

func (s *Store) CreateDocSyncTask(ctx context.Context, taskType, targetType string, targetID int64, payload string) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ($1,$2,$3,$4)`, taskType, targetType, targetID, payload)
	return err
}

func (s *Store) GetTask(ctx context.Context, taskID int64) (*model.Task, error) {
	var t model.Task
	var payload []byte
	err := s.DB.QueryRow(ctx, `SELECT id, task_type, target_type, target_id, payload, status, retry_count, COALESCE(last_error,''), run_after, created_at, updated_at FROM sync_tasks WHERE id=$1`, taskID).Scan(&t.ID, &t.TaskType, &t.TargetType, &t.TargetID, &payload, &t.Status, &t.RetryCount, &t.LastError, &t.RunAfter, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Payload = string(payload)
	return &t, nil
}

func (s *Store) ClaimRunnableTasks(ctx context.Context, limit int) ([]model.Task, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id, task_type, target_type, target_id, payload, status, retry_count, COALESCE(last_error,''), run_after, created_at, updated_at FROM sync_tasks WHERE status IN ('QUEUED','RETRYING') AND run_after <= NOW() ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Task{}
	for rows.Next() {
		var t model.Task
		var payload []byte
		if err := rows.Scan(&t.ID, &t.TaskType, &t.TargetType, &t.TargetID, &payload, &t.Status, &t.RetryCount, &t.LastError, &t.RunAfter, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Payload = string(payload)
		items = append(items, t)
	}
	for _, t := range items {
		if _, err := tx.Exec(ctx, `UPDATE sync_tasks SET status='RUNNING', executed_at=NOW(), updated_at=NOW() WHERE id=$1`, t.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) MarkTaskSuccess(ctx context.Context, taskID int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE sync_tasks SET status='SUCCESS', finished_at=NOW(), updated_at=NOW() WHERE id=$1`, taskID)
	return err
}

func (s *Store) MarkTaskFailure(ctx context.Context, taskID int64, retryCount int, lastErr string) error {
	status := "RETRYING"
	if retryCount >= 3 {
		status = "FAILED"
	}
	_, err := s.DB.Exec(ctx, `UPDATE sync_tasks SET status=$1, retry_count=$2, last_error=$3, run_after=NOW() + interval '1 minute', updated_at=NOW() WHERE id=$4`, status, retryCount, lastErr, taskID)
	return err
}

func (s *Store) UpsertDocMapping(ctx context.Context, knowledgeID int64, categoryID *int64, targetDocID, targetBlockID, parentBlockID, externalKey, anchorKey, docLink, anchorLink, syncStatus string, version int) error {
	_, err := s.DB.Exec(ctx, `
INSERT INTO doc_sync_mappings (knowledge_id, category_id, target_doc_id, target_block_id, parent_block_id, external_key, anchor_key, doc_link, anchor_link, last_sync_version, sync_status, last_synced_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
ON CONFLICT (knowledge_id)
DO UPDATE SET category_id=EXCLUDED.category_id, target_doc_id=EXCLUDED.target_doc_id, target_block_id=EXCLUDED.target_block_id, parent_block_id=EXCLUDED.parent_block_id, external_key=EXCLUDED.external_key, anchor_key=EXCLUDED.anchor_key, doc_link=EXCLUDED.doc_link, anchor_link=EXCLUDED.anchor_link, last_sync_version=EXCLUDED.last_sync_version, sync_status=EXCLUDED.sync_status, last_synced_at=NOW(), updated_at=NOW()
`, knowledgeID, categoryID, targetDocID, targetBlockID, parentBlockID, externalKey, anchorKey, docLink, anchorLink, version, syncStatus)
	return err
}

func (s *Store) GetKnowledgeByIDAnyState(ctx context.Context, knowledgeID int64) (*model.KnowledgeItem, error) {
	var item model.KnowledgeItem
	err := s.DB.QueryRow(ctx, `SELECT id, user_id, draft_id, title, COALESCE(summary,''), content_markdown, tags, primary_category_id, category_path, COALESCE(confidence,0), status, current_version, auto_classified, COALESCE(auto_classify_confidence,0), COALESCE(doc_link,''), COALESCE(doc_anchor_link,''), removed_at, purge_at, created_at, updated_at FROM knowledge_items WHERE id=$1`, knowledgeID).Scan(
		&item.ID, &item.UserID, &item.DraftID, &item.Title, &item.Summary, &item.ContentMarkdown, &item.Tags, &item.PrimaryCategoryID, &item.CategoryPath, &item.Confidence, &item.Status, &item.CurrentVersion, &item.AutoClassified, &item.AutoClassifyConfidence, &item.DocLink, &item.DocAnchorLink, &item.RemovedAt, &item.PurgeAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) BackfillFromDoc(ctx context.Context, userID, knowledgeID int64, newContent, reason string) (*model.KnowledgeItem, error) {
	item, err := s.GetKnowledge(ctx, userID, knowledgeID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(newContent) == "" || strings.TrimSpace(newContent) == strings.TrimSpace(item.ContentMarkdown) {
		return nil, fmt.Errorf("DOC_BACKFILL_NO_DIFF")
	}
	nextVersion := item.CurrentVersion + 1
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE knowledge_items SET content_markdown=$1, current_version=$2, search_vector = setweight(to_tsvector('simple', COALESCE(title, '')), 'A') || setweight(to_tsvector('simple', COALESCE(summary, '')), 'B') || setweight(to_tsvector('simple', COALESCE($1::text, '')), 'C') || setweight(to_tsvector('simple', array_to_string(COALESCE(tags, '{}'::text[]), ' ')), 'B'), updated_at=NOW() WHERE id=$3 AND user_id=$4`, newContent, nextVersion, knowledgeID, userID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_versions (knowledge_id, version_no, source, title, summary, content_markdown, tags, category_path, editor_user_id, change_reason) VALUES ($1,$2,'DOC_SYNC_BACKFILL',$3,$4,$5,$6,$7,$8,$9)`, knowledgeID, nextVersion, item.Title, item.Summary, newContent, item.Tags, item.CategoryPath, userID, reason)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sync_tasks (task_type, target_type, target_id, payload) VALUES ('DOC_SYNC_KNOWLEDGE','knowledge',$1,$2)`, knowledgeID, `{"reason":"doc_backfill"}`)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, userID, knowledgeID)
}

func (s *Store) PurgeExpiredKnowledge(ctx context.Context) (int64, error) {
	cmd, err := s.DB.Exec(ctx, `DELETE FROM knowledge_items WHERE status='REMOVED_SOFT' AND purge_at IS NOT NULL AND purge_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func SyntheticAnchor(knowledgeID int64, version int) string {
	return fmt.Sprintf("k-%d-v-%d-%s", knowledgeID, version, uuid.NewString()[:8])
}
