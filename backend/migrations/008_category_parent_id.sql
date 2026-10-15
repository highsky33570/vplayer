-- Nullable parent for hierarchical categories (Phase 1).
-- No FOREIGN KEY: matches existing migration style (indexes only).

ALTER TABLE categories
  ADD COLUMN parent_id BIGINT UNSIGNED NULL AFTER id;

CREATE INDEX idx_categories_parent ON categories (parent_id);
