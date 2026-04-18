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
func mwOutstandingCostsForWar(
	ctx context.Context,
	q *dbgen.Queries,
	snap mwWarSnapshot,
	ranks map[int64]int16,
	row int16,
) ([]gamepkg.BattleCostKey, error) {
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

// mwOutstandingSurrenderClaimsForGame returns surrender claims in the game
// that have not yet been fulfilled (opponent has not taken an asset).
func mwOutstandingSurrenderClaimsForGame(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) ([]dbgen.WarSurrenderClaim, error) {
	return q.ListOpenSurrenderClaimsByGame(ctx, gameID)
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
