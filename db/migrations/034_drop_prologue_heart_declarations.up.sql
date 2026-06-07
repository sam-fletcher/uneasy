-- Drops the legacy count-based prologue_heart_declarations table. The
-- prologue ranking flow now uses the per-card "committed hearts" model
-- (migration 024: prologue_committed_hearts), so the old declarations
-- table and its DeclareHearts/FinalizeTrackRanking handlers are gone.
DROP TABLE IF EXISTS prologue_heart_declarations;
