-- Rollback: drop budgets table and category column.
DROP TABLE IF EXISTS budgets;
ALTER TABLE tx_log DROP COLUMN IF EXISTS category;
