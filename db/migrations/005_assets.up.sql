-- Phase 2: Assets, marginalia, secrets, and secret visibility.

CREATE TABLE assets (
  id                BIGSERIAL PRIMARY KEY,
  game_id           BIGINT      NOT NULL REFERENCES games(id),
  owner_id          BIGINT      NOT NULL REFERENCES players(id),
  creator_id        BIGINT      NOT NULL REFERENCES players(id),
  asset_type        TEXT        NOT NULL CHECK (asset_type IN ('peer','holding','artifact','resource')),
  name              TEXT        NOT NULL,
  is_main_character BOOLEAN     NOT NULL DEFAULT FALSE,
  is_leveraged      BOOLEAN     NOT NULL DEFAULT FALSE,
  is_destroyed      BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  destroyed_at      TIMESTAMPTZ
);

CREATE TABLE marginalia (
  id          BIGSERIAL PRIMARY KEY,
  asset_id    BIGINT      NOT NULL REFERENCES assets(id),
  position    SMALLINT    NOT NULL CHECK (position BETWEEN 1 AND 4),
  text        TEXT        NOT NULL,
  is_torn     BOOLEAN     NOT NULL DEFAULT FALSE,
  torn_at     TIMESTAMPTZ,
  torn_by_id  BIGINT      REFERENCES players(id),
  UNIQUE (asset_id, position)
);

CREATE TABLE secrets (
  id          BIGSERIAL PRIMARY KEY,
  asset_id    BIGINT      NOT NULL REFERENCES assets(id),
  author_id   BIGINT      NOT NULL REFERENCES players(id),
  text        TEXT        NOT NULL,
  is_revealed BOOLEAN     NOT NULL DEFAULT FALSE,
  revealed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE secret_visibility (
  secret_id   BIGINT      NOT NULL REFERENCES secrets(id),
  player_id   BIGINT      NOT NULL REFERENCES players(id),
  seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (secret_id, player_id)
);
