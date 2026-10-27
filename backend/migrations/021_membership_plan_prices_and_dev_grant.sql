-- Retail prices: $4.99 / $13.99 / $25.99 / $49.99
UPDATE membership_plans SET price_usd = 4.99  WHERE slug = '1m';
UPDATE membership_plans SET price_usd = 13.99 WHERE slug = '3m';
UPDATE membership_plans SET price_usd = 25.99 WHERE slug = '6m';
UPDATE membership_plans SET price_usd = 49.99 WHERE slug = '1y';

-- Seed toy / owner account as a paid 1-year (49.99) member when present.
UPDATE users
SET
  is_member = 1,
  membership_expires_at = DATE_ADD(UTC_TIMESTAMP(), INTERVAL 365 DAY)
WHERE email = 'devcraft210@gmail.com';
