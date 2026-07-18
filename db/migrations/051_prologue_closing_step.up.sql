-- 051_prologue_closing_step.up.sql
-- adr/PROLOGUE_CLOSING_STAGE_PLAN.md Session 1: games.prologue_ranking_step's
-- CHECK constraint (019_prologue.up.sql) enumerated 'extra_peers' as the
-- step ≤3-player games landed on after ranking. That step is retired — ALL
-- player counts now converge on 'closing' instead — so the constraint swaps
-- 'extra_peers' for 'closing'.
--
-- Drop the old constraint first — the forward-migration UPDATE below writes
-- 'closing', which the old constraint (no 'closing' in its list) would
-- itself reject if it ran first.
ALTER TABLE games DROP CONSTRAINT games_prologue_ranking_step_check;

-- Forward-migrate any in-flight game currently parked at 'extra_peers'.
-- 'extra_peers' was ≤3p-only, so every such row is a ≤3p game that still
-- owes its players' extra peers — exactly what the closing step's ≤3p
-- checklist item now also requires, so this is a like-for-like remap, not a
-- skipped step.
UPDATE games SET prologue_ranking_step = 'closing' WHERE prologue_ranking_step = 'extra_peers';

ALTER TABLE games
  ADD CONSTRAINT games_prologue_ranking_step_check
    CHECK (prologue_ranking_step IN (
      'declare_power','place_set_asides_power',
      'declare_knowledge','place_set_asides_knowledge',
      'declare_esteem','place_set_asides_esteem',
      'closing'
    ));
