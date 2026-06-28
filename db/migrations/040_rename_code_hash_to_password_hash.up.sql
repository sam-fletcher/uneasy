-- Migration 040: rename accounts.code_hash to accounts.password_hash.
--
-- "code" was an outdated term for what is, and always was, a password. The
-- auth flow uses no emailed codes. Rename the column so the schema matches
-- the user-facing language ("Password") and the rest of the code.
ALTER TABLE accounts RENAME COLUMN code_hash TO password_hash;
