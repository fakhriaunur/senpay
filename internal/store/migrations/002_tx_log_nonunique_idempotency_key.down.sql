-- Restore unique constraint on tx_log.idempotency_key.
DROP INDEX IF EXISTS idx_tx_log_idempotency_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tx_log_idempotency_key ON tx_log(idempotency_key);
