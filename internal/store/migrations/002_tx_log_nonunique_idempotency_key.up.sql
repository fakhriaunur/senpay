-- Remove unique constraint on tx_log.idempotency_key to allow
-- multiple entries (debit + credit + fee) per transfer with the same key.
-- Idempotency is enforced at the application level (Redis + SERIALIZABLE tx),
-- not at the schema level for tx_log entries.
DROP INDEX IF EXISTS idx_tx_log_idempotency_key;
CREATE INDEX IF NOT EXISTS idx_tx_log_idempotency_key ON tx_log(idempotency_key);
