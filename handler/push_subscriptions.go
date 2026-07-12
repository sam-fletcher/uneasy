package handler

// push_subscriptions.go — POST/DELETE /api/push-subscriptions
// (adr/NOTIFICATIONS_PLAN.md Session 3): registers and removes the browser
// PushSubscription objects the Session 4 service worker will create. Kept
// separate from push_notifications.go, which owns the reconcile+send tick
// that reads these rows back out.

import (
	"encoding/json"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// Length caps for a PushSubscription's fields. endpoint URLs from real push
// services (FCM, Mozilla autopush, Apple) run well under 512 bytes; p256dh
// (an uncompressed P-256 point) and auth (a 16-byte secret) are both short,
// fixed-size base64url strings once encoded. Generous enough that no real
// subscription ever hits them — they exist to bound a hostile client's body.
const (
	maxPushEndpointLen = 2048
	maxPushKeyLen      = 256
)

// CreatePushSubscription handles POST /api/push-subscriptions (session-authed).
//
// Body: {"endpoint": "...", "keys": {"p256dh": "...", "auth": "..."}} — the
// exact shape of PushSubscription.toJSON() from the browser Push API.
// Upserts on endpoint: a device re-subscribing (e.g. after key rotation)
// replaces its old keys and account link rather than erroring on the unique
// constraint.
func CreatePushSubscription(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var body struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		endpoint, ok := textField(w, "endpoint", body.Endpoint, maxPushEndpointLen)
		if !ok {
			return
		}
		if endpoint == "" {
			respondErr(w, http.StatusBadRequest, "endpoint is required")
			return
		}
		if !isHTTPSURL(endpoint) {
			respondErr(w, http.StatusBadRequest, "endpoint must be an https:// URL")
			return
		}
		p256dh, ok := textField(w, "p256dh", body.Keys.P256dh, maxPushKeyLen)
		if !ok {
			return
		}
		if p256dh == "" {
			respondErr(w, http.StatusBadRequest, "keys.p256dh is required")
			return
		}
		auth, ok := textField(w, "auth", body.Keys.Auth, maxPushKeyLen)
		if !ok {
			return
		}
		if auth == "" {
			respondErr(w, http.StatusBadRequest, "keys.auth is required")
			return
		}

		sub, err := s.Q.UpsertPushSubscription(r.Context(), dbgen.UpsertPushSubscriptionParams{
			AccountID: acct.ID,
			Endpoint:  endpoint,
			P256dh:    p256dh,
			Auth:      auth,
		})
		if err != nil {
			respondInternalErr(w, r, "could not save push subscription", err)
			return
		}
		respond(w, http.StatusCreated, map[string]any{"subscription": sub})
	}
}

// DeletePushSubscription handles DELETE /api/push-subscriptions (session-authed).
//
// Body: {"endpoint": "..."}. Idempotent, like DeleteSession: deleting an
// endpoint that doesn't exist (or belongs to another account) is a no-op,
// not an error — a device that's already unsubscribed shouldn't see a 404.
func DeletePushSubscription(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Endpoint == "" {
			respondErr(w, http.StatusBadRequest, "endpoint is required")
			return
		}

		if err := s.Q.DeletePushSubscriptionByEndpoint(r.Context(), dbgen.DeletePushSubscriptionByEndpointParams{
			Endpoint:  body.Endpoint,
			AccountID: acct.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not remove push subscription", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// isHTTPSURL reports whether s starts with the https:// scheme. Push service
// endpoints are always https (even against a localhost dev server — the
// secure-context requirement is on the page registering the service worker,
// not on the push service's own URL), so this is a plain prefix check rather
// than a full URL parse.
func isHTTPSURL(s string) bool {
	const httpsPrefix = "https://"
	return len(s) > len(httpsPrefix) && s[:len(httpsPrefix)] == httpsPrefix
}
