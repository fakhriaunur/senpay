-- Drop CHECK constraints added in migration 006.
ALTER TABLE va_topup DROP CONSTRAINT IF EXISTS chk_va_topup_status;
