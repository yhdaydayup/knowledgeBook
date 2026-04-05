ALTER TABLE knowledge_drafts
  ADD COLUMN IF NOT EXISTS chat_id TEXT,
  ADD COLUMN IF NOT EXISTS source_message_id TEXT,
  ADD COLUMN IF NOT EXISTS reply_to_message_id TEXT,
  ADD COLUMN IF NOT EXISTS card_message_id TEXT,
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_reminded_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS interaction_context JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE knowledge_drafts
SET status = 'PENDING_CONFIRMATION'
WHERE status = 'PENDING_REVIEW';

UPDATE knowledge_drafts
SET status = 'REJECTED',
    resolved_at = COALESCE(resolved_at, reviewed_at, updated_at, created_at)
WHERE status = 'IGNORED';

UPDATE knowledge_drafts
SET expires_at = COALESCE(expires_at, created_at + interval '1 hour')
WHERE status = 'PENDING_CONFIRMATION';

CREATE INDEX IF NOT EXISTS idx_drafts_user_chat_status_created ON knowledge_drafts(user_id, chat_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_drafts_status_expires_at ON knowledge_drafts(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_drafts_source_message_id ON knowledge_drafts(source_message_id);
CREATE INDEX IF NOT EXISTS idx_drafts_reply_to_message_id ON knowledge_drafts(reply_to_message_id);
CREATE INDEX IF NOT EXISTS idx_drafts_card_message_id ON knowledge_drafts(card_message_id);