-- 038_shake_up_claim_title.up.sql
-- Shake-Up "Claim a new title" (ADR-007 Phase B, Shake-Up half) now stamps a
-- real title marginalia instead of creating a generic "New Title" artifact. The
-- spend must carry which title id the player chose and the freeform flavor text
-- for the marginalia, so the commit-time effect can route through the shared
-- CreateTitleMarginalia + EstablishThrone path (the same one the Prologue uses).
ALTER TABLE shake_up_spends
  ADD COLUMN target_title_id TEXT,
  ADD COLUMN title_flavor    TEXT;
