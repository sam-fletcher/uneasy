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
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

const prologueTurnsPerPlayer = 3

// prologueTurnState reports who is currently on-turn during the choosing
// sub-phase. Players take turns in join order (facilitator first, since
// they're inserted at table-creation). The active player is the one with
// the fewest completed turns; ties broken by join order. Returns nil
// currentPlayer once every player has taken `prologueTurnsPerPlayer`.
//
// turnNumber is the 1-indexed total of choices already committed across
// all players, plus 1 (i.e. the number of the *next* turn).
func prologueTurnState(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) (currentPlayer *dbgen.Player, turnNumber int, err error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, 0, err
	}
	if len(players) == 0 {
		return nil, 1, nil
	}
	// Round 0: every player takes one turn before any player takes their second.
	// active = player with fewest turns; ties → earliest joined_at (which is
	// the iteration order returned by GetPlayersByGame).
	var totalTaken int
	var best *dbgen.Player
	bestCount := int64(prologueTurnsPerPlayer + 1)
	for i := range players {
		n, err := q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
			GameID: gameID, PlayerID: players[i].ID,
		})
		if err != nil {
			return nil, 0, err
		}
		totalTaken += int(n)
		if n < int64(prologueTurnsPerPlayer) && n < bestCount {
			best = &players[i]
			bestCount = n
		}
	}
	return best, totalTaken + 1, nil
}

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
func GetPrologueSheets(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		claims, err := s.Q.ListPrologueChoiceClaimsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load claims")
			return
		}
		active, turnNumber, err := prologueTurnState(ctx, s.Q, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute turn order")
			return
		}
		var activeID *int64
		if active != nil {
			id := active.ID
			activeID = &id
		}
		respond(w, http.StatusOK, map[string]any{
			"sheets":            gamepkg.PrologueSheets,
			"claims":            claims,
			"current_player_id": activeID,
			"turn_number":       turnNumber,
		})
	}
}

// GetPrologueCards handles GET /api/games/{id}/prologue/cards.
//
// Returns each player's current card hand.
func GetPrologueCards(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		cards, err := s.Q.ListPlayerCardsByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load cards")
			return
		}
		respond(w, http.StatusOK, map[string]any{"cards": cards})
	}
}

// ── Card-asset suggestions ───────────────────────────────────────────────────

// GetPrologueCardSuggestions handles GET /api/games/{id}/prologue/card-suggestions?suit=X.
//
// Returns three random example names for an asset of the suit's natural type
// that have not yet been used by any asset name in this game. Used to
// populate the multiple-choice picker when a player creates a card-derived
// asset during the Prologue.
func GetPrologueCardSuggestions(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		suitParam := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("suit")))
		if len(suitParam) != 1 {
			respondErr(w, http.StatusBadRequest, "suit must be one of C, D, S, H")
			return
		}
		assetType := gamepkg.AssetTypeForSuit(rune(suitParam[0]))
		if assetType == "" {
			respondErr(w, http.StatusBadRequest, "unknown suit")
			return
		}
		pool := gamepkg.PrologueExamples[assetType]
		if len(pool) == 0 {
			respond(w, http.StatusOK, map[string]any{"suggestions": []string{}})
			return
		}

		used := map[string]struct{}{}
		assets, err := s.Q.ListAssetsByGame(r.Context(), gameID)
		if err == nil {
			for _, a := range assets {
				used[strings.ToLower(strings.TrimSpace(a.Name))] = struct{}{}
			}
		}

		available := make([]string, 0, len(pool))
		for _, name := range pool {
			if _, taken := used[strings.ToLower(strings.TrimSpace(name))]; taken {
				continue
			}
			available = append(available, name)
		}

		rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
		if len(available) > 3 {
			available = available[:3]
		}

		respond(w, http.StatusOK, map[string]any{
			"suggestions": available,
			"asset_type":  assetType,
		})
	}
}

// ── Choose ───────────────────────────────────────────────────────────────────

// CardAssetText is the per-card narrative text supplied by the player when
// they create a new card-derived asset during the Prologue. For takes (the
// card's asset already exists and merely transfers owners), text is not
// required and may be empty.
type CardAssetText struct {
	Suit  string `json:"suit"`
	Value string `json:"value"`
	Text  string `json:"text"`
}

// chooseRequestBody is the validated request body for ChoosePrologue.
type chooseRequestBody struct {
	SheetType       string
	ChoiceName      string
	AssetText       string
	MarginaliumText string
	LawOrRumorText  string
	CardAssets      []CardAssetText
}

