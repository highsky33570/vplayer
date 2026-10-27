-- Admin-controlled membership gate: videos marked membership_required are
-- visible only to users with is_member=1 (admins always count as members).
-- Additive only. Defaults keep existing rows public.

ALTER TABLE videos
  ADD COLUMN membership_required TINYINT(1) NOT NULL DEFAULT 0 AFTER status;

CREATE INDEX idx_videos_membership_required ON videos (membership_required, status, is_active);

ALTER TABLE users
  ADD COLUMN is_member TINYINT(1) NOT NULL DEFAULT 0 AFTER is_admin;
