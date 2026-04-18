DROP TABLE IF EXISTS war_surrender_claims;
ALTER TABLE war_battle_costs   DROP COLUMN IF EXISTS is_entry;
ALTER TABLE war_participants   DROP COLUMN IF EXISTS entry_payment_complete;
