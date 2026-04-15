-- Migration 011: Index for SP esteem lockout queries.
-- Needed to efficiently find a player's most recent plan of a given type,
-- used when checking whether an esteem lockout from Spread Propaganda mar
-- option (b) is still in effect.

CREATE INDEX idx_plans_preparer_type
    ON plans(game_id, preparer_id, plan_type, prepared_at_row DESC);
