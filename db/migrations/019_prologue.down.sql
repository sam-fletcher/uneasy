-- 019_prologue.down.sql

ALTER TABLE games DROP COLUMN IF EXISTS prologue_ranking_step;

DROP TABLE IF EXISTS prologue_heart_declarations;
DROP TABLE IF EXISTS player_cards;
DROP TABLE IF EXISTS prologue_choices;

DROP INDEX IF EXISTS assets_linked_card_per_game;

ALTER TABLE assets
  DROP COLUMN IF EXISTS linked_card_value,
  DROP COLUMN IF EXISTS linked_card_suit;
