CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(64) NOT NULL PRIMARY KEY,
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE categories
  ADD COLUMN source_system VARCHAR(32) NULL AFTER slug,
  ADD COLUMN source_id VARCHAR(64) NULL AFTER source_system,
  ADD COLUMN last_synced_at DATETIME NULL AFTER created_at,
  ADD COLUMN sync_status VARCHAR(32) NOT NULL DEFAULT 'ok' AFTER last_synced_at,
  ADD COLUMN sync_error TEXT NULL AFTER sync_status,
  ADD COLUMN is_active TINYINT(1) NOT NULL DEFAULT 1 AFTER sync_error;

ALTER TABLE videos
  ADD COLUMN source_system VARCHAR(32) NULL AFTER id,
  ADD COLUMN source_id VARCHAR(64) NULL AFTER source_system,
  ADD COLUMN source_category_id VARCHAR(64) NULL AFTER category_id,
  ADD COLUMN year VARCHAR(16) NOT NULL DEFAULT '' AFTER description,
  ADD COLUMN area VARCHAR(64) NOT NULL DEFAULT '' AFTER year,
  ADD COLUMN director VARCHAR(255) NOT NULL DEFAULT '' AFTER area,
  ADD COLUMN actors TEXT NULL AFTER director,
  ADD COLUMN rating DECIMAL(4,1) NOT NULL DEFAULT 0 AFTER actors,
  ADD COLUMN poster_source_url VARCHAR(1024) NOT NULL DEFAULT '' AFTER cover_key,
  ADD COLUMN cover_r2_key VARCHAR(512) NOT NULL DEFAULT '' AFTER poster_source_url,
  ADD COLUMN playback_source VARCHAR(64) NOT NULL DEFAULT '' AFTER hls_master_key,
  ADD COLUMN play_sid INT NOT NULL DEFAULT 1 AFTER playback_source,
  ADD COLUMN play_nid INT NOT NULL DEFAULT 1 AFTER play_sid,
  ADD COLUMN playback_url VARCHAR(2048) NOT NULL DEFAULT '' AFTER play_nid,
  ADD COLUMN source_updated_at DATETIME NULL AFTER playback_url,
  ADD COLUMN last_synced_at DATETIME NULL AFTER source_updated_at,
  ADD COLUMN sync_status VARCHAR(32) NOT NULL DEFAULT 'ok' AFTER last_synced_at,
  ADD COLUMN sync_error TEXT NULL AFTER sync_status,
  ADD COLUMN is_active TINYINT(1) NOT NULL DEFAULT 1 AFTER sync_error,
  ADD COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP AFTER created_at;

CREATE UNIQUE INDEX uq_categories_source ON categories (source_system, source_id);
CREATE UNIQUE INDEX uq_videos_source ON videos (source_system, source_id);
CREATE INDEX idx_videos_category_active ON videos (category_id, is_active, status);
CREATE INDEX idx_videos_source_updated ON videos (source_system, source_updated_at);

CREATE TABLE IF NOT EXISTS video_episodes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  video_id BIGINT UNSIGNED NOT NULL,
  source_system VARCHAR(32) NOT NULL,
  source_video_id VARCHAR(64) NOT NULL,
  sid INT NOT NULL DEFAULT 1,
  nid INT NOT NULL DEFAULT 1,
  title VARCHAR(255) NOT NULL DEFAULT '',
  playback_source VARCHAR(64) NOT NULL DEFAULT '',
  playback_url VARCHAR(2048) NOT NULL DEFAULT '',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  last_synced_at DATETIME NULL,
  sync_status VARCHAR(32) NOT NULL DEFAULT 'ok',
  sync_error TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_episode_source (source_system, source_video_id, sid, nid),
  KEY idx_episode_video (video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_runs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_system VARCHAR(32) NOT NULL,
  trigger_type VARCHAR(32) NOT NULL DEFAULT 'manual',
  started_at DATETIME NOT NULL,
  finished_at DATETIME NULL,
  scanned INT NOT NULL DEFAULT 0,
  created_count INT NOT NULL DEFAULT 0,
  updated_count INT NOT NULL DEFAULT 0,
  unchanged_count INT NOT NULL DEFAULT 0,
  failed_count INT NOT NULL DEFAULT 0,
  deactivated_count INT NOT NULL DEFAULT 0,
  summary_json JSON NULL,
  error_text TEXT NULL,
  KEY idx_sync_runs_source (source_system, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
