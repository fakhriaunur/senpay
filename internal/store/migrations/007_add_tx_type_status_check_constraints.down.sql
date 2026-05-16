-- Drop CHECK constraints added in migration 007.
ALTER TABLE tx_log DROP CONSTRAINT IF EXISTS chk_tx_log_tx_type;
ALTER TABLE tx_log DROP CONSTRAINT IF EXISTS chk_tx_log_status;
ALTER TABLE withdraw_records DROP CONSTRAINT IF EXISTS chk_withdraw_records_status;
