-- Admin-controlled 热门推荐 badge on catalog cards.
-- Additive only. Defaults keep existing rows without the badge.

ALTER TABLE videos
  ADD COLUMN is_hot TINYINT(1) NOT NULL DEFAULT 0 AFTER membership_required;

CREATE INDEX idx_videos_is_hot ON videos (is_hot, status, is_active);
