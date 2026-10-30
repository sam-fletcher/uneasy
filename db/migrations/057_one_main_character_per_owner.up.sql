-- A player holds exactly one live main character per game. Every write path
-- already keeps this (TransferAsset clears the flag, promotion clears the old
-- MC first), so this index is the backstop: a path that breaks it fails loudly
-- instead of leaving GetMainCharacterByOwner to pick an arbitrary row.
--
-- is_destroyed is part of the predicate because DestroyAsset leaves the flag
-- set on the destroyed row, and the conscript flow then creates a new MC.

-- Repair first so the index can never fail to build at startup: keep the
-- oldest live flagged asset per owner. No-op when the invariant holds.
UPDATE assets a SET is_main_character = FALSE
WHERE a.is_main_character AND NOT a.is_destroyed
  AND EXISTS (
    SELECT 1 FROM assets b
    WHERE b.game_id = a.game_id AND b.owner_id = a.owner_id
      AND b.is_main_character AND NOT b.is_destroyed AND b.id < a.id
  );

CREATE UNIQUE INDEX assets_one_main_character_idx
  ON assets (game_id, owner_id)
  WHERE is_main_character AND NOT is_destroyed;
