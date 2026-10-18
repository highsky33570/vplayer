-- Native VPlayer comments. No foreign keys, matching earlier migrations.
-- created_at / updated_at are UTC wall times written by the application.
CREATE TABLE IF NOT EXISTS video_comments (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  video_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  body VARCHAR(1000) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'visible',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_video_comments_video_created (video_id, created_at, id),
  KEY idx_video_comments_user (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
