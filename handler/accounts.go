package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

const sessionCookieMaxAge = int(365 * 24 * time.Hour / time.Second)

// maxPasswordBytes is bcrypt's hard limit: bcrypt.GenerateFromPassword
// errors on anything longer, which without this guard surfaces as a
// confusing 500. Not a password-strength policy — there is no minimum.
const maxPasswordBytes = 72

// validNotifyCadenceHours mirrors the accounts.notify_cadence_hours CHECK
// constraint (migration 048) — the five cadence options a player can pick in
// Profile → Notifications. NULL (not in this set) means off.
var validNotifyCadenceHours = map[int16]bool{1: true, 3: true, 8: true, 24: true, 72: true}

// CreateAccount handles POST /api/accounts.
//
// Body: {"username": "...", "password": "...", "email": "..."?}
// Creates the account, opens a session, and sets the cookie.
func CreateAccount(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string  `json:"username"`
			Password string  `json:"password"`
			Email    *string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		username, ok := textField(w, "username", body.Username, maxUsernameLen)
		if !ok {
			return
		}
		body.Username = username
		if body.Username == "" {
			respondErr(w, http.StatusBadRequest, "username is required")
			return
		}
		if body.Password == "" {
			respondErr(w, http.StatusBadRequest, "password is required")
			return
		}
		if len(body.Password) > maxPasswordBytes {
			respondErr(w, http.StatusBadRequest, "password too long (max 72 characters)")
			return
		}
		if body.Email != nil {
			email, ok := textField(w, "email", *body.Email, maxEmailLen)
			if !ok {
				return
			}
			body.Email = &email
		}

		ctx := r.Context()

		if _, err := s.Q.GetAccountByUsername(ctx, body.Username); err == nil {
			respondErr(w, http.StatusConflict, "username taken")
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusInternalServerError, "could not check username")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			respondInternalErr(w, r, "could not hash password", err)
			return
		}

		account, err := s.Q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username:     body.Username,
			PasswordHash: string(hash),
			Email:        body.Email,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create account", err)
			return
		}

		if err = openSession(ctx, w, s.Q, account.ID); err != nil {
			respondInternalErr(w, r, "could not open session", err)
			return
		}

		respond(w, http.StatusCreated, accountResponse(&account))
	}
}

// GetMe handles GET /api/accounts/me.
func GetMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"id":                   acct.ID,
			"username":             acct.Username,
			"email":                acct.Email,
			"notify_cadence_hours": acct.NotifyCadenceHours,
			"vapid_public_key":     vapidPublicKey,
		})
	}
}

// UpdateMe handles PATCH /api/accounts/me.
//
// Body fields are all optional: {"username": ..., "email": ..., "password": ...,
// "notify_cadence_hours": ...}. notify_cadence_hours is presence-aware: a
// caller-supplied JSON null ({"notify_cadence_hours": null}) explicitly turns
// notifications off, distinct from omitting the field entirely (which leaves
// the existing cadence untouched) — reading into a typed struct alone can't
// tell those apart, since both decode to a nil pointer, so the raw body is
// also decoded into a map to check key presence.
func UpdateMe(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "could not read body")
			return
		}

		var body struct {
			Username           *string `json:"username"`
			Email              *string `json:"email"`
			Password           *string `json:"password"`
			NotifyCadenceHours *int16  `json:"notify_cadence_hours"`
		}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var rawFields map[string]json.RawMessage
		_ = json.Unmarshal(bodyBytes, &rawFields)
		_, cadencePresent := rawFields["notify_cadence_hours"]

		ctx := r.Context()

		// Pre-validate inputs outside the transaction so we can return clean
		// 4xx errors without opening a connection. The actual writes (which
		// can partially succeed if any one fails) run atomically below.
		var newUsername *string
		if body.Username != nil {
			name, ok := textField(w, "username", *body.Username, maxUsernameLen)
			if !ok {
				return
			}
			if name == "" {
				respondErr(w, http.StatusBadRequest, "username cannot be empty")
				return
			}
			newUsername = &name
		}
		var newEmail *string
		if body.Email != nil {
			email, ok := textField(w, "email", *body.Email, maxEmailLen)
			if !ok {
				return
			}
			newEmail = &email
		}
		var newPasswordHash *string
		if body.Password != nil {
			hash, ok := hashPasswordField(w, r, *body.Password)
			if !ok {
				return
			}
			newPasswordHash = hash
		}
		if cadencePresent && body.NotifyCadenceHours != nil && !validNotifyCadenceHours[*body.NotifyCadenceHours] {
			respondErr(w, http.StatusBadRequest, "notify_cadence_hours must be one of 1, 3, 8, 24, 72, or null")
			return
		}

		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			return updateAccountFields(ctx, q, acct, newUsername, newEmail, newPasswordHash,
				cadencePresent, body.NotifyCadenceHours)
		})
		if err != nil {
			respondHTTPErr(w, r, err)
			return
		}

		updated, err := s.Q.GetAccountByID(ctx, acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not reload account", err)
			return
		}
		respond(w, http.StatusOK, accountResponse(&updated))
	}
}

