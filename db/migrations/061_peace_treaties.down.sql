-- Treaties read as nothing again; the proposals themselves stay. Markers that
-- were moved past them stay where they are, which costs nothing.
DROP INDEX IF EXISTS war_peace_proposals_accepted_idx;
ALTER TABLE war_peace_proposals DROP CONSTRAINT IF EXISTS war_peace_proposals_accepted_has_rev;
ALTER TABLE war_peace_proposals DROP COLUMN IF EXISTS rev;
