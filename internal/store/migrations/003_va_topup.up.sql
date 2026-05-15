-- Track VA (Virtual Account) top-up requests and their lifecycle.
-- Each row represents a VA number generated for a top-up request.
-- Status transitions: active (VA generated, awaiting payment) → paid (webhook received) → expired (TTL passed).
CREATE TABLE IF NOT EXISTS va_topup (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id),
    va_number TEXT NOT NULL,
    amount_sen BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    tx_log_id UUID REFERENCES tx_log(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_va_topup_va_number ON va_topup(va_number);
CREATE INDEX IF NOT EXISTS idx_va_topup_idempotency_key ON va_topup(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_va_topup_user_id ON va_topup(user_id);
CREATE INDEX IF NOT EXISTS idx_va_topup_status ON va_topup(status);
