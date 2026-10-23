-- Explicit OLEHDTV membership classification. Additive only: no deletes, no index.
-- Appended as the last column with a constant default so MySQL 8.0 can apply
-- ALGORITHM=INSTANT (metadata only, existing rows read as '').
-- playback_status stays "unavailable" so watch playback keeps the existing
-- unavailable response. restriction_reason records why, and stays empty for
-- pending, parser, and playable rows. Historical unavailable rows are not
-- guessed here, they stay '' until an explicit membership classification.
ALTER TABLE video_episodes
  ADD COLUMN restriction_reason VARCHAR(32) NOT NULL DEFAULT '',
  ALGORITHM=INSTANT;