// validateChooseRequestBody decodes and validates the request body for ChoosePrologue.
func validateChooseRequestBody(r *http.Request) (*chooseRequestBody, error) {
	var raw struct {
		SheetType       string          `json:"sheet_type"`
		ChoiceName      string          `json:"choice_name"`
		AssetText       string          `json:"asset_text"`
		MarginaliumText string          `json:"marginalium_text"`
		LawOrRumorText  string          `json:"law_or_rumor_text"`
		CardAssets      []CardAssetText `json:"card_assets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	body := &chooseRequestBody{
		SheetType:       raw.SheetType,
		ChoiceName:      raw.ChoiceName,
		AssetText:       strings.TrimSpace(raw.AssetText),
		MarginaliumText: strings.TrimSpace(raw.MarginaliumText),
		LawOrRumorText:  strings.TrimSpace(raw.LawOrRumorText),
		CardAssets:      raw.CardAssets,
	}

	if body.AssetText == "" {
		return nil, errors.New("asset_text is required")
	}
	if body.SheetType == gamepkg.PrologueSheetTitles && body.MarginaliumText == "" {
		return nil, errors.New("marginalium_text is required for titles")
	}
	if body.SheetType == gamepkg.PrologueSheetLawsRumors && body.LawOrRumorText == "" {
		return nil, errors.New("law_or_rumor_text is required for laws_rumors")
	}

	return body, nil
}

// validatePlayerCanChoose checks turn order, turn cap, and box claim status.
func validatePlayerCanChoose(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	sheetType, choiceName string,
) error {
	// Turn-order enforcement: only the active player may claim a box.
	active, _, err := prologueTurnState(ctx, q, gameID)
	if err != nil {
		return httpErr(http.StatusInternalServerError, "could not compute turn order")
	}
	if active == nil || active.ID != playerID {
		return httpErr(http.StatusConflict, "it is not your turn")
	}

	// Per-player turn cap (defensive — prologueTurnState already enforces it).
	taken, err := q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
		GameID: gameID, PlayerID: playerID,
	})
	if err != nil {
		return httpErr(http.StatusInternalServerError, "could not count player turns")
	}
	if taken >= prologueTurnsPerPlayer {
		return httpErr(http.StatusConflict, "you have already taken your three prologue turns")
	}

	// Box must not be claimed.
	claimed, err := q.PrologueChoiceClaimed(ctx, dbgen.PrologueChoiceClaimedParams{
		GameID: gameID, SheetType: sheetType, ChoiceName: choiceName,
	})
	if err != nil {
		return httpErr(http.StatusInternalServerError, "could not check claim status")
	}
	if claimed {
		return httpErr(http.StatusConflict, "that box has already been claimed")
	}
	return nil
}

// findMainCharacter retrieves the player's main character asset.
func findMainCharacter(ctx context.Context, q *dbgen.Queries, playerID int64) (int64, error) {
	ownerAssets, err := q.ListAssetsByOwner(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("could not load main character: %w", err)
	}
	for _, a := range ownerAssets {
		if a.IsMainCharacter && !a.IsDestroyed {
			return a.ID, nil
		}
	}
	return 0, errors.New("main character not found")
}

// findOpenMarginaliaPosition finds an open marginalia slot (1-4) on an asset.
// Returns 0 if no open position is available.
func findOpenMarginaliaPosition(ctx context.Context, q *dbgen.Queries, assetID int64) (int16, error) {
	existing, err := q.ListMarginaliaByAsset(ctx, assetID)
	if err != nil {
		return 0, fmt.Errorf("could not load marginalia: %w", err)
	}
	used := map[int16]bool{}
	for _, m := range existing {
		used[m.Position] = true
	}
	for p := int16(1); p <= 4; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, nil
}

// addTitleMarginalium adds a marginalium to the player's main character.
func addTitleMarginalium(ctx context.Context, q *dbgen.Queries, playerID int64, text string) error {
	mainCharID, err := findMainCharacter(ctx, q, playerID)
	if err != nil {
		return httpErr(http.StatusInternalServerError, err.Error())
	}

	pos, err := findOpenMarginaliaPosition(ctx, q, mainCharID)
	if err != nil {
		return httpErr(http.StatusInternalServerError, err.Error())
	}
	if pos == 0 {
		return httpErr(http.StatusConflict, "main character has no open marginalia slots")
	}

	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  mainCharID,
		Position: pos,
		Text:     text,
	})
	if err != nil {
		return httpErr(http.StatusInternalServerError, "could not add title marginalium")
	}
	return nil
}

// isLawChoice detects whether a choice is a law or rumor based on the choice name.
func isLawChoice(choiceName string) bool {
	return strings.Contains(strings.ToLower(choiceName), "law")
}

// addLawOrRumor records a law or rumor in the public record and broadcasts
// the corresponding WS event so connected clients update their Laws/Rumors
// panels without a refresh.
func addLawOrRumor(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID, playerID int64,
	choiceName, text string,
) error {
	pid := playerID
	if isLawChoice(choiceName) {
		law, err := q.CreateLaw(ctx, dbgen.CreateLawParams{
			GameID:      gameID,
			Text:        text,
			SignatoryID: &pid,
		})
		if err != nil {
			return httpErr(http.StatusInternalServerError, "could not record law")
		}
		broadcastEvent(manager, gameID, model.EventLawEnacted, model.LawEnactedPayload{Law: law})
	} else {
		rumor, err := q.CreateRumor(ctx, dbgen.CreateRumorParams{
			GameID:         gameID,
			Text:           text,
			SourcePlayerID: &pid,
		})
		if err != nil {
			return httpErr(http.StatusInternalServerError, "could not record rumor")
		}
		broadcastEvent(manager, gameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	}
	return nil
}

// buildCardTextLookup creates a map from card key ("C|K") to player-supplied text.
func buildCardTextLookup(cardAssets []CardAssetText) map[string]string {
	lookup := make(map[string]string)
	for _, ct := range cardAssets {
		key := strings.ToUpper(ct.Suit) + "|" + strings.ToUpper(ct.Value)
		lookup[key] = strings.TrimSpace(ct.Text)
	}
	return lookup
}

// broadcastTurnAdvanced broadcasts the turn advancement event.
func broadcastTurnAdvanced(ctx context.Context, manager *hub.Manager, q *dbgen.Queries, gameID int64) {
	// Advance the turn marker.
	nextActive, nextTurn, terr := prologueTurnState(ctx, q, gameID)
	if terr == nil {
		payload := model.PrologueTurnAdvancedPayload{TurnNumber: nextTurn}
		if nextActive != nil {
			id := nextActive.ID
			payload.CurrentPlayerID = &id
		}
		broadcastEvent(manager, gameID, model.EventPrologueTurnAdvanced, payload)
	}
}

// ChoosePrologue handles POST /api/games/{id}/prologue/choose.
//
// Body:
//
//	{
//	  "sheet_type":         "...",
//	  "choice_name":        "...",
//	  "asset_text":         "...",   // name for the sheet-derived asset
//	  "marginalium_text":   "...",   // titles only — added to main character
//	  "law_or_rumor_text":  "...",   // laws_rumors only — added to public record
//	  "card_assets": [
//	    {"suit":"C","value":"K","text":"..."}, ...    // text required for makes; ignored for takes
//	  ]
//	}
//
// Records the prologue_choice row, creates the choice-specific asset, adds
// the title marginalium / law / rumor as appropriate, and processes both
// linked cards (make-or-take semantics). Players are required to author
// every text field; silent automation is not permitted.
func ChoosePrologue(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if !requirePrologueChoosing(w, &game) {
			return
		}

		body, err := validateChooseRequestBody(r)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
		if choice == nil {
			respondErr(w, http.StatusBadRequest, "no such prologue choice")
			return
		}

		// All writes for a single choice — turn record, choice asset, sheet-type
		// side-effect (marginalium/law/rumor), and per-card make-or-take —
		// must commit atomically. Without this, a downstream failure (e.g.
		// "no open marginalia slots") leaves the prologue_choice row
		// committed, silently consuming the player's turn and shifting the
		// turn marker to the next player.
		var turnNumber int16
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			var txErr error
			turnNumber, txErr = recordPrologueChoice(ctx, q, manager, gameID, player.ID, body, choice)
			return txErr
		})
		if err != nil {
			respondHTTPErr(w, err)
			return
		}

		broadcastEvent(manager, gameID, model.EventPrologueChoiceClaimed, model.PrologueChoiceClaimedPayload{
			PlayerID:   player.ID,
			SheetType:  body.SheetType,
			ChoiceName: body.ChoiceName,
			TurnNumber: turnNumber,
		})

		broadcastTurnAdvanced(ctx, manager, s.Q, gameID)

		respond(w, http.StatusOK, map[string]any{
			"sheet_type":  body.SheetType,
			"choice_name": body.ChoiceName,
			"turn_number": turnNumber,
		})
	}
}

func recordPrologueChoice(ctx context.Context, q *dbgen.Queries, manager *hub.Manager,
	gameID, playerID int64, body *chooseRequestBody, choice *gamepkg.PrologueChoice,
) (int16, error) {
	var turnNumber int16
	if err := validatePlayerCanChoose(ctx, q, gameID, playerID, body.SheetType, body.ChoiceName); err != nil {
		return turnNumber, err
	}

	taken, err := q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
		GameID: gameID, PlayerID: playerID,
	})
	if err != nil {
		return turnNumber, httpErr(http.StatusInternalServerError, "could not count player turns")
	}
	turnNumber = int16(taken) + 1

	if _, err := q.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
		GameID:     gameID,
		PlayerID:   playerID,
		TurnNumber: turnNumber,
		SheetType:  body.SheetType,
		ChoiceName: body.ChoiceName,
	}); err != nil {
		return turnNumber, httpErr(http.StatusInternalServerError, "could not record choice")
	}

	assetType := gamepkg.AssetTypeForSheet(body.SheetType)
	if _, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    gameID,
		OwnerID:   playerID,
		CreatorID: playerID,
		AssetType: model.AssetType(assetType),
		Name:      body.AssetText,
	}); err != nil {
		return turnNumber, httpErr(http.StatusInternalServerError, "could not create choice asset")
	}

	switch body.SheetType {
	case gamepkg.PrologueSheetTitles:
		if err := addTitleMarginalium(ctx, q, playerID, body.MarginaliumText); err != nil {
			return turnNumber, err
		}
	case gamepkg.PrologueSheetLawsRumors:
		if err := addLawOrRumor(ctx, q, manager, gameID, playerID, body.ChoiceName, body.LawOrRumorText); err != nil {
			return turnNumber, err
		}
	}

	cardTextLookup := buildCardTextLookup(body.CardAssets)
	for _, card := range choice.Cards {
		key := strings.ToUpper(string(card.Suit)) + "|" + strings.ToUpper(card.Value)
		if err := processPrologueCardClaim(ctx, q, manager,
			gameID, playerID, card, cardTextLookup[key],
		); err != nil {
			return turnNumber, httpErr(http.StatusInternalServerError, err.Error())
		}
	}
	return turnNumber, nil
}

// processPrologueCardClaim implements make-or-take. If no asset is currently
// linked to the card, create one of the suit's natural type for the
// claimer using makeText as its name; otherwise transfer the existing
// asset (and the player_cards row) to the claimer (makeText is unused for
// takes).
func processPrologueCardClaim(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID, claimerID int64,
	card gamepkg.Card,
	makeText string,
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

	// First claim — create a new asset linked to this card. Player-supplied
	// text is required.
	if strings.TrimSpace(makeText) == "" {
		return fmt.Errorf("text required for new card asset %s", cardLabel(card))
	}
	asset, err := q.CreateAssetWithLinkedCard(ctx, dbgen.CreateAssetWithLinkedCardParams{
		GameID:          gameID,
		OwnerID:         claimerID,
		CreatorID:       claimerID,
		AssetType:       model.AssetType(gamepkg.AssetTypeForSuit(card.Suit)),
		Name:            strings.TrimSpace(makeText),
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
func BeginPrologueRanking(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, s.Q)
		if !ok {
			return
		}
		if !requirePrologueChoosing(w, game) {
			return
		}

		ctx := r.Context()

		// Every player must have 3 turns recorded.
		players, err := s.Q.GetPlayersByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		for _, p := range players {
			n, err := s.Q.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{
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

		step := gamepkg.PrologueStepDeclarePower
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			if cErr := q.ClearAssetLinkedCards(ctx, game.ID); cErr != nil {
				return httpErr(http.StatusInternalServerError, "could not detach card links")
			}
			if sErr := q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
				ID: game.ID, PrologueRankingStep: &step,
			}); sErr != nil {
				return httpErr(http.StatusInternalServerError, "could not enter ranking step")
			}
			return nil
		})
		if err != nil {
			respondHTTPErr(w, err)
			return
		}

		broadcastEvent(manager, game.ID, model.EventPrologueRankingStepChanged,
			model.PrologueRankingStepChangedPayload{Step: step})
		respond(w, http.StatusOK, map[string]any{"step": step})
	}
}
