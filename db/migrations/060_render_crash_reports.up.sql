-- 060_render_crash_reports.up.sql
-- One row per render crash a player's browser reports: a svelte:boundary
-- caught a throw while drawing the page (frontend/src/lib/boundaryError.ts).
--
-- Why it lives in the database: these throws are rare, invisible to everyone
-- but the player who hit one, and always bugs. The server stores them so the
-- owner can read them (SELECT, by hand); nothing in the game reads this table.
--
-- Both FKs are ON DELETE SET NULL, as in feedback_submissions (046): a report
-- is a record of a crash, not a child row of its game or account, so it should
-- detach rather than vanish when dev/delete-game or account deletion runs.
--
-- The rate limit (handler/render_crash.go) counts this table per account, so a
-- crash can never spend a player's feedback budget, which counts a different
-- table.

CREATE TABLE render_crash_reports (
  id          BIGSERIAL   PRIMARY KEY,
  account_id  BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
  game_id     BIGINT      REFERENCES games(id) ON DELETE SET NULL, -- NULL off a table page
  boundary    TEXT        NOT NULL, -- which boundary caught it, as the client labels it
  message     TEXT        NOT NULL,
  stack       TEXT,                 -- the Error's stack, when the throw was an Error
  route       TEXT        NOT NULL, -- the page's pathname when it crashed
  build       TEXT        NOT NULL, -- the SvelteKit build id the tab was running
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX render_crash_reports_account_recent
  ON render_crash_reports (account_id, created_at);
