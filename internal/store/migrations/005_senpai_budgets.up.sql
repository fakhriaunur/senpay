-- Add category column to tx_log for spending categorization.
-- Categories are assigned during transfer: Makanan, Transportasi, Belanja, Hiburan, Kesehatan, Pendidikan, Tagihan, Lainnya.
ALTER TABLE tx_log ADD COLUMN IF NOT EXISTS category TEXT;

-- Create budgets table for monthly spending budgets per category.
CREATE TABLE IF NOT EXISTS budgets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    category TEXT NOT NULL,
    limit_sen BIGINT NOT NULL CHECK (limit_sen > 0),
    spent_sen BIGINT NOT NULL DEFAULT 0,
    month INTEGER NOT NULL,
    year INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, category, month, year)
);

CREATE INDEX IF NOT EXISTS idx_budgets_user_id ON budgets(user_id);
CREATE INDEX IF NOT EXISTS idx_budgets_user_month ON budgets(user_id, year, month);
CREATE INDEX IF NOT EXISTS idx_tx_log_category ON tx_log(category);
