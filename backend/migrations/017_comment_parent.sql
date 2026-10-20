-- Native VPlayer comment replies (parent_id). No FOREIGN KEYs.
ALTER TABLE video_comments
  ADD COLUMN parent_id BIGINT UNSIGNED NULL AFTER user_id,
  ADD KEY idx_video_comments_parent (parent_id);
