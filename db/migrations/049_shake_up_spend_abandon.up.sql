-- 049_shake_up_spend_abandon.up.sql
-- ADR-008: Shake-Up spend commitment ("pay or abandon"). A resolved spend can
-- now close in a terminal "abandoned" state (extra cost unaffordable, or
-- voluntarily declined) instead of always committing. abandoned_at marks
-- that outcome, distinct from committed_at.
--
-- "Open spend" was committed_at IS NULL; an abandoned spend must not read as
-- open (it would wedge the auction — no one could ever announce again), so
-- the partial index and the open-spend query both gain the extra guard.
ALTER TABLE shake_up_spends
  ADD COLUMN abandoned_at TIMESTAMPTZ;

DROP INDEX shake_up_spends_open_idx;
CREATE INDEX shake_up_spends_open_idx ON shake_up_spends (game_id)
  WHERE committed_at IS NULL AND abandoned_at IS NULL;
