-- 047_password_reset_tokens.up.sql
-- adr/FEEDBACK_AND_RESET_PLAN.md Session 2: single-use, expiring tokens for
-- operator-driven password resets (cmd/resetlink, POST /api/password-resets).
--
-- token_hash is SHA-256 of a 32-random-byte base64url raw token; only the
-- hash is ever persisted (a DB leak must not yield live reset links), so the
-- PK doubles as the lookup key and there is no need for a separate id.
CREATE TABLE password_reset_tokens (
  token_hash  TEXT        PRIMARY KEY,
  account_id  BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_account ON password_reset_tokens(account_id);
