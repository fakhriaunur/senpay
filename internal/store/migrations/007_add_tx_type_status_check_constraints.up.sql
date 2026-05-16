-- Add CHECK constraints on tx_type and tx_status columns in tx_log.
-- These enforce database-level validation matching the Go newtypes:
--   TxType (topup, transfer, withdraw, fee)
--   TxStatus (pending, committed, failed, compensated, timeout)
--
-- Existing data validation: all rows should already satisfy these constraints.
-- If any row violates, the migration will fail -- fix data first.

-- tx_log.tx_type: TxType values
ALTER TABLE tx_log DROP CONSTRAINT IF EXISTS chk_tx_log_tx_type;
ALTER TABLE tx_log ADD CONSTRAINT chk_tx_log_tx_type
    CHECK (tx_type IN ('topup', 'transfer', 'withdraw', 'fee'));

-- tx_log.status: TxStatus values
ALTER TABLE tx_log DROP CONSTRAINT IF EXISTS chk_tx_log_status;
ALTER TABLE tx_log ADD CONSTRAINT chk_tx_log_status
    CHECK (status IN ('pending', 'committed', 'failed', 'compensated', 'timeout'));

-- withdraw_records.status: TxStatus values
ALTER TABLE withdraw_records DROP CONSTRAINT IF EXISTS chk_withdraw_records_status;
ALTER TABLE withdraw_records ADD CONSTRAINT chk_withdraw_records_status
    CHECK (status IN ('pending', 'committed', 'failed', 'timeout'));
