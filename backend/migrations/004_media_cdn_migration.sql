-- Progressive R2/CDN media migration fields (backward compatible).
-- Existing playback_url / poster_source_url remain until migration succeeds.

ALTER TABLE video_episodes
  ADD COLUMN hls_object_key VARCHAR(512) NOT NULL DEFAULT '' AFTER playback_url,
  ADD COLUMN storage_provider VARCHAR(32) NOT NULL DEFAULT '' AFTER hls_object_key,
  ADD COLUMN migration_status VARCHAR(32) NOT NULL DEFAULT '' AFTER storage_provider,
  ADD COLUMN migration_error TEXT NULL AFTER migration_status,
  ADD COLUMN migrated_at DATETIME NULL AFTER migration_error;

CREATE INDEX idx_episodes_migration_status ON video_episodes (migration_status, id);

ALTER TABLE videos
  ADD COLUMN storage_provider VARCHAR(32) NOT NULL DEFAULT '' AFTER cover_r2_key;
