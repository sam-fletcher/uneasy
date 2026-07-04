-- 043_plan_scenes.up.sql
-- Chat Overhaul Phase 5 (adr/CHAT_OVERHAUL_PLAN.md). Roleplay-heavy plan
-- resolutions (Host Festivity, Propose Decree, Chronicle Histories,
-- Clandestinely Liaise) open a Scene of their own so validateSpeakingAs
-- (handler/scenes.go) stops blocking in-character speech during them. A
-- plan-scene skips the turn-scene's location/time setup step entirely — it
-- opens at plan.resolving and closes at plan.resolved/cancelled.
ALTER TABLE scenes ADD COLUMN kind TEXT NOT NULL DEFAULT 'turn'
  CHECK (kind IN ('turn', 'plan'));
ALTER TABLE scenes ADD COLUMN plan_id BIGINT REFERENCES plans(id) ON DELETE CASCADE;

-- Turn-scenes still require exactly one location and a time_elapsed value
-- (the setup form); plan-scenes require neither. Replace the old
-- location-only CHECK with a kind-conditional one, and drop the blanket
-- time_elapsed NOT NULL so a plan-scene can leave it unset.
ALTER TABLE scenes DROP CONSTRAINT scenes_check;
ALTER TABLE scenes ALTER COLUMN time_elapsed DROP NOT NULL;
ALTER TABLE scenes ADD CONSTRAINT scenes_location_by_kind CHECK (
  (kind = 'turn' AND (location_holding_id IS NULL) <> (location_custom IS NULL)
    AND time_elapsed IS NOT NULL)
  OR
  (kind = 'plan' AND location_holding_id IS NULL AND location_custom IS NULL
    AND time_elapsed IS NULL)
);
ALTER TABLE scenes ADD CONSTRAINT scenes_plan_id_by_kind CHECK (
  (kind = 'plan') = (plan_id IS NOT NULL)
);
