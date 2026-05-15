-- Add CHECK constraints on columns storing enum values.
-- These enforce database-level validation matching the Go newtypes:
--   VAStatus (active, paid, expired)
--   CallbackStatus (success, failed) — not stored in DB
--   BankProvider (stub, snap) — not stored in DB
--   EntryType (debit, credit) — not stored in DB
--
-- Existing data validation: all rows should already satisfy these constraints.
-- If any row violates, the migration will fail — fix data first.

-- va_topup.status: VAStatus values
ALTER TABLE va_topup DROP CONSTRAINT IF EXISTS chk_va_topup_status;
ALTER TABLE va_topup ADD CONSTRAINT chk_va_topup_status
    CHECK (status IN ('active', 'paid', 'expired'));
