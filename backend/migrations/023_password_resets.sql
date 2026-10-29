-- Email verification codes for forgot-password / reset password.
-- Additive only. Does not alter users.password_hash until a successful reset.

CREATE TABLE IF NOT EXISTS password_resets (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  email VARCHAR(191) NOT NULL,
  code_hash VARCHAR(64) NOT NULL,
  expires_at DATETIME NOT NULL,
  attempt_count INT NOT NULL DEFAULT 0,
  resend_count INT NOT NULL DEFAULT 0,
  last_sent_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_password_resets_email (email),
  KEY idx_password_resets_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
