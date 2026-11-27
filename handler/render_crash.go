package handler

// render_crash.go — POST /api/render-crashes: a player's browser reports a
// render throw that a svelte:boundary caught (frontend/src/lib/renderCrash.ts).
// Only that one kind of report comes here. Transport failures, 4xx and 500
// responses are never sent, so they don't reach this table.
//
// The shape follows feedback.go: session-authed, a DB count for the rate limit,
// one row per report. It has its own limit and its own table, so a crash never
// spends the feedback budget (feedbackRateLimit counts feedback_submissions).

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// renderCrashRateLimit caps reports per account within the
// CountRecentRenderCrashReportsByAccount window (1 hour). A component that
// throws on every render would otherwise file one report per re-render; ten an
// hour is more than any real session needs to show the owner the bug.
const renderCrashRateLimit = 10

// Field caps. The client truncates to these before sending (renderCrash.ts),
// so a long stack is shortened there rather than refused here.
const (
	renderCrashBoundaryLen = 200
	renderCrashMessageLen  = 1000
	renderCrashStackLen    = 8000
	renderCrashRouteLen    = 500
	renderCrashBuildLen    = 100
)

// CreateRenderCrashReport handles POST /api/render-crashes (session-authed).
//
// Body: {"boundary": "...", "message": "...", "stack": "..."?, "route": "...",
// "build": "...", "game_id": ...?}. boundary and message are required.
//
// The account is taken from the session, never from the body. A logged-out
// tab has no account to file against, so it gets 401 and nothing is stored.
func CreateRenderCrashReport(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var reqBody struct {
			Boundary string `json:"boundary"`
			Message  string `json:"message"`
			Stack    string `json:"stack"`
			Route    string `json:"route"`
			Build    string `json:"build"`
			GameID   *int64 `json:"game_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		boundary, ok := textField(w, "boundary", reqBody.Boundary, renderCrashBoundaryLen)
		if !ok {
			return
		}
		if boundary == "" {
			respondErr(w, http.StatusBadRequest, "boundary is required")
			return
		}
		message, ok := textField(w, "message", reqBody.Message, renderCrashMessageLen)
		if !ok {
			return
		}
		if message == "" {
			respondErr(w, http.StatusBadRequest, "message is required")
			return
		}
		// textField trims only the ends of the stack, which is harmless for a trace.
		stack, ok := textField(w, "stack", reqBody.Stack, renderCrashStackLen)
		if !ok {
			return
		}
		route, ok := textField(w, "route", reqBody.Route, renderCrashRouteLen)
		if !ok {
			return
		}
		build, ok := textField(w, "build", reqBody.Build, renderCrashBuildLen)
		if !ok {
			return
		}

		ctx := r.Context()
		count, err := s.Q.CountRecentRenderCrashReportsByAccount(ctx, &acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not check render crash rate limit", err)
			return
		}
		if count >= renderCrashRateLimit {
			// The client swallows every failure of this call, so the wording is
			// for whoever reads the response in a debugger, not a player.
			respondErr(w, http.StatusTooManyRequests, "too many render crash reports for now")
			return
		}

		// The game is context, not a constraint on the report. A tab left open
		// after its game was deleted, or one that names a game the account was
		// never seated in, still gets its crash recorded, just without the link.
		// Without this check the foreign key would refuse the insert and the
		// report would be lost.
		gameID := reqBody.GameID
		if gameID != nil {
			_, err := s.Q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
				AccountID: acct.ID, GameID: *gameID,
			})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				gameID = nil
			case err != nil:
				respondInternalErr(w, r, "could not check the game for a render crash report", err)
				return
			}
		}

		rep, err := s.Q.InsertRenderCrashReport(ctx, dbgen.InsertRenderCrashReportParams{
			AccountID: &acct.ID,
			GameID:    gameID,
			Boundary:  boundary,
			Message:   message,
			Stack:     nilIfEmpty(stack),
			Route:     route,
			Build:     build,
		})
		if err != nil {
			respondInternalErr(w, r, "could not record render crash report", err)
			return
		}

		respond(w, http.StatusCreated, map[string]any{"id": rep.ID})
	}
}
