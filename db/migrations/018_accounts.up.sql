-- Migration 018: Accounts and sessions.
--
-- Replaces user_tokens-as-identity with named accounts that can be logged
-- into from multiple devices and joined to multiple games. The cookie
-- token becomes a session pointer rather than the identity itself.
--
-- DEV-ONLY MIGRATION: drops user_tokens and players.cookie_token without
-- a backfill path. Run against a wiped dev DB.

CREATE TABLE accounts (
  id           BIGSERIAL   PRIMARY KEY,
  username     TEXT        NOT NULL,
  code_hash    TEXT        NOT NULL,
  email        TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX accounts_username_lower ON accounts (LOWER(username));

CREATE TABLE sessions (
  token       TEXT        PRIMARY KEY,
  account_id  BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_account ON sessions(account_id);

-- players.cookie_token → players.account_id.
-- Drop all rows first since the FK target is going away.
DELETE FROM players;

ALTER TABLE players
  DROP COLUMN cookie_token,
  ADD COLUMN account_id BIGINT NOT NULL REFERENCES accounts(id);

CREATE UNIQUE INDEX players_account_game ON players(account_id, game_id);

DROP TABLE user_tokens;
