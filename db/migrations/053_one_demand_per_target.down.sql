-- Revert to migration 017's predicate, which let a resolved demand free its
-- target's slot for a second demand.

DROP INDEX IF EXISTS uq_one_demand_per_target;

CREATE UNIQUE INDEX uq_one_demand_per_target
  ON plans (targeted_plan_id)
  WHERE targeted_plan_id IS NOT NULL
    AND status NOT IN ('resolved', 'cancelled');
