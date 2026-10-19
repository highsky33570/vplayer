-- Imported per-video comments from a source site. No foreign keys, no user_id.
-- fetched_at / created_at / updated_at are UTC wall times written by the application.
-- source_created_raw is the source page text unchanged. It has no verified timezone.
CREATE TABLE IF NOT EXISTS video_source_comments (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  video_id BIGINT UNSIGNED NOT NULL,
  source_system VARCHAR(32) NOT NULL,
  source_video_id VARCHAR(64) NOT NULL,
  source_comment_id VARCHAR(32) NOT NULL,
  parent_source_comment_id VARCHAR(32) NULL,
  author_name VARCHAR(64) NOT NULL,
  avatar_path VARCHAR(512) NULL,
  body TEXT NOT NULL,
  source_created_raw VARCHAR(64) NULL,
  digg_up INT UNSIGNED NOT NULL DEFAULT 0,
  digg_down INT UNSIGNED NOT NULL DEFAULT 0,
  status VARCHAR(16) NOT NULL DEFAULT 'visible',
  fetched_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_video_source_comments (source_system, source_comment_id),
  KEY idx_video_source_comments_video (video_id, status),
  KEY idx_video_source_comments_source_video (source_system, source_video_id),
  KEY idx_video_source_comments_parent (parent_source_comment_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
