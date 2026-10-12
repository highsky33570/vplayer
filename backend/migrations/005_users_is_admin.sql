-- Admin authorization foundation: users.is_admin (default non-admin).
ALTER TABLE users
  ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE AFTER nickname;
