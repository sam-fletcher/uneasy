package handler

// handler/prologue.go — Structured prologue endpoints (Phase 4b).
//
// Lifecycle:
//
//   tone_setting → start-prologue → prologue (choosing) → begin-ranking
//     → declare/finalize/place-set-asides repeated for power, knowledge,
//       esteem in order
//     → extra-peers (≤3 players) → main_event
//
// State columns:
//
//   games.phase                  one of "lobby","tone_setting","prologue",
//                                "main_event","shake_up","ended"
//   games.prologue_ranking_step  NULL during the choosing sub-phase; one of
//                                game.PrologueStep* during the ranking
//                                sub-flow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

const prologueTurnsPerPlayer = 3

// requirePrologueChoosing writes 409 and returns false unless the game is in
// the prologue phase and not yet in the ranking sub-flow.
func requirePrologueChoosing(w http.ResponseWriter, game *dbgen.Game) bool {
	if game.Phase != model.PhasePrologue {
		respondErr(w, http.StatusConflict, "game is not in the prologue phase")
		return false
	}
	if game.PrologueRankingStep != nil {
		respondErr(w, http.StatusConflict, "prologue is past the choosing phase")
		return false
	}
	return true
}

// ── Reads ────────────────────────────────────────────────────────────────────

// GetPrologueSheets handles GET /api/games/{id}/prologue/sheets.
//
// Returns the static sheet/choice data plus claim state:
//
//	{
//	  "sheets": [...],                       // gamepkg.PrologueSheets
//	  "claims": [{"sheet_type", "choice_name", "player_id", "turn_number"}, ...]
//	}
func GetPrologueSheets(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		claims, err := q.ListPrologueChoiceClaimsByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load claims")
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"sheets": gamepkg.PrologueSheets,
			"claims": claims,
		})
	}
}

// GetPrologueCards handles GET /api/games/{id}/prologue/cards.
//
// Returns each player's current card hand.
func GetPrologueCards(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		cards, err := q.ListPlayerCardsByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load cards")
			return
		}
		respond(w, http.StatusOK, map[string]any{"cards": cards})
	}
}

// ── Choose ───────────────────────────────────────────────────────────────────

