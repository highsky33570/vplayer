-- Already-applied migration 013 created source_comment_backfill without an
-- explicit collation, so MySQL 8 stored its text columns as
-- utf8mb4_0900_ai_ci. videos.source_system and videos.source_id are
-- utf8mb4_general_ci. Drain selection compares those columns directly:
--   source_comment_backfill.source_system = videos.source_system
-- and MySQL rejects the mix with error 1267.
--
-- CONVERT TO rewrites the existing character data into utf8mb4_general_ci
-- and rebuilds uq_source_comment_backfill and idx_source_comment_backfill_status.
-- Checkpoint rows, counters, timestamps, and video_id values stay.
-- videos is not altered.
ALTER TABLE source_comment_backfill
  CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
