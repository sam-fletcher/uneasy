-- 043_plan_scenes.down.sql
ALTER TABLE scenes DROP CONSTRAINT IF EXISTS scenes_plan_id_by_kind;
ALTER TABLE scenes DROP CONSTRAINT IF EXISTS scenes_location_by_kind;
ALTER TABLE scenes ALTER COLUMN time_elapsed SET NOT NULL;
ALTER TABLE scenes ADD CONSTRAINT scenes_check
  CHECK ((location_holding_id IS NULL) <> (location_custom IS NULL));
ALTER TABLE scenes DROP COLUMN IF EXISTS plan_id;
ALTER TABLE scenes DROP COLUMN IF EXISTS kind;