// hashPasswordField validates and bcrypt-hashes a non-empty password update
// for UpdateMe, writing the appropriate 4xx (or 500, on a hash failure) and
// returning ok=false on any error. Split out to keep UpdateMe's branching
// flat — the caller only needs to check ok and return.
func hashPasswordField(w http.ResponseWriter, r *http.Request, password string) (*string, bool) {
	if password == "" {
		respondErr(w, http.StatusBadRequest, "password cannot be empty")
		return nil, false
	}
	if len(password) > maxPasswordBytes {
		respondErr(w, http.StatusBadRequest, "password too long (max 72 characters)")
		return nil, false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		respondInternalErr(w, r, "could not hash password", err)
		return nil, false
	}
	h := string(hash)
	return &h, true
}

// loadWaitStates computes the "waiting on" set for every table a player sits
// at, concurrently. Each one is a phase-dependent chain of ~3-5 queries, so
// doing them in series made ListMyTables scale with a player's table count.
// Uses the same bounded fan-out as GetGameState — see gameStateFanOut for why
// the pool needs a ceiling. Ended games and games deleted mid-request are
// skipped: both mean "nobody is being waited on".
func loadWaitStates(
	ctx context.Context,
	q *dbgen.Queries,
	rows []dbgen.ListPlayersByAccountRow,
	gameByID map[int64]dbgen.Game,
) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(rows))
	var firstErr error
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, gameStateFanOut)

	for _, row := range rows {
		game, haveGame := gameByID[row.GameID]
		if row.Phase == model.PhaseEnded || !haveGame {
			continue
		}
		wg.Add(1)
		go func(g dbgen.Game) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ws, err := computeWaitStateForGame(ctx, q, g)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if ws.ActingPlayerIDs != nil {
				out[g.ID] = ws.ActingPlayerIDs
			}
		}(game)
	}
	wg.Wait()
	return out, firstErr
}

// ListMyTables handles GET /api/accounts/me/tables.
//
// Each table carries enough context for the profile page to render a useful
// card: the game's phase, the full roster in join order (facilitator first),
// who the game is waiting on (ComputeWaitState), who is online — account
// -level WebSocket presence, so "online" means "has some table open", not
// necessarily this one — and how many chat posts the player hasn't read.
//
// waiting_on_player_ids and unread_count are deliberately separate signals:
// the first says "you owe the table a move", the second says "the table has
// been talking". A player can be caught up and still owe a move, or be idle
// with 40 posts of scene to read.
func ListMyTables(s *db.Store, m *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}
		rows, err := s.Q.ListPlayersByAccount(r.Context(), acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not list tables", err)
			return
		}
		// Game rows, rosters and unread counts for every table up front. All
		// three used to run once per table inside the loop, so a player in a
		// dozen games paid three dozen round trips to render a page they open
		// constantly — and the count grew with every table they joined.
		gameIDs := make([]int64, 0, len(rows))
		for _, row := range rows {
			gameIDs = append(gameIDs, row.GameID)
		}
		games, err := s.Q.ListGamesByIDs(r.Context(), gameIDs)
		if err != nil {
			respondInternalErr(w, r, "could not load tables", err)
			return
		}
		gameByID := make(map[int64]dbgen.Game, len(games))
		for _, g := range games {
			gameByID[g.ID] = g
		}

		allPlayers, err := s.Q.ListPlayersByGames(r.Context(), gameIDs)
		if err != nil {
			respondInternalErr(w, r, "could not list table players", err)
			return
		}
		rosterByGame := make(map[int64][]dbgen.Player, len(rows))
		for _, p := range allPlayers {
			rosterByGame[p.GameID] = append(rosterByGame[p.GameID], p)
		}

		unreadRows, err := s.Q.CountUnreadPostsByAccount(r.Context(), dbgen.CountUnreadPostsByAccountParams{
			AccountID:   acct.ID,
			MinSeverity: model.SeverityDefault,
		})
		if err != nil {
			respondInternalErr(w, r, "could not count unread posts", err)
			return
		}
		unreadByPlayer := make(map[int64]int64, len(unreadRows))
		for _, u := range unreadRows {
			unreadByPlayer[u.ViewerID] = u.UnreadCount
		}

		// Wait states for every table at once, before the assembly loop. Running
		// them one table after another made this endpoint's cost scale with how
		// many tables a player sits at — the single reason the profile page felt
		// as slow as a table.
		waitByGame, waitErr := loadWaitStates(r.Context(), s.Q, rows, gameByID)
		if waitErr != nil {
			respondInternalErr(w, r, "could not compute wait state", waitErr)
			return
		}

		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			roster := rosterByGame[row.GameID]
			players := make([]map[string]any, 0, len(roster))
			for _, p := range roster {
				players = append(players, map[string]any{
					"id":           p.ID,
					"display_name": p.DisplayName,
					"token_color":  p.TokenColor,
					"seat_order":   p.SeatOrder,
					"online":       m.IsAccountOnline(p.AccountID),
				})
			}
			// Computed in the fan-out above. Absent means either an ended game,
			// a game deleted mid-request, or a nil ActingPlayerIDs — all three
			// mean "nobody", and the empty slice is what the client expects.
			waitingOn := waitByGame[row.GameID]
			if waitingOn == nil {
				waitingOn = []int64{}
			}
			// Absent means the batch found nothing unread for this table, which
			// is a real zero — the LEFT JOIN returns a row per player either
			// way, so a missing key can only mean the player row vanished
			// mid-request.
			unread := unreadByPlayer[row.ID]
			out = append(out, map[string]any{
				"game_id":               row.GameID,
				"join_code":             row.JoinCode,
				"is_facilitator":        row.IsFacilitator,
				"joined_at":             row.JoinedAt,
				"phase":                 row.Phase,
				"player_id":             row.ID,
				"players":               players,
				"waiting_on_player_ids": waitingOn,
				"unread_count":          unread,
			})
		}
		respond(w, http.StatusOK, map[string]any{"tables": out})
	}
}