// ChoosePrologue handles POST /api/games/{id}/prologue/choose.
//
// Body: {"sheet_type": "...", "choice_name": "..."}.
//
// Records the prologue_choice row, creates the choice-specific asset (whose
// type is determined by the sheet), and processes both linked cards
// (make-or-take semantics).
func ChoosePrologue(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		game, err := q.GetGameByID(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if !requirePrologueChoosing(w, &game) {
			return
		}

		var body struct {
			SheetType  string `json:"sheet_type"`
			ChoiceName string `json:"choice_name"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
		if choice == nil {
			respondErr(w, http.StatusBadRequest, "no such prologue choice")
			return
		}

		ctx := r.Context()

		// Per-player turn cap.
		taken, err := q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
			GameID: gameID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count player turns")
			return
		}
		if taken >= prologueTurnsPerPlayer {
			respondErr(w, http.StatusConflict, "you have already taken your three prologue turns")
			return
		}

		// Box must not be claimed.
		claimed, err := q.PrologueChoiceClaimed(ctx, dbgen.PrologueChoiceClaimedParams{
			GameID: gameID, SheetType: body.SheetType, ChoiceName: body.ChoiceName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check claim status")
			return
		}
		if claimed {
			respondErr(w, http.StatusConflict, "that box has already been claimed")
			return
		}

		turnNumber := int16(taken) + 1
		_, err = q.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
			GameID:     gameID,
			PlayerID:   player.ID,
			TurnNumber: turnNumber,
			SheetType:  body.SheetType,
			ChoiceName: body.ChoiceName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record choice")
			return
		}

		// Choice-specific asset (artifact / holding / resource depending
		// on sheet type). Name = choice name; players can edit it later.
		assetType := gamepkg.AssetTypeForSheet(body.SheetType)
		_, err = q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:    gameID,
			OwnerID:   player.ID,
			CreatorID: player.ID,
			AssetType: model.AssetType(assetType),
			Name:      body.ChoiceName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create choice asset")
			return
		}

		// Process both linked cards.
		for _, card := range choice.Cards {
			if err = processPrologueCardClaim(ctx, q, manager, gameID, player.ID, card); err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		broadcastEvent(manager, gameID, model.EventPrologueChoiceClaimed, model.PrologueChoiceClaimedPayload{
			PlayerID:   player.ID,
			SheetType:  body.SheetType,
			ChoiceName: body.ChoiceName,
			TurnNumber: turnNumber,
		})

		respond(w, http.StatusOK, map[string]any{
			"sheet_type":  body.SheetType,
			"choice_name": body.ChoiceName,
			"turn_number": turnNumber,
		})
	}
}

// processPrologueCardClaim implements make-or-take. If no asset is currently
// linked to the card, create one of the suit's natural type for the
// claimer; otherwise transfer the existing asset (and the player_cards row)
// to the claimer.
func processPrologueCardClaim(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID, claimerID int64,
	card gamepkg.Card,
) error {
	suit := string(card.Suit)
	existingOwner, err := q.GetCardOwner(ctx, dbgen.GetCardOwnerParams{
		GameID: gameID, CardSuit: suit, CardValue: card.Value,
	})
	if err == nil {
		// Transfer the existing asset and the player_cards row.
		asset, errAsset := q.GetAssetByLinkedCard(ctx, dbgen.GetAssetByLinkedCardParams{
			GameID: gameID, LinkedCardSuit: &suit, LinkedCardValue: &card.Value,
		})
		if errAsset != nil {
			return fmt.Errorf("linked asset missing: %w", errAsset)
		}
		oldOwner := asset.OwnerID
		if oldOwner == claimerID {
			return nil // already mine; rare but possible if same card listed twice
		}
		err = q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID: asset.ID, OwnerID: claimerID,
		})
		if err != nil {
			return fmt.Errorf("transfer asset: %w", err)
		}
		err = q.TransferPlayerCard(ctx, dbgen.TransferPlayerCardParams{
			PlayerID: claimerID, GameID: gameID, CardSuit: suit, CardValue: card.Value,
		})
		if err != nil {
			return fmt.Errorf("transfer card: %w", err)
		}
		updated, _ := q.GetAssetByID(ctx, asset.ID)
		broadcastEvent(manager, gameID, model.EventAssetTaken, model.AssetTakenPayload{
			Asset: updated, OldOwnerID: oldOwner, NewOwnerID: claimerID,
		})
		_ = existingOwner
		return nil
	}

	// First claim — create a new asset linked to this card.
	asset, err := q.CreateAssetWithLinkedCard(ctx, dbgen.CreateAssetWithLinkedCardParams{
		GameID:          gameID,
		OwnerID:         claimerID,
		CreatorID:       claimerID,
		AssetType:       model.AssetType(gamepkg.AssetTypeForSuit(card.Suit)),
		Name:            cardLabel(card),
		LinkedCardSuit:  &suit,
		LinkedCardValue: &card.Value,
	})
	if err != nil {
		return fmt.Errorf("create card asset: %w", err)
	}
	err = q.InsertPlayerCard(ctx, dbgen.InsertPlayerCardParams{
		GameID: gameID, PlayerID: claimerID, CardSuit: suit, CardValue: card.Value,
	})
	if err != nil {
		return fmt.Errorf("record card hand: %w", err)
	}
	broadcastEvent(manager, gameID, model.EventAssetCreated, model.AssetPayload{Asset: asset})
	return nil
}

// cardLabel returns a short display name for an asset created from a card
// claim, e.g. "K♥" or "10♦". Players will typically rename these immediately.
func cardLabel(c gamepkg.Card) string {
	suit := "?"
	switch c.Suit {
	case gamepkg.SuitClubs:
		suit = "♣"
	case gamepkg.SuitDiamonds:
		suit = "♦"
	case gamepkg.SuitSpades:
		suit = "♠"
	case gamepkg.SuitHearts:
		suit = "♥"
	}
	return c.Value + suit
}

// ── Begin ranking ────────────────────────────────────────────────────────────

// BeginPrologueRanking handles POST /api/games/{id}/prologue/begin-ranking.
//
// Facilitator-only. Requires every player to have taken three turns. Detaches
// linked_card_* from all assets in the game (the cards "live with the player"
// from here onward, not the asset) and enters the ranking sub-flow at
// declare_power.
func BeginPrologueRanking(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}
		if !requirePrologueChoosing(w, game) {
			return
		}

		ctx := r.Context()

		// Every player must have 3 turns recorded.
		players, err := q.GetPlayersByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		for _, p := range players {
			n, err := q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
				GameID: game.ID, PlayerID: p.ID,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not count turns")
				return
			}
			if n < prologueTurnsPerPlayer {
				respondErr(w, http.StatusBadRequest,
					fmt.Sprintf("player %s has not taken all three turns", p.DisplayName))
				return
			}
		}

		if err := q.ClearAssetLinkedCards(ctx, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not detach card links")
			return
		}
		step := gamepkg.PrologueStepDeclarePower
		if err := q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
			ID: game.ID, PrologueRankingStep: &step,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not enter ranking step")
			return
		}

		broadcastEvent(manager, game.ID, model.EventPrologueRankingStepChanged,
			model.PrologueRankingStepChangedPayload{Step: step})
		respond(w, http.StatusOK, map[string]any{"step": step})
	}
}
