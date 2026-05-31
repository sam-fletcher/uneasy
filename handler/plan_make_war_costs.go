package handler

// handler/plan_make_war_costs.go — Shared cost-of-battle queries used by the
// row-advance gate (turn.go) and the /pay-battle-cost route. Splitting this
// out of plan_make_war.go keeps the core plan handler readable.

import (
	"context"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// mwPowerRanks returns a player_id → power rank map for the game. Empty rank
// slots (PlayerID == nil) are skipped. Used for reverse-power ordering.
func mwPowerRanks(ctx context.Context, q *dbgen.Queries, gameID int64) (map[int64]int16, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	out := map[int64]int16{}
	for _, r := range rankings {
		if r.Category != model.CategoryPower || r.PlayerID == nil {
			continue
		}
		out[*r.PlayerID] = r.Rank
	}
	return out, nil
}

// mwWarSnapshot captures what the cost-of-battle calculation needs for one war.
//
// Sides/Surrendered/Active only include fully-joined participants
// (entry_payment_complete=TRUE). Pending late joiners are tracked separately
// in PendingSides so the cost-of-battle ordering and peace vote don't see
// them until they have completed their entry payments.
type mwWarSnapshot struct {
	War          dbgen.War
	Active       []int64         // non-surrendered fully-joined participants
	Sides        map[int64]int16 // fully-joined participants → side (incl. surrendered)
	Surrendered  map[int64]bool
	PendingSides map[int64]int16 // late joiners who owe entry payments
}

func mwSnapshotWar(ctx context.Context, q *dbgen.Queries, war dbgen.War) (mwWarSnapshot, error) {
	parts, err := q.ListWarParticipants(ctx, war.ID)
	if err != nil {
		return mwWarSnapshot{}, err
	}
	snap := mwWarSnapshot{
		War:          war,
		Sides:        map[int64]int16{},
		Surrendered:  map[int64]bool{},
		PendingSides: map[int64]int16{},
	}
	for _, p := range parts {
		if !p.EntryPaymentComplete {
			snap.PendingSides[p.PlayerID] = p.Side
			continue
		}
		snap.Sides[p.PlayerID] = p.Side
		if p.SurrenderedAtRow != nil {
			snap.Surrendered[p.PlayerID] = true
		} else {
			snap.Active = append(snap.Active, p.PlayerID)
		}
	}
	return snap, nil
}

// mwOutstandingCostsForWar returns the unpaid (payer, opponent) pairs for this
// war on the given row, in reverse-power + ascending-opponent order.
//
// Returns no costs while the war hasn't actually begun: a Make War plan sits
// in the public record until its delay reveal closes (plan.row_number stays
// NULL), and the rules state cost of battle starts the row *after* the
// declaration scene resolves — i.e. only when row > plan.row_number.
func mwOutstandingCostsForWar(
	ctx context.Context,
	q *dbgen.Queries,
	snap mwWarSnapshot,
	ranks map[int64]int16,
	row int16,
) ([]gamepkg.BattleCostKey, error) {
	plan, err := q.GetPlanByID(ctx, snap.War.OriginPlanID)
	if err != nil {
		return nil, err
	}
	if plan.RowNumber == nil || row <= *plan.RowNumber {
		return nil, nil
	}
	paid := map[gamepkg.BattleCostKey]bool{}
	for _, p := range snap.Active {
		rows, err := q.ListBattleCostsByPayerForRow(ctx, dbgen.ListBattleCostsByPayerForRowParams{
			WarID:     snap.War.ID,
			RowNumber: row,
			PayerID:   p,
		})
		if err != nil {
			return nil, err
		}
		for _, bc := range rows {
			paid[gamepkg.BattleCostKey{PayerID: bc.PayerID, OpponentID: bc.OpponentID}] = true
		}
	}
	return gamepkg.MissingBattleCosts(snap.Active, snap.Sides, ranks, paid), nil
}

// mwOutstandingCostsForGame aggregates outstanding costs across every active
// war in the game on `row`. The returned map is war_id → missing payments.
func mwOutstandingCostsForGame(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	row int16,
) (map[int64][]gamepkg.BattleCostKey, error) {
	wars, err := q.ListActiveWarsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	ranks, err := mwPowerRanks(ctx, q, gameID)
	if err != nil {
		return nil, err
	}
	out := map[int64][]gamepkg.BattleCostKey{}
	for _, w := range wars {
		snap, err := mwSnapshotWar(ctx, q, w)
		if err != nil {
			return nil, err
		}
		missing, err := mwOutstandingCostsForWar(ctx, q, snap, ranks, row)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			out[w.ID] = missing
		}
	}
	return out, nil
}

