package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

const maxPlayersPerGame = 5

// createMainCharacterPeer creates the empty main-character peer asset for a
// newly seated player. Per the Prologue rules, main characters exist before
// the Prologue loop; players fill in name and marginalia at any time.
func createMainCharacterPeer(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	player dbgen.Player,
) error {
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          gameID,
		OwnerID:         player.ID,
		CreatorID:       player.ID,
		AssetType:       model.AssetPeer,
		Name:            "[Main Character]",
		IsMainCharacter: true,
	})
	if err != nil {
		return err
	}
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventAssetCreated, model.AssetPayload{Asset: asset})
	}
	return nil
}

// CreateTable handles POST /api/tables.
//
// Creates a new game table and seats the calling account as facilitator.
// Requires a logged-in session.
func CreateTable(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		ctx := r.Context()

		code, err := db.GenerateJoinCode()
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not generate join code")
			return
		}

		var game dbgen.Game
		var player dbgen.Player
		seat := int16(1)
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			g, gErr := q.CreateGame(ctx, code)
			if gErr != nil {
				return errors.New("could not create table")
			}
			game = g

			p, pErr := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
				GameID:        game.ID,
				DisplayName:   acct.Username,
				AccountID:     acct.ID,
				IsFacilitator: true,
			})
			if pErr != nil {
				return errors.New("could not create player")
			}
			player = p

			if sErr := q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
				ID: player.ID, SeatOrder: &seat,
			}); sErr != nil {
				return errors.New("could not set seat order")
			}
			player.SeatOrder = &seat

			if fErr := q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
				FacilitatorID: &player.ID,
				ID:            game.ID,
			}); fErr != nil {
				return errors.New("could not set facilitator")
			}
			game.FacilitatorID = &player.ID

			if tErr := db.SeedDefaultToneTopics(ctx, q, game.ID); tErr != nil {
				return errors.New("could not seed tone topics")
			}
			if mcErr := createMainCharacterPeer(ctx, q, manager, game.ID, player); mcErr != nil {
				return errors.New("could not create main character")
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		manager.GetOrCreate(game.ID)

		respond(w, http.StatusCreated, map[string]any{
			"game":   game,
			"player": player,
		})
	}
}

// JoinTable handles POST /api/tables/join.
//
// Adds the calling account to an existing table via join code. Idempotent
// if the account is already seated. Rejects if the table is at the
// hard-coded 5-player cap.
func JoinTable(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var body struct {
			JoinCode string `json:"join_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.JoinCode = strings.ToUpper(strings.TrimSpace(body.JoinCode))
		if body.JoinCode == "" {
			respondErr(w, http.StatusBadRequest, "join_code is required")
			return
		}

		ctx := r.Context()

		game, err := s.Q.GetGameByJoinCode(ctx, body.JoinCode)
		if err != nil {
			respondErr(w, http.StatusNotFound, "join code not found")
			return
		}

		// Already seated → idempotent success.
		existing, err := s.Q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
			AccountID: acct.ID,
			GameID:    game.ID,
		})
		if err == nil {
			respond(w, http.StatusOK, map[string]any{"game": game, "player": existing})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusInternalServerError, "could not check membership")
			return
		}

		// Capacity check (not race-free; acceptable for ~10 users).
		count, err := s.Q.CountPlayersInGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check capacity")
			return
		}
		if count >= maxPlayersPerGame {
			respondErr(w, http.StatusConflict, "table is full")
			return
		}

		var player dbgen.Player
		seat := int16(count + 1)
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			p, pErr := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
				GameID:        game.ID,
				DisplayName:   acct.Username,
				AccountID:     acct.ID,
				IsFacilitator: false,
			})
			if pErr != nil {
				return errors.New("could not join table")
			}
			player = p

			if sErr := q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
				ID: player.ID, SeatOrder: &seat,
			}); sErr != nil {
				return errors.New("could not set seat order")
			}
			player.SeatOrder = &seat

			if mcErr := createMainCharacterPeer(ctx, q, manager, game.ID, player); mcErr != nil {
				return errors.New("could not create main character")
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		if h, ok := manager.Get(game.ID); ok {
			h.BroadcastEvent(model.EventPlayerJoined, model.PlayerJoinedPayload{Player: player})
		}

		respond(w, http.StatusCreated, map[string]any{
			"game":   game,
			"player": player,
		})
	}
}

// GetTable handles GET /api/tables/{id}.
func GetTable(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()

		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}

		players, err := s.Q.GetPlayersByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load members")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"game":    game,
			"players": players,
		})
	}
}
