-- Dedicated failure timestamp for automatic media-migration retry cooldown.
-- migrated_at remains success-only (set by MarkEpisodeMigrated).

ALTER TABLE video_episodes
  ADD COLUMN migration_failed_at DATETIME NULL AFTER migrated_at;
