-- Track last successful login for admin user management.
ALTER TABLE users
  ADD COLUMN last_login_at DATETIME NULL AFTER created_at;
