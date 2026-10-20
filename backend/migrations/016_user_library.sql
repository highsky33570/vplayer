-- User library: avatar hue, favorites (+ default folders), watch history, notifications.
-- No FOREIGN KEYs (project convention).

ALTER TABLE users
  ADD COLUMN avatar_hue SMALLINT NOT NULL DEFAULT -1 AFTER nickname;

CREATE TABLE IF NOT EXISTS user_favorite_folders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(64) NOT NULL,
  is_default TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  KEY idx_fav_folders_user (user_id, id),
  KEY idx_fav_folders_user_default (user_id, is_default)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_favorites (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  folder_id BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL,
  UNIQUE KEY uq_user_favorites_user_video (user_id, video_id),
  KEY idx_user_favorites_user_created (user_id, created_at, id),
  KEY idx_user_favorites_folder_created (folder_id, created_at, id),
  KEY idx_user_favorites_video (video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_watch_history (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  video_id BIGINT UNSIGNED NOT NULL,
  last_watched_at DATETIME NOT NULL,
  progress_sec INT NOT NULL DEFAULT 0,
  UNIQUE KEY uq_user_history_user_video (user_id, video_id),
  KEY idx_user_history_user_watched (user_id, last_watched_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_notifications (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  kind VARCHAR(32) NOT NULL,
  title VARCHAR(191) NOT NULL,
  body VARCHAR(1000) NOT NULL DEFAULT '',
  video_id BIGINT UNSIGNED NULL,
  comment_id BIGINT UNSIGNED NULL,
  actor_user_id BIGINT UNSIGNED NULL,
  is_read TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  KEY idx_user_notif_user_created (user_id, created_at, id),
  KEY idx_user_notif_user_unread (user_id, is_read, created_at),
  KEY idx_user_notif_user_kind_created (user_id, kind, created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
