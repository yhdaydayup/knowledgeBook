ALTER TABLE knowledge_drafts
  ADD COLUMN IF NOT EXISTS raw_content TEXT,
  ADD COLUMN IF NOT EXISTS normalized_title VARCHAR(256),
  ADD COLUMN IF NOT EXISTS normalized_summary TEXT,
  ADD COLUMN IF NOT EXISTS normalized_points JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS llm_confidence NUMERIC(5,4);

CREATE TABLE IF NOT EXISTS knowledge_similarity (
  id BIGSERIAL PRIMARY KEY,
  draft_id BIGINT NOT NULL REFERENCES knowledge_drafts(id) ON DELETE CASCADE,
  knowledge_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
  similarity_score NUMERIC(5,4) NOT NULL,
  relation_type VARCHAR(64) NOT NULL,
  reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(draft_id, knowledge_id)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_similarity_draft_score ON knowledge_similarity(draft_id, similarity_score DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_similarity_knowledge ON knowledge_similarity(knowledge_id);