// mwOutstandingSurrenderClaimsForGame returns surrender claims in the game that
// still block row advance: unfulfilled (the opponent has not taken an asset)
// AND fulfillable (the surrendered player still owns at least one non-destroyed
// asset for an opponent to take).
//
// A surrendered player can be left with zero claimable assets — every asset
// destroyed paying the cost of battle, or already seized by an earlier
// claimant. The rules let each opponent take "one asset of their choice", but
// when there is nothing left to take the claim can never be fulfilled. Treating
// such claims as outstanding would gate the row forever (a soft-lock), so they
// are filtered out here: with no claimable assets the seizure resolves as a
// no-op and the row may advance.
func mwOutstandingSurrenderClaimsForGame(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) ([]dbgen.WarSurrenderClaim, error) {
	claims, err := q.ListOpenSurrenderClaimsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		return nil, nil
	}
	// Cache per surrendered player — several opponents may hold claims against
	// the same player, and the claimable pool only depends on the surrendered
	// player's assets.
	fulfillable := map[int64]bool{}
	out := make([]dbgen.WarSurrenderClaim, 0, len(claims))
	for _, c := range claims {
		has, seen := fulfillable[c.SurrenderedID]
		if !seen {
			n, err := mwClaimableAssetCount(ctx, q, c.SurrenderedID)
			if err != nil {
				return nil, err
			}
			has = n > 0
			fulfillable[c.SurrenderedID] = has
		}
		if has {
			out = append(out, c)
		}
	}
	return out, nil
}

// mwClaimableAssetCount reports how many non-destroyed assets a surrendered
// player still owns — the pool an opponent may seize via /take-surrender-asset.
func mwClaimableAssetCount(ctx context.Context, q *dbgen.Queries, ownerID int64) (int, error) {
	assets, err := q.ListAssetsByOwner(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range assets {
		if !a.IsDestroyed {
			n++
		}
	}
	return n, nil
}

// mwBroadcastBattleCostsDue sends war.battle_cost_due for each active war on
// the game. Silent on DB errors — this is best-effort notification.
func mwBroadcastBattleCostsDue(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	row int16,
) {
	wars, err := q.ListActiveWarsByGame(ctx, gameID)
	if err != nil || len(wars) == 0 {
		return
	}
	ranks, err := mwPowerRanks(ctx, q, gameID)
	if err != nil {
		return
	}
	h, hasHub := manager.Get(gameID)
	if !hasHub {
		return
	}
	for _, w := range wars {
		snap, err := mwSnapshotWar(ctx, q, w)
		if err != nil {
			continue
		}
		missing, err := mwOutstandingCostsForWar(ctx, q, snap, ranks, row)
		if err != nil || len(missing) == 0 {
			continue
		}
		byPayer := map[int64][]int64{}
		order := []int64{}
		for _, k := range missing {
			if _, seen := byPayer[k.PayerID]; !seen {
				order = append(order, k.PayerID)
			}
			byPayer[k.PayerID] = append(byPayer[k.PayerID], k.OpponentID)
		}
		payers := make([]model.CostOwedByPlayer, 0, len(order))
		for _, p := range order {
			payers = append(payers, model.CostOwedByPlayer{PlayerID: p, OpponentIDs: byPayer[p]})
		}
		h.BroadcastEvent(model.EventWarBattleCostDue, model.WarBattleCostDuePayload{
			WarID: w.ID, RowNumber: row, Payers: payers,
		})
	}
}
