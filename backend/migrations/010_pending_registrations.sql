-- Pending email-verification registrations (pre-users row).
-- Existing users/admins are unchanged and are not required to verify retroactively.
CREATE TABLE IF NOT EXISTS pending_registrations (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  email VARCHAR(191) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  nickname VARCHAR(64) NOT NULL DEFAULT '',
  code_hash VARCHAR(64) NOT NULL,
  expires_at DATETIME NOT NULL,
  attempt_count INT NOT NULL DEFAULT 0,
  resend_count INT NOT NULL DEFAULT 0,
  last_sent_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_pending_registrations_email (email),
  KEY idx_pending_registrations_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
