-- Reverse: any NULL row_number rows must be backfilled before re-applying
-- NOT NULL. Plans awaiting a delay reveal would block this; rolling back
-- requires either closing those reveals first or accepting the data loss.
UPDATE plans SET row_number = 1 WHERE row_number IS NULL;
ALTER TABLE plans ALTER COLUMN row_number SET NOT NULL;
