package handler

// feedback.go — the two intakes that share the feedback_submissions table
// (adr/FEEDBACK_AND_RESET_PLAN.md): a login-gated feedback form and a
// logged-out "locked out?" password-reset request. Both notify a private
// Discord channel (notify.go) best-effort, after the DB commit.
//
// Feedback bodies are free text and may quote in-game secrets — they must
// never reach the in-game action log (no EmitSystemPost anywhere here).

import (
	"encoding/json"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// feedbackRateLimit caps feedback submissions per account within the
// CountRecentFeedbackByAccount window (1 hour) — a DB count instead of new
// middleware state, since feedback traffic is low-volume by nature.
const feedbackRateLimit = 5

// CreateFeedback handles POST /api/feedback (session-authed).
//
// Body: {"body": "...", "contact": "..."?, "game_id": ...?, "route": "..."?, "phase": "..."?}
// Auto-captures the account and User-Agent server-side; route/phase/game_id
// are client-supplied context for the "where were they" line in Discord.
func CreateFeedback(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var reqBody struct {
			Body    string `json:"body"`
			Contact string `json:"contact"`
			GameID  *int64 `json:"game_id"`
			Route   string `json:"route"`
			Phase   string `json:"phase"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		body, ok := textField(w, "body", reqBody.Body, maxNarrativeLen)
		if !ok {
			return
		}
		if body == "" {
			respondErr(w, http.StatusBadRequest, "body is required")
			return
		}
		contact, ok := textField(w, "contact", reqBody.Contact, maxEmailLen)
		if !ok {
			return
		}
		route, ok := textField(w, "route", reqBody.Route, maxEmailLen)
		if !ok {
			return
		}
		phase, ok := textField(w, "phase", reqBody.Phase, maxEmailLen)
		if !ok {
			return
		}

		ctx := r.Context()
		count, err := s.Q.CountRecentFeedbackByAccount(ctx, &acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not check feedback rate limit", err)
			return
		}
		if count >= feedbackRateLimit {
			respondErr(w, http.StatusTooManyRequests, "too much feedback for now — please try again in a bit")
			return
		}

		contextJSON, err := json.Marshal(map[string]string{
			"route":      route,
			"phase":      phase,
			"user_agent": r.UserAgent(),
		})
		if err != nil {
			respondInternalErr(w, r, "could not encode feedback context", err)
			return
		}

		sub, err := s.Q.InsertFeedbackSubmission(ctx, dbgen.InsertFeedbackSubmissionParams{
			Kind:      model.FeedbackKindFeedback,
			AccountID: &acct.ID,
			GameID:    reqBody.GameID,
			Body:      body,
			Contact:   nilIfEmpty(contact),
			Context:   contextJSON,
		})
		if err != nil {
			respondInternalErr(w, r, "could not record feedback", err)
			return
		}

		gameLabel := ""
		if sub.GameID != nil {
			gameLabel = fmt.Sprintf("game #%d", *sub.GameID)
		}
		go NotifyDiscord(model.FeedbackKindFeedback, acct.Username, gameLabel, route, body, contact)

		respond(w, http.StatusCreated, map[string]any{"submission": sub})
	}
}

// CreateResetRequest handles POST /api/reset-requests (logged-out).
//
// Body: {"username": "...", "contact": "...", "body": "..."?, "website": "..."?}
// "website" is a hidden honeypot field real users never see or fill in —
// a non-empty value marks the request as a bot and it's silently discarded,
// responding 200 as if it had succeeded. Otherwise always responds 200 (the
// only 400s are for missing required fields) so the response never reveals
// whether the typed username matches a real account; the owner resolves that
// by hand from the Discord ping.
func CreateResetRequest(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var reqBody struct {
			Username string `json:"username"`
			Contact  string `json:"contact"`
			Body     string `json:"body"`
			Website  string `json:"website"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if reqBody.Website != "" {
			respond(w, http.StatusOK, map[string]any{"ok": true})
			return
		}

		username, ok := textField(w, "username", reqBody.Username, maxUsernameLen)
		if !ok {
			return
		}
		if username == "" {
			respondErr(w, http.StatusBadRequest, "username is required")
			return
		}
		contact, ok := textField(w, "contact", reqBody.Contact, maxEmailLen)
		if !ok {
			return
		}
		if contact == "" {
			respondErr(w, http.StatusBadRequest, "contact is required — we can't reach you without it")
			return
		}
		body, ok := textField(w, "body", reqBody.Body, maxNarrativeLen)
		if !ok {
			return
		}

		if _, err := s.Q.InsertFeedbackSubmission(r.Context(), dbgen.InsertFeedbackSubmissionParams{
			Kind:     model.FeedbackKindResetRequest,
			Username: &username,
			Body:     body,
			Contact:  &contact,
		}); err != nil {
			respondInternalErr(w, r, "could not record reset request", err)
			return
		}

		go NotifyDiscord(model.FeedbackKindResetRequest, username, "", "", body, contact)

		respond(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// nilIfEmpty returns nil for an empty string, else a pointer to s — for
// optional text columns where "" and NULL should be treated the same.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
