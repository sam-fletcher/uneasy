-- Migration 017: Make Demands (Phase 3d).
--
-- A Make Demands plan targets another plan. On a made roll, the demander
-- and the target-plan's preparer draft the four demand options; the
-- winners are persisted on the demand's own plan row (demand_option_winners)
-- so the target plan's resolution can consult them cheaply. On a marred
-- roll, the demand's target may prepare a free counter-demand; if no
-- candidate plan yet exists, a pending_counter_demands row is inserted
-- and consumed the next time that player prepares any plan.

ALTER TABLE plans
  ADD COLUMN targeted_plan_id       BIGINT REFERENCES plans(id),
  ADD COLUMN demand_option_winners  JSONB;

-- Rule #5: at most one unresolved demand per target plan.
CREATE UNIQUE INDEX uq_one_demand_per_target
  ON plans (targeted_plan_id)
  WHERE targeted_plan_id IS NOT NULL
    AND status NOT IN ('resolved', 'cancelled');

CREATE TABLE pending_counter_demands (
  id                  BIGSERIAL PRIMARY KEY,
  game_id             BIGINT   NOT NULL REFERENCES games(id),
  demanding_player_id BIGINT   NOT NULL REFERENCES players(id),
  target_player_id    BIGINT   NOT NULL REFERENCES players(id),
  origin_plan_id      BIGINT   NOT NULL REFERENCES plans(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at         TIMESTAMPTZ,
  resolved_plan_id    BIGINT   REFERENCES plans(id)
);

CREATE INDEX idx_pending_counter_demands_open
  ON pending_counter_demands (demanding_player_id)
  WHERE resolved_at IS NULL;
