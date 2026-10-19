-- Ongoing source-comment maintenance. This is not the historical backfill.
-- source_comment_backfill stays a one-time completion record and is not read or updated here.
-- A cycle freezes the highest eligible video id at start. Later catalog inserts wait
-- for the next cycle. Incremental and full repair each have their own cycles.
-- Text columns use utf8mb4_general_ci so they match videos.source_system / source_id.
CREATE TABLE IF NOT EXISTS source_comment_maintenance_cycles (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  source_system VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  mode VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  status VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  max_video_id BIGINT UNSIGNED NOT NULL,
  error_message VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL,
  started_at DATETIME NOT NULL,
  completed_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_source_comment_maintenance_cycles (source_system, mode, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS source_comment_maintenance (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  cycle_id BIGINT UNSIGNED NOT NULL,
  source_system VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  mode VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  source_video_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  status VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  pages_fetched INT UNSIGNED NOT NULL DEFAULT 0,
  comments_discovered INT UNSIGNED NOT NULL DEFAULT 0,
  inserted_count INT UNSIGNED NOT NULL DEFAULT 0,
  updated_count INT UNSIGNED NOT NULL DEFAULT 0,
  skipped_count INT UNSIGNED NOT NULL DEFAULT 0,
  error_message VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL,
  last_started_at DATETIME NULL,
  last_completed_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_source_comment_maintenance (cycle_id, video_id),
  KEY idx_source_comment_maintenance_status (cycle_id, status, video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
