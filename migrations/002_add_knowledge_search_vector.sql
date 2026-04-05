ALTER TABLE knowledge_items
ADD COLUMN IF NOT EXISTS search_vector tsvector;

UPDATE knowledge_items
SET search_vector =
  setweight(to_tsvector('simple', COALESCE(title, '')), 'A') ||
  setweight(to_tsvector('simple', COALESCE(summary, '')), 'B') ||
  setweight(to_tsvector('simple', COALESCE(content_markdown, '')), 'C') ||
  setweight(to_tsvector('simple', array_to_string(COALESCE(tags, '{}'::text[]), ' ')), 'B')
WHERE search_vector IS NULL;

CREATE INDEX IF NOT EXISTS idx_items_search_vector ON knowledge_items USING GIN (search_vector);
