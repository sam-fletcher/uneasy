-- 049_shake_up_spend_abandon.down.sql
DROP INDEX shake_up_spends_open_idx;
CREATE INDEX shake_up_spends_open_idx ON shake_up_spends (game_id) WHERE committed_at IS NULL;

ALTER TABLE shake_up_spends
  DROP COLUMN abandoned_at;
