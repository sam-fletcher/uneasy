-- Ensure every player has a main-character peer asset. Previously these
-- were created when the facilitator started the Prologue; now they're
-- created on join. Backfills any pre-existing player missing one.
INSERT INTO assets (game_id, owner_id, creator_id, asset_type, name, is_main_character)
SELECT p.game_id, p.id, p.id, 'peer', p.display_name, TRUE
FROM players p
WHERE NOT EXISTS (
  SELECT 1 FROM assets a
  WHERE a.owner_id = p.id AND a.is_main_character = TRUE
);
