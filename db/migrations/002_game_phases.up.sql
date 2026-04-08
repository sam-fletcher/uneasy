-- Phase 2: Add game phase tracking and player extensions.
--
-- games gains: phase, current_row, focus_player_id, ending_mode, dummy_token_mode.
-- players gains: token_color, seat_order.

ALTER TABLE games ADD COLUMN phase TEXT NOT NULL DEFAULT 'lobby'
  CHECK (phase IN (
    'lobby',
    'tone_setting',
    'prologue',
    'main_event',
    'shake_up',
    'ended'
  ));

ALTER TABLE games ADD COLUMN current_row SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE games ADD COLUMN focus_player_id BIGINT REFERENCES players(id);

ALTER TABLE games ADD COLUMN ending_mode TEXT
  CHECK (ending_mode IN ('smooth_landing','long_campaign','explosive_finale'));

ALTER TABLE games ADD COLUMN dummy_token_mode TEXT NOT NULL DEFAULT 'static'
  CHECK (dummy_token_mode IN ('static','dynamic'));

-- Player extensions for game mechanics.
ALTER TABLE players ADD COLUMN token_color TEXT;
ALTER TABLE players ADD COLUMN seat_order SMALLINT;
