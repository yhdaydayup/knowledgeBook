ALTER TABLE doc_sync_mappings
ADD COLUMN IF NOT EXISTS target_block_id VARCHAR(128),
ADD COLUMN IF NOT EXISTS parent_block_id VARCHAR(128),
ADD COLUMN IF NOT EXISTS external_key VARCHAR(256);

CREATE UNIQUE INDEX IF NOT EXISTS idx_doc_sync_mappings_external_key
ON doc_sync_mappings(external_key)
WHERE external_key IS NOT NULL;
