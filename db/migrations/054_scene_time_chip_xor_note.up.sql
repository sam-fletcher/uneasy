-- 054_scene_time_chip_xor_note.up.sql
-- A turn-scene's When is one of the six chips OR a free-text note, never both
-- and never neither — the rule the setup form has always enforced (selecting a
-- chip clears the note and vice versa) but the schema never expressed. 043
-- required time_elapsed NOT NULL, so a note-only scene could not be stored at
-- all, and the form papered over that by sending a `?? 'moments'` fallback
-- alongside the note. Those scenes have been claiming "Moments later" in the
-- scene-details panel ever since.
--
-- The backfill is exact rather than a guess: the form has cleared the chip on
-- note input since ee01443 (2026-05-14), months before the first public game,
-- so a turn-scene holding BOTH values can only be the fallback at work. The
-- note is what the player actually wrote, and it is already what every display
-- path prefers (sceneTimeLabel), so clearing the chip changes no rendered text
-- — it just stops the panel appending a duration nobody chose.
--
-- Order matters: 043's CHECK still forbids a null time_elapsed, so the
-- constraint has to come off BEFORE the backfill and the new one go on after.
-- Backfilling first passes on an empty table and fails on every real one.
ALTER TABLE scenes DROP CONSTRAINT scenes_location_by_kind;

UPDATE scenes SET time_elapsed = NULL
WHERE kind = 'turn' AND time_note IS NOT NULL;

-- Mirrors the location XOR in the same clause: Where is a holding XOR free
-- text, When is a chip XOR free text.
ALTER TABLE scenes ADD CONSTRAINT scenes_location_by_kind CHECK (
  (kind = 'turn' AND (location_holding_id IS NULL) <> (location_custom IS NULL)
    AND (time_elapsed IS NULL) <> (time_note IS NULL))
  OR
  (kind = 'plan' AND location_holding_id IS NULL AND location_custom IS NULL
    AND time_elapsed IS NULL)
);
