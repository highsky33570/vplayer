-- Large-catalog indexes + episode playback status / last_seen
ALTER TABLE videos
  ADD COLUMN last_seen_at DATETIME NULL AFTER last_synced_at,
  ADD COLUMN detail_url VARCHAR(1024) NOT NULL DEFAULT '' AFTER playback_url,
  ADD COLUMN status_text VARCHAR(255) NOT NULL DEFAULT '' AFTER detail_url;

ALTER TABLE video_episodes
  ADD COLUMN play_page_url VARCHAR(1024) NOT NULL DEFAULT '' AFTER playback_url,
  ADD COLUMN playback_status VARCHAR(32) NOT NULL DEFAULT 'ok' AFTER play_page_url;

CREATE INDEX idx_videos_status_active_id ON videos (status, is_active, id);
CREATE INDEX idx_videos_cat_status_id ON videos (category_id, status, is_active, id);
CREATE INDEX idx_episodes_video_sid_nid ON video_episodes (video_id, sid, nid);
