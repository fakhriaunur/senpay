-- Track withdraw requests and their lifecycle.
-- Each row represents a withdraw request sent to the bank.
-- Status transitions: pending (funds reserved) → committed (bank confirmed) / failed (bank rejected) / timeout (no response).
CREATE TABLE IF NOT EXISTS withdraw_records (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id),
    bank_account TEXT NOT NULL,
    amount_sen BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    committed_at TIMESTAMPTZ,
    reversed_at TIMESTAMPTZ,
    tx_log_id UUID REFERENCES tx_log(id)
);

CREATE INDEX IF NOT EXISTS idx_withdraw_records_idempotency_key ON withdraw_records(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_withdraw_records_user_id ON withdraw_records(user_id);
CREATE INDEX IF NOT EXISTS idx_withdraw_records_status ON withdraw_records(status);
