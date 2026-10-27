-- Paid 大会员: time-bounded membership + EPUSDT crypto orders.
-- is_member remains for admin lifetime grants (expires_at NULL) and paid sync flag.

ALTER TABLE users
  ADD COLUMN membership_expires_at DATETIME NULL DEFAULT NULL AFTER is_member;

CREATE TABLE IF NOT EXISTS membership_plans (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  slug VARCHAR(32) NOT NULL,
  name VARCHAR(64) NOT NULL,
  price_usd DECIMAL(10,2) NOT NULL,
  duration_days INT NOT NULL,
  sort INT NOT NULL DEFAULT 0,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  UNIQUE KEY uk_membership_plans_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO membership_plans (slug, name, price_usd, duration_days, sort, is_active) VALUES
  ('1m',  '1个月', 4.99,  30,  10, 1),
  ('3m',  '3个月', 13.99, 90,  20, 1),
  ('6m',  '6个月', 25.99, 180, 30, 1),
  ('1y',  '1年',   49.99, 365, 40, 1)
ON DUPLICATE KEY UPDATE
  name=VALUES(name),
  price_usd=VALUES(price_usd),
  duration_days=VALUES(duration_days),
  sort=VALUES(sort),
  is_active=VALUES(is_active);

CREATE TABLE IF NOT EXISTS membership_orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  order_no VARCHAR(32) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  plan_slug VARCHAR(32) NOT NULL,
  plan_name VARCHAR(64) NOT NULL,
  duration_days INT NOT NULL,
  amount_usd DECIMAL(10,2) NOT NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'usd',
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  gateway_provider VARCHAR(32) NOT NULL DEFAULT 'epusdt',
  gateway_trade_id VARCHAR(64) NULL,
  gateway_payment_url VARCHAR(512) NULL,
  pay_address VARCHAR(128) NULL,
  amount_usdt DECIMAL(18,8) NULL,
  gateway_token VARCHAR(32) NULL,
  gateway_network VARCHAR(32) NULL,
  gateway_status VARCHAR(32) NULL,
  block_txid VARCHAR(128) NULL,
  expire_at DATETIME NOT NULL,
  paid_at DATETIME NULL,
  membership_granted_until DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_membership_orders_order_no (order_no),
  KEY idx_membership_orders_user (user_id, created_at),
  KEY idx_membership_orders_status_expire (status, expire_at),
  KEY idx_membership_orders_trade (gateway_trade_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
