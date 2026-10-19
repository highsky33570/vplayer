-- Per-video checkpoint for the operator source-comment backfill.
-- operational_runs records one job invocation and has no video identity,
-- so resume state lives here instead. The API applies this file on startup.
-- Nothing in the scheduler reads or writes this table.
-- Text columns use utf8mb4_general_ci so they match videos.source_system
-- and videos.source_id. CHARSET=utf8mb4 alone inherits MySQL 8's
-- utf8mb4_0900_ai_ci default and cannot be joined to those columns.
CREATE TABLE IF NOT EXISTS source_comment_backfill (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_system VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  mode VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  source_video_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  status VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  pages_fetched INT UNSIGNED NOT NULL DEFAULT 0,
  comments_discovered INT UNSIGNED NOT NULL DEFAULT 0,
  parents INT UNSIGNED NOT NULL DEFAULT 0,
  replies INT UNSIGNED NOT NULL DEFAULT 0,
  inserted_count INT UNSIGNED NOT NULL DEFAULT 0,
  updated_count INT UNSIGNED NOT NULL DEFAULT 0,
  skipped_count INT UNSIGNED NOT NULL DEFAULT 0,
  error_message VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL,
  started_at DATETIME NULL,
  completed_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_source_comment_backfill (source_system, video_id, mode),
  KEY idx_source_comment_backfill_status (source_system, mode, status, video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
