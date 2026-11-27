-- 061_peace_treaties.up.sql
-- Agreed peace terms are kept (owner's ruling, 2026-11-26): they show on the Laws
-- tab as a treaty, in a group of their own, and the war's closing line quotes
-- them. A treaty is not a new kind of row. It is an accepted peace proposal
-- (war_peace_proposals.status = 'accepted'), with the players who agreed read
-- from war_peace_votes, so there is nothing to copy and nothing to drift.
--
-- What a treaty needs that a proposal lacks is a `rev`, so the Laws tab's "new"
-- dot (migration 058) can light for it the way it does for a law: the dot
-- compares an entry's rev with the viewer's laws_seen_rev, and both are drawn
-- from the one sequence, laws_rumors_rev_seq. The rev is drawn when the
-- proposal is accepted and only then; an open or rejected proposal has none.
-- The CHECK below makes "accepted without a rev" impossible, so a future path
-- that accepts a proposal and forgets to draw one fails loudly instead of
-- producing a treaty that never shows as new.
--
-- One-way door only for the CHECK and the column: the down migration drops
-- both, and treaties then read as nothing (the proposals stay).

ALTER TABLE war_peace_proposals ADD COLUMN rev BIGINT;

-- Proposals accepted before this migration become treaties too. They are
-- numbered in the order they were accepted, from just past everything the
-- sequence has issued, and the sequence is moved to the last of them so later
-- laws and edits stay above every treaty.
DO $$
DECLARE
  base  BIGINT;
  total BIGINT;
BEGIN
  SELECT CASE WHEN is_called THEN last_value ELSE last_value - 1 END
    INTO base FROM laws_rumors_rev_seq;
  SELECT count(*) INTO total FROM war_peace_proposals WHERE status = 'accepted';

  UPDATE war_peace_proposals p SET rev = base + n.rn
  FROM (
    SELECT id, row_number() OVER (ORDER BY resolved_at, id) AS rn
    FROM war_peace_proposals WHERE status = 'accepted'
  ) n
  WHERE p.id = n.id;

  IF total > 0 THEN
    PERFORM setval('laws_rumors_rev_seq', base + total, true);
  END IF;
END $$;

ALTER TABLE war_peace_proposals
  ADD CONSTRAINT war_peace_proposals_accepted_has_rev
  CHECK (status <> 'accepted' OR rev IS NOT NULL);

-- Nobody should see a dot on deploy for a war that ended weeks ago: a player who
-- was caught up on laws (their marker at or past the newest law) is moved past
-- the game's treaties too. Someone with an unseen law keeps their marker, so
-- that law still shows; the old treaties then show as new to them, which is
-- true. (Treaty revs are above every law rev, so moving a marker onto them would
-- otherwise mark an unseen law seen.)
UPDATE player_laws_rumors_seen s
SET laws_seen_rev = t.top
FROM players p
JOIN (
  SELECT w.game_id, max(pp.rev) AS top
  FROM war_peace_proposals pp JOIN wars w ON w.id = pp.war_id
  WHERE pp.status = 'accepted'
  GROUP BY w.game_id
) t ON t.game_id = p.game_id
WHERE s.player_id = p.id
  AND s.laws_seen_rev >= COALESCE((SELECT max(l.rev) FROM laws l WHERE l.game_id = p.game_id), 0)
  AND s.laws_seen_rev < t.top;

-- The Laws list reads a game's treaties through the war.
CREATE INDEX war_peace_proposals_accepted_idx
  ON war_peace_proposals (war_id) WHERE status = 'accepted';
