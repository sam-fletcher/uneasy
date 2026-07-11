package handler

// password_resets.go — redemption side of the operator-driven password
// reset (adr/FEEDBACK_AND_RESET_PLAN.md Session 2). cmd/resetlink issues the
// token; this handler redeems it. No self-service token issuance exists —
// the owner verifies the requester socially and runs resetlink by hand.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// resetTokenInvalidMsg is returned for every way a presented token can fail
// to redeem (not found, expired, already used) — deliberately uniform so the
// response never tells a caller which of the three applies.
const resetTokenInvalidMsg = "link invalid or expired"

// hashResetToken returns the hex-encoded SHA-256 hash of a raw reset token,
// for looking up password_reset_tokens by hash — never by a raw string
// compare, which would be a timing oracle on the token itself.
func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreatePasswordReset handles POST /api/password-resets (logged-out).
//
// Body: {"token": "...", "new_password": "..."}
// On success: hashes and stores the new password, marks the token used,
// deletes every session for the account (standard reset hygiene), and
// responds 200. No auto-login — the client sends the user to the login page.
func CreatePasswordReset(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token       string `json:"token"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Token == "" {
			respondErr(w, http.StatusBadRequest, resetTokenInvalidMsg)
			return
		}
		if body.NewPassword == "" {
			respondErr(w, http.StatusBadRequest, "password is required")
			return
		}
		if len(body.NewPassword) > maxPasswordBytes {
			respondErr(w, http.StatusBadRequest, "password too long (max 72 characters)")
			return
		}

		ctx := r.Context()
		tokenHash := hashResetToken(body.Token)

		tok, err := s.Q.GetPasswordResetToken(ctx, tokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusBadRequest, resetTokenInvalidMsg)
			return
		} else if err != nil {
			respondInternalErr(w, r, "could not look up reset token", err)
			return
		}
		if tok.UsedAt.Valid || tok.ExpiresAt.Time.Before(time.Now()) {
			respondErr(w, http.StatusBadRequest, resetTokenInvalidMsg)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			respondInternalErr(w, r, "could not hash password", err)
			return
		}

		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			if _, err := q.UpdateAccountPassword(ctx, dbgen.UpdateAccountPasswordParams{
				ID:           tok.AccountID,
				PasswordHash: string(hash),
			}); err != nil {
				return err
			}
			if err := q.MarkPasswordResetTokenUsed(ctx, tokenHash); err != nil {
				return err
			}
			return q.DeleteSessionsForAccount(ctx, tok.AccountID)
		})
		if err != nil {
			respondInternalErr(w, r, "could not reset password", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"ok": true})
	}
}
