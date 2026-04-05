CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  open_id VARCHAR(64) UNIQUE NOT NULL,
  name VARCHAR(128),
  role VARCHAR(32) NOT NULL DEFAULT 'user',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE categories (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  name VARCHAR(128) NOT NULL,
  parent_id BIGINT REFERENCES categories(id),
  level INT NOT NULL CHECK (level >= 1 AND level <= 5),
  path TEXT NOT NULL,
  path_key TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  source VARCHAR(32) NOT NULL DEFAULT 'system',
  status VARCHAR(32) NOT NULL DEFAULT 'enabled',
  doc_node_key TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, path_key)
);

CREATE TABLE knowledge_drafts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  input_type VARCHAR(32) NOT NULL,
  input_text TEXT NOT NULL,
  title VARCHAR(256),
  summary TEXT,
  content_markdown TEXT,
  tags TEXT[] NOT NULL DEFAULT '{}',
  recommended_category_path TEXT,
  recommendation_confidence NUMERIC(5,4),
  auto_accepted_category BOOLEAN NOT NULL DEFAULT FALSE,
  status VARCHAR(32) NOT NULL DEFAULT 'PENDING_REVIEW',
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE knowledge_items (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  draft_id BIGINT REFERENCES knowledge_drafts(id),
  title VARCHAR(256) NOT NULL,
  summary TEXT,
  content_markdown TEXT NOT NULL,
  tags TEXT[] NOT NULL DEFAULT '{}',
  primary_category_id BIGINT REFERENCES categories(id),
  category_path TEXT NOT NULL,
  confidence NUMERIC(5,4),
  status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
  current_version INT NOT NULL DEFAULT 1,
  auto_classified BOOLEAN NOT NULL DEFAULT FALSE,
  auto_classify_confidence NUMERIC(5,4),
  doc_link TEXT,
  doc_anchor_link TEXT,
  removed_at TIMESTAMPTZ,
  purge_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE knowledge_versions (
  id BIGSERIAL PRIMARY KEY,
  knowledge_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
  version_no INT NOT NULL,
  source VARCHAR(32) NOT NULL,
  title VARCHAR(256) NOT NULL,
  summary TEXT,
  content_markdown TEXT NOT NULL,
  tags TEXT[] NOT NULL DEFAULT '{}',
  category_path TEXT NOT NULL,
  editor_user_id BIGINT REFERENCES users(id),
  change_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(knowledge_id, version_no)
);

CREATE TABLE ai_category_recommendations (
  id BIGSERIAL PRIMARY KEY,
  draft_id BIGINT NOT NULL REFERENCES knowledge_drafts(id) ON DELETE CASCADE,
  rank_no INT NOT NULL,
  recommended_category_id BIGINT REFERENCES categories(id),
  recommended_path TEXT NOT NULL,
  confidence NUMERIC(5,4) NOT NULL,
  reason TEXT,
  accepted BOOLEAN,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE doc_sync_mappings (
  id BIGSERIAL PRIMARY KEY,
  knowledge_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
  category_id BIGINT REFERENCES categories(id),
  parent_doc_id VARCHAR(128),
  target_doc_id VARCHAR(128) NOT NULL,
  anchor_key VARCHAR(256) NOT NULL,
  doc_link TEXT NOT NULL,
  anchor_link TEXT NOT NULL,
  last_sync_version INT NOT NULL DEFAULT 1,
  last_sync_hash VARCHAR(64),
  sync_status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(knowledge_id)
);

CREATE TABLE sync_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_type VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(32) NOT NULL DEFAULT 'QUEUED',
  retry_count INT NOT NULL DEFAULT 0,
  run_after TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error TEXT,
  executed_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE operation_logs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  actor_type VARCHAR(32) NOT NULL,
  action_type VARCHAR(64) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT,
  detail JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_user_parent ON categories(user_id, parent_id, sort_order);
CREATE INDEX idx_categories_user_path_key ON categories(user_id, path_key);
CREATE INDEX idx_drafts_user_status ON knowledge_drafts(user_id, status, created_at DESC);
CREATE INDEX idx_items_user_status ON knowledge_items(user_id, status, updated_at DESC);
CREATE INDEX idx_items_user_category ON knowledge_items(user_id, primary_category_id, updated_at DESC);
CREATE INDEX idx_items_purge_at ON knowledge_items(purge_at) WHERE purge_at IS NOT NULL;
CREATE INDEX idx_versions_knowledge_version ON knowledge_versions(knowledge_id, version_no DESC);
CREATE INDEX idx_ai_reco_draft_rank ON ai_category_recommendations(draft_id, rank_no);
CREATE INDEX idx_sync_tasks_status_run_after ON sync_tasks(status, run_after);
CREATE INDEX idx_logs_user_action_time ON operation_logs(user_id, action_type, created_at DESC);
