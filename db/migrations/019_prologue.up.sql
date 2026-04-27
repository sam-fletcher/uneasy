-- 019_prologue.up.sql
-- Phase 4b: structured prologue.
--
-- Adds the two prologue-specific tables (prologue_choices, player_cards),
-- the linked_card_* columns on assets that make a card-linked asset takeable
-- by a later prologue claim, and the games.prologue_ranking_step state
-- machine column that drives the track-by-track ranking sub-flow.

ALTER TABLE assets
  ADD COLUMN linked_card_suit  CHAR(1) CHECK (linked_card_suit IN ('C','D','S','H')),
  ADD COLUMN linked_card_value TEXT
    CHECK (linked_card_value IN ('A','2','3','4','5','6','7','8','9','10','J','Q','K'));

-- A given card can be linked to at most one asset at a time. (Suit+value is
-- unique per game; a card may be re-linked to a different game's asset.)
CREATE UNIQUE INDEX assets_linked_card_per_game
  ON assets (game_id, linked_card_suit, linked_card_value)
  WHERE linked_card_suit IS NOT NULL;

CREATE TABLE prologue_choices (
  id           BIGSERIAL PRIMARY KEY,
  game_id      BIGINT      NOT NULL REFERENCES games(id),
  player_id    BIGINT      NOT NULL REFERENCES players(id),
  turn_number  SMALLINT    NOT NULL CHECK (turn_number BETWEEN 1 AND 3),
  sheet_type   TEXT        NOT NULL CHECK (sheet_type IN ('titles','hailing_from','laws_rumors')),
  choice_name  TEXT        NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, sheet_type, choice_name),
  UNIQUE (game_id, player_id, turn_number)
);

CREATE INDEX prologue_choices_player_idx
  ON prologue_choices (game_id, player_id);

-- Cards held by each player at prologue end. Updated as cards change hands
-- via make/take during the prologue's box claims; frozen once the ranking
-- sub-flow begins.
CREATE TABLE player_cards (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT  NOT NULL REFERENCES games(id),
  player_id   BIGINT  NOT NULL REFERENCES players(id),
  card_suit   CHAR(1) NOT NULL CHECK (card_suit IN ('C','D','S','H')),
  card_value  TEXT    NOT NULL CHECK (card_value IN ('A','2','3','4','5','6','7','8','9','10','J','Q','K')),
  UNIQUE (game_id, card_suit, card_value)
);

CREATE INDEX player_cards_owner_idx
  ON player_cards (game_id, player_id);

-- Hearts declared as another suit during a track's ranking step. Each row
-- consumes one heart card from the player's hand for the duration of the
-- ranking flow. The (player, track) pair is unique because each track
-- collects all of a player's heart declarations into a single row.
CREATE TABLE prologue_heart_declarations (
  id         BIGSERIAL PRIMARY KEY,
  game_id    BIGINT      NOT NULL REFERENCES games(id),
  player_id  BIGINT      NOT NULL REFERENCES players(id),
  track      TEXT        NOT NULL CHECK (track IN ('power','knowledge','esteem')),
  count      SMALLINT    NOT NULL CHECK (count >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, player_id, track)
);

-- Drives the prologue ranking state machine. NULL while still in the choice
-- phase (and during main_event onward); non-NULL only during the ranking
-- sub-flow.
ALTER TABLE games
  ADD COLUMN prologue_ranking_step TEXT
    CHECK (prologue_ranking_step IN (
      'declare_power','place_set_asides_power',
      'declare_knowledge','place_set_asides_knowledge',
      'declare_esteem','place_set_asides_esteem',
      'extra_peers'
    ));
