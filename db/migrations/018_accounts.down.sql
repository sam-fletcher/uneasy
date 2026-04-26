-- Reverse migration 018. Destroys all account/session/player data.

DELETE FROM players;

ALTER TABLE players
  DROP COLUMN account_id,
  ADD COLUMN cookie_token TEXT NOT NULL;

CREATE TABLE user_tokens (
  token        TEXT        PRIMARY KEY,
  display_name TEXT        NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE players
  ADD CONSTRAINT players_cookie_token_key UNIQUE (cookie_token),
  ADD CONSTRAINT players_cookie_token_fkey
    FOREIGN KEY (cookie_token) REFERENCES user_tokens(token);

DROP TABLE sessions;
DROP TABLE accounts;
