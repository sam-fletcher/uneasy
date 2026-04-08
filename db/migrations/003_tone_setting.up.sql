-- Phase 2: Tone-setting topics table.

CREATE TABLE tone_topics (
  id        BIGSERIAL PRIMARY KEY,
  game_id   BIGINT   NOT NULL REFERENCES games(id),
  topic     TEXT     NOT NULL,
  status    TEXT     NOT NULL DEFAULT 'default'
            CHECK (status IN ('default','include','avoid_detail','never')),
  UNIQUE (game_id, topic)
);
