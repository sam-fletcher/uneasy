-- 038_shake_up_claim_title.down.sql
ALTER TABLE shake_up_spends
  DROP COLUMN IF EXISTS target_title_id,
  DROP COLUMN IF EXISTS title_flavor;
