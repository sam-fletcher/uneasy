-- 026_scenes.up.sql
-- Adds explicit structure to Main Event scenes. A scene records where it
-- takes place (a holding asset OR free-text), how much in-fiction time has
-- elapsed since the previous scene, and which peer assets are present.
-- Scenes also carry the follow-on prompt from the plan that was just
-- resolved (or empty for free scenes that don't follow a plan).
--
-- See SCENES_PLAN.md for the full design.

CREATE TABLE scenes (
  id                   BIGSERIAL   PRIMARY KEY,
  game_id              BIGINT      NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  row_number           SMALLINT    NOT NULL,
  focus_player_id      BIGINT      NOT NULL REFERENCES players(id),
  location_holding_id  BIGINT      REFERENCES assets(id),
  location_custom      TEXT,
  time_elapsed         TEXT        NOT NULL CHECK (time_elapsed IN (
                         'moments','hours','days','weeks',
                         'flashback','simultaneous'
                       )),
  time_note            TEXT,
  prompt               TEXT        NOT NULL DEFAULT '',
  resolved_plan_id     BIGINT      REFERENCES plans(id),
  started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at             TIMESTAMPTZ,
  -- Exactly one of the two location fields must be set.
  CHECK ((location_holding_id IS NULL) <> (location_custom IS NULL))
);

CREATE INDEX scenes_game_row ON scenes(game_id, row_number);

-- At most one active (not yet ended) scene per game.
CREATE UNIQUE INDEX scenes_one_active_per_game
  ON scenes(game_id) WHERE ended_at IS NULL;

-- Peers present in a scene. controller_player_id may be NULL for an
-- unclaimed focus-player peer; once claimed, the row is updated and the
-- peer is locked to that player for the remainder of the scene.
CREATE TABLE scene_peers (
  scene_id              BIGINT NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
  peer_asset_id         BIGINT NOT NULL REFERENCES assets(id),
  controller_player_id  BIGINT REFERENCES players(id),
  PRIMARY KEY (scene_id, peer_asset_id)
);

CREATE INDEX scene_peers_scene ON scene_peers(scene_id);
