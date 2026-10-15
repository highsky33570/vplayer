-- Optional OLEHDTV leaf-type membership for child-category browse (Phase 3).
-- Does not change videos.category_id (canonical top-level ownership).
-- No backfill.

ALTER TABLE videos
  ADD COLUMN leaf_source_id VARCHAR(64) NULL AFTER source_category_id;

CREATE INDEX idx_videos_leaf_source ON videos (leaf_source_id);
