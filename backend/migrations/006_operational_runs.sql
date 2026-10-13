CREATE TABLE IF NOT EXISTS operational_runs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  job_type VARCHAR(32) NOT NULL,
  trigger_type VARCHAR(32) NOT NULL,
  status VARCHAR(16) NOT NULL,
  requested_by_user_id BIGINT UNSIGNED NULL,
  requested_at DATETIME NOT NULL,
  started_at DATETIME NULL,
  finished_at DATETIME NULL,

  scanned INT NULL,
  created_count INT NULL,
  updated_count INT NULL,
  unchanged_count INT NULL,
  failed_count INT NULL,

  selected_count INT NULL,
  succeeded_count INT NULL,
  skipped_count INT NULL,
  unavailable_count INT NULL,

  error_code VARCHAR(64) NULL,
  has_error TINYINT(1) NOT NULL DEFAULT 0,

  sync_run_id BIGINT UNSIGNED NULL,

  KEY idx_op_runs_job_started (job_type, started_at, id),
  KEY idx_op_runs_status_started (status, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
