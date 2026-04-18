DROP TABLE IF EXISTS pending_counter_demands;
DROP INDEX IF EXISTS uq_one_demand_per_target;
ALTER TABLE plans
  DROP COLUMN IF EXISTS demand_option_winners,
  DROP COLUMN IF EXISTS targeted_plan_id;
