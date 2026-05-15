CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    phone TEXT NOT NULL,
    pin_hash TEXT NOT NULL,
    kyc_level TEXT NOT NULL DEFAULT 'basic',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone ON users(phone);

CREATE TABLE IF NOT EXISTS tx_log (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    tx_type TEXT NOT NULL,
    sender_id UUID REFERENCES users(id),
    receiver_id UUID REFERENCES users(id),
    amount_sen BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'IDR',
    status TEXT NOT NULL DEFAULT 'pending',
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    committed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tx_log_idempotency_key ON tx_log(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_tx_log_sender_id ON tx_log(sender_id);
CREATE INDEX IF NOT EXISTS idx_tx_log_receiver_id ON tx_log(receiver_id);
CREATE INDEX IF NOT EXISTS idx_tx_log_created_at ON tx_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tx_log_status ON tx_log(status);

CREATE TABLE IF NOT EXISTS balance_snapshot (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    balance_sen BIGINT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