func accountResponse(a *dbgen.Account) map[string]any {
	return map[string]any{
		"id":                   a.ID,
		"username":             a.Username,
		"email":                a.Email,
		"notify_cadence_hours": a.NotifyCadenceHours,
	}
}

// updateAccountFields applies the given account field updates within a
// transaction. cadenceSet distinguishes "notify_cadence_hours omitted" (false
// — leave untouched) from "notify_cadence_hours present in the request body"
// (true — write newCadence, which may itself be nil to turn notifications
// off); see UpdateMe's doc comment for why presence can't be read off
// newCadence alone.
func updateAccountFields(ctx context.Context, q *dbgen.Queries, acct *appMiddleware.Account,
	newUsername, newEmail *string, newPasswordHash *string,
	cadenceSet bool, newCadence *int16,
) error {
	if newUsername != nil {
		if existing, err := q.GetAccountByUsername(ctx, *newUsername); err == nil && existing.ID != acct.ID {
			return httpErr(http.StatusConflict, "username taken")
		}
		if _, err := q.UpdateAccountUsername(ctx, dbgen.UpdateAccountUsernameParams{
			ID:       acct.ID,
			Username: *newUsername,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update username")
		}
		// players.display_name is a snapshot taken at join time, so propagate
		// the rename to every seat this account holds across in-progress games.
		if err := q.UpdateDisplayNameByAccount(ctx, dbgen.UpdateDisplayNameByAccountParams{
			AccountID:   acct.ID,
			DisplayName: *newUsername,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update player names")
		}
	}
	if newEmail != nil {
		var emailPtr *string
		if *newEmail != "" {
			emailPtr = newEmail
		}
		if _, err := q.UpdateAccountEmail(ctx, dbgen.UpdateAccountEmailParams{
			ID:    acct.ID,
			Email: emailPtr,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update email")
		}
	}
	if newPasswordHash != nil {
		if _, err := q.UpdateAccountPassword(ctx, dbgen.UpdateAccountPasswordParams{
			ID:           acct.ID,
			PasswordHash: *newPasswordHash,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update password")
		}
	}
	if cadenceSet {
		if _, err := q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
			ID:                 acct.ID,
			NotifyCadenceHours: newCadence,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update notification cadence")
		}
	}
	return nil
}

// openSession creates a sessions row and sets the cookie. Internal helper
// shared by CreateAccount, sessions.go, and dev.go; takes *dbgen.Queries
// directly so callers inside a transaction can pass their transactional
// handle if needed.
func openSession(ctx context.Context, w http.ResponseWriter, q *dbgen.Queries, accountID int64) error {
	token, err := db.NewCookieToken()
	if err != nil {
		return err
	}
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{
		Token:     token,
		AccountID: accountID,
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "player_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionCookieMaxAge,
	})
	return nil
}
