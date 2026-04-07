-- Phase 1 schema: identity, tables, and posts.
-- This is a deliberate subset of the full data model (DATA_MODEL.md).
-- Game-mechanic columns are added in later migrations.

-- user_tokens: pre-game identity.
-- Stores a player's display name against their cookie token before (and
-- independent of) any game membership. One row per browser identity.
CREATE TABLE user_tokens (
  token        TEXT        PRIMARY KEY,
  display_name TEXT        NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- games: one row per table (a play session).
-- Phase 1 only needs the join code and facilitator link.
-- The full data model adds: phase, current_row, focus_player_id, etc.
CREATE TABLE games (
  id             BIGSERIAL   PRIMARY KEY,
  join_code      TEXT        NOT NULL UNIQUE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  facilitator_id BIGINT      -- set after the first players row is created
);

-- players: one row per person per game (a "seat at the table").
-- The cookie_token links back to user_tokens.
-- Phase 1 allows one game per person (cookie_token UNIQUE).
-- The full data model adds: token_color, seat_order, shake_up_tokens, etc.
CREATE TABLE players (
  id             BIGSERIAL   PRIMARY KEY,
  game_id        BIGINT      NOT NULL REFERENCES games(id),
  display_name   TEXT        NOT NULL,
  cookie_token   TEXT        NOT NULL UNIQUE REFERENCES user_tokens(token),
  joined_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  is_facilitator BOOLEAN     NOT NULL DEFAULT FALSE
);

-- Add the FK from games.facilitator_id now that players exists.
ALTER TABLE games
  ADD CONSTRAINT fk_games_facilitator
  FOREIGN KEY (facilitator_id) REFERENCES players(id);

-- posts: the play-by-post feed.
-- Phase 1-only table: a flat chronological list of messages in a table.
-- In Phase 2+ this will be replaced by the scene_posts table from DATA_MODEL.md,
-- which is threaded by public record row and plan.
CREATE TABLE posts (
  id         BIGSERIAL   PRIMARY KEY,
  game_id    BIGINT      NOT NULL REFERENCES games(id),
  author_id  BIGINT      NOT NULL REFERENCES players(id),
  body       TEXT        NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for the common query: "give me all posts for this table in order"
CREATE INDEX posts_game_id_created_at ON posts(game_id, created_at);
