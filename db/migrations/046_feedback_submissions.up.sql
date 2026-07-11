-- 046_feedback_submissions.up.sql
-- adr/FEEDBACK_AND_RESET_PLAN.md Session 1: a single table backs both the
-- login-gated feedback form and the logged-out "locked out?" reset-request
-- intake (kind distinguishes the two). The DB row is the source of truth;
-- a Discord webhook (handler/notify.go) is a best-effort notification only.
--
-- Both FKs are ON DELETE SET NULL, not CASCADE: a submission must survive
-- dev/delete-game and account deletion (see the migration-041 cascade
-- audit) — it's an intake record about a game/account, not a child row
-- scoped to one, so it should detach rather than vanish with its parent.
CREATE TABLE feedback_submissions (
  id          BIGSERIAL   PRIMARY KEY,
  kind        TEXT        NOT NULL CHECK (kind IN ('feedback','reset_request')),
  account_id  BIGINT      REFERENCES accounts(id) ON DELETE SET NULL, -- NULL for reset_request
  username    TEXT,       -- reset_request: as-typed; may not match any account
  game_id     BIGINT      REFERENCES games(id) ON DELETE SET NULL,
  body        TEXT        NOT NULL,
  contact     TEXT,       -- optional (feedback) / required (reset_request)
  context     JSONB,      -- route, phase, user agent
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX feedback_submissions_account_recent
  ON feedback_submissions (account_id, created_at);
