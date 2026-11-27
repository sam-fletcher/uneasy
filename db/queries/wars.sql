-- sqlc queries for Make War (Phase 3d).

-- ── Wars ──────────────────────────────────────────────────────────────

-- name: CreateWar :one
INSERT INTO wars (game_id, origin_plan_id, started_at_row)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWarByOriginPlan :one
SELECT * FROM wars WHERE origin_plan_id = $1;

-- name: ListActiveWarsByGame :many
SELECT * FROM wars WHERE game_id = $1 AND status = 'active'
ORDER BY id;

-- name: ListActiveOrClaimedWarsByGame :many
-- What GET /wars lists: every active war, plus an ended war that still owes
-- its winners a surrender seizure (the claims keep the row blocked, see
-- ListOpenSurrenderClaimsByGame, so clients need the war to render the claim
-- form).
--
-- One statement on purpose. This used to be two queries (active, then ended
-- with open claims) appended together; a war that ended between them was in
-- both, the client held it twice, and a keyed {#each} over its plans threw.
-- Postgres gives one statement one snapshot, so a war is in this list once.
SELECT * FROM wars w
WHERE w.game_id = $1
  AND (
    w.status = 'active'
    OR (
      w.status = 'ended'
      AND EXISTS (
        SELECT 1 FROM war_surrender_claims c
        WHERE c.war_id = w.id AND c.fulfilled_at IS NULL
      )
    )
  )
ORDER BY w.id;

-- name: EndWar :exec
UPDATE wars
SET status = 'ended', ended_at_row = $2, end_reason = $3
WHERE id = $1;

-- ── War participants ──────────────────────────────────────────────────

-- name: AddWarParticipant :exec
INSERT INTO war_participants (war_id, player_id, side, joined_at_row)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: AddWarParticipantPending :exec
-- Late joiners to an already-active war start with entry_payment_complete=FALSE;
-- they must pay one break/leverage per existing opposing opponent before being
-- counted as a full participant.
INSERT INTO war_participants (
  war_id, player_id, side, joined_at_row, entry_payment_complete
)
VALUES ($1, $2, $3, $4, FALSE)
ON CONFLICT DO NOTHING;

-- name: SetWarParticipantEntryComplete :exec
UPDATE war_participants
SET entry_payment_complete = TRUE
WHERE war_id = $1 AND player_id = $2;

-- name: GetWarParticipant :one
SELECT * FROM war_participants
WHERE war_id = $1 AND player_id = $2;

-- name: ListWarParticipants :many
SELECT * FROM war_participants
WHERE war_id = $1
ORDER BY side, player_id;

-- name: ListActiveWarParticipants :many
-- Active participants: joined, entry-paid, and not surrendered.
SELECT * FROM war_participants
WHERE war_id = $1
  AND surrendered_at_row IS NULL
  AND entry_payment_complete = TRUE
ORDER BY side, player_id;

-- name: SetWarParticipantSurrendered :exec
UPDATE war_participants
SET surrendered_at_row = $3
WHERE war_id = $1 AND player_id = $2;

-- ── Battle costs ──────────────────────────────────────────────────────

-- name: CreateBattleCost :one
INSERT INTO war_battle_costs (
  war_id, row_number, payer_id, opponent_id, choice,
  asset_id_1, asset_id_2, surrendered, is_entry
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListBattleCostsForRow :many
SELECT * FROM war_battle_costs
WHERE war_id = $1 AND row_number = $2
ORDER BY created_at;

-- name: ListBattleCostsByPayerForRow :many
SELECT * FROM war_battle_costs
WHERE war_id = $1 AND row_number = $2 AND payer_id = $3
ORDER BY created_at;

-- ── Peace proposals ───────────────────────────────────────────────────

-- name: CreatePeaceProposal :one
INSERT INTO war_peace_proposals (war_id, proposer_id, terms)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOpenPeaceProposal :one
SELECT * FROM war_peace_proposals
WHERE war_id = $1 AND status = 'open'
ORDER BY id DESC LIMIT 1;

-- name: GetPeaceProposal :one
SELECT * FROM war_peace_proposals WHERE id = $1;

-- name: SetPeaceProposalStatus :exec
-- For a rejection. Accepting goes through AcceptPeaceProposal, which also draws
-- the treaty's rev; the table's CHECK refuses an 'accepted' row without one.
UPDATE war_peace_proposals
SET status = $2, resolved_at = now()
WHERE id = $1;

-- name: AcceptPeaceProposal :one
-- The proposal becomes a treaty (migration 061). It draws a rev from the laws
-- and rumors sequence, so the Laws tab's "new" dot lights for everyone whose
-- marker is behind it, exactly as for a new law. Only an open proposal can be
-- accepted: a second call gets no row.
UPDATE war_peace_proposals
SET status = 'accepted', resolved_at = now(), rev = nextval('laws_rumors_rev_seq')
WHERE id = $1 AND status = 'open'
RETURNING *;

-- name: ListTreaties :many
-- A game's agreed peace terms, oldest agreement first, with the players who
-- agreed. Public to the whole table, like the laws they sit under: the terms
-- were posted to the log when proposed. agreed_by is the players whose accepts
-- closed the vote, the proposer's included (proposing is an acceptance): the
-- accepting votes of the war's ACTIVE participants, the same set the vote's
-- tally counts (ListActiveWarParticipants). A player who accepted and then
-- surrendered before the vote closed was no longer a party to the peace.
SELECT p.id, p.war_id, w.origin_plan_id, p.terms, COALESCE(p.rev, 0)::BIGINT AS rev, p.resolved_at,
       COALESCE(array_agg(v.player_id ORDER BY v.player_id)
                FILTER (WHERE v.accepted AND wp.player_id IS NOT NULL), '{}')::BIGINT[] AS agreed_by
FROM war_peace_proposals p
JOIN wars w ON w.id = p.war_id
LEFT JOIN war_peace_votes v ON v.proposal_id = p.id
LEFT JOIN war_participants wp
  ON wp.war_id = p.war_id AND wp.player_id = v.player_id
 AND wp.surrendered_at_row IS NULL AND wp.entry_payment_complete = TRUE
WHERE w.game_id = $1 AND p.status = 'accepted'
GROUP BY p.id, w.origin_plan_id
ORDER BY p.rev ASC;

-- name: UpsertPeaceVote :exec
INSERT INTO war_peace_votes (proposal_id, player_id, accepted)
VALUES ($1, $2, $3)
ON CONFLICT (proposal_id, player_id) DO UPDATE
SET accepted = EXCLUDED.accepted, created_at = now();

-- name: ListPeaceVotes :many
SELECT * FROM war_peace_votes
WHERE proposal_id = $1
ORDER BY player_id;

-- ── Surrender claims ──────────────────────────────────────────────────

-- name: CreateSurrenderClaim :exec
-- Idempotent: if this (surrendered_id, claimant_id) pair already has an open
-- claim, do nothing (e.g. a player tried to surrender twice).
INSERT INTO war_surrender_claims (war_id, surrendered_id, claimant_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: GetSurrenderClaim :one
SELECT * FROM war_surrender_claims
WHERE war_id = $1 AND surrendered_id = $2 AND claimant_id = $3;

-- name: FulfillSurrenderClaim :exec
UPDATE war_surrender_claims
SET asset_id = $2, fulfilled_at = now()
WHERE id = $1;

-- name: ListOpenSurrenderClaimsByWar :many
SELECT * FROM war_surrender_claims
WHERE war_id = $1 AND fulfilled_at IS NULL
ORDER BY id;

-- name: ListOpenSurrenderClaimsByGame :many
SELECT c.* FROM war_surrender_claims c
JOIN wars w ON w.id = c.war_id
WHERE w.game_id = $1 AND c.fulfilled_at IS NULL
ORDER BY c.id;
