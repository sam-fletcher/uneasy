-- 054_scene_time_chip_xor_note.down.sql
-- Restores the 043 constraint (every turn-scene carries a time_elapsed).
-- Note-only scenes written under 054 would violate it, so give them back the
-- neutral "moments" — which is exactly the value 054's backfill cleared, since
-- the pre-054 form always sent 'moments' alongside a note. Round-trips for
-- every row the up migration touched. A row that predates the form's
-- mutual-exclusion fix (ee01443, 2026-05-14) and held a note beside a
-- deliberate 'days'/'weeks' chip comes back as 'moments'; no such row exists
-- in any deployed game.
--
-- Constraint off first, as in the up: 054's XOR forbids a chip beside a note,
-- which is exactly the state this backfill has to pass through.
ALTER TABLE scenes DROP CONSTRAINT scenes_location_by_kind;

UPDATE scenes SET time_elapsed = 'moments'
WHERE kind = 'turn' AND time_elapsed IS NULL;

ALTER TABLE scenes ADD CONSTRAINT scenes_location_by_kind CHECK (
  (kind = 'turn' AND (location_holding_id IS NULL) <> (location_custom IS NULL)
    AND time_elapsed IS NOT NULL)
  OR
  (kind = 'plan' AND location_holding_id IS NULL AND location_custom IS NULL
    AND time_elapsed IS NULL)
);
