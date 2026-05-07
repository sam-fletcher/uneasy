-- 025_prologue_extra_peers.up.sql
-- Track per-player extra-peer claims during the extra_peers stage of
-- the prologue (≤3-player games). Each player picks exactly one
-- unused title; both constraints are enforced here.

CREATE TABLE prologue_extra_peers (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  player_id   BIGINT      NOT NULL REFERENCES players(id),
  title_name  TEXT        NOT NULL,
  asset_id    BIGINT      NOT NULL REFERENCES assets(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, player_id),
  UNIQUE (game_id, title_name)
);
