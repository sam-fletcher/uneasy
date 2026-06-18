-- Migration 035: at most one open interactive dice roll per game.
--
-- The whole app is built around a single in-flight dice roll at a time: the
-- table page tracks one activeRoll, and GetActiveRollForGame returns "the
-- latest still-open roll". This index enforces that invariant at the source of
-- truth so two players racing to start a roll can't both succeed — the second
-- INSERT fails and the handler maps it to a 409 (retry later), rather than
-- silently producing two open rolls that clobber each other in the UI.
--
-- Scope: only interactive rolls. Shake-up rolls (is_shake_up = TRUE) are an
-- instant, per-player audit mechanic during a different game phase, never an
-- interactive active roll, and several can be unresolved at once — so they are
-- excluded. "Open" means not yet resolved (resolved_at IS NULL).
CREATE UNIQUE INDEX uq_one_open_roll_per_game
  ON dice_rolls (game_id)
  WHERE resolved_at IS NULL AND is_shake_up = FALSE;
