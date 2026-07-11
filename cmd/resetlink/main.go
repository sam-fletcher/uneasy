// Command resetlink generates a single-use, 24-hour password-reset link for
// an account (the hashpw mold, for the operator-driven flow in
// adr/FEEDBACK_AND_RESET_PLAN.md). The raw token is printed once and never
// stored or reprintable — if lost, just run the command again. See
// docs/OPERATIONS.md.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "uneasy/db/gen"
)

const (
	resetTokenBytes = 32
	resetTokenTTL   = 24 * time.Hour

	// placeholderOrigin is printed when PUBLIC_ORIGIN is unset anywhere (env
	// or flag), so the operator can't mistake the output for a real link —
	// the Railway URL wasn't known yet as of this plan being written.
	placeholderOrigin = "https://PUBLIC_ORIGIN-NOT-SET.example"
)

func main() {
	origin := flag.String("origin", "", "public origin to build the link against (defaults to $PUBLIC_ORIGIN)")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: resetlink [-origin URL] <username>")
		os.Exit(1)
	}

	if err := run(args[0], *origin); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run does the actual work, returning an error rather than calling os.Exit
// directly so the deferred pool.Close() always executes.
func run(username, originFlag string) error {
	publicOrigin := originFlag
	if publicOrigin == "" {
		publicOrigin = os.Getenv("PUBLIC_ORIGIN")
	}
	if publicOrigin == "" {
		publicOrigin = placeholderOrigin
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	q := dbgen.New(pool)

	// GetAccountByUsername matches LOWER(username), the same semantics as
	// the accounts_username_lower unique index (migration 018).
	account, err := q.GetAccountByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no account named %q", username)
	} else if err != nil {
		return fmt.Errorf("look up account: %w", err)
	}

	raw, err := generateToken()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(raw))

	_, err = q.InsertPasswordResetToken(ctx, dbgen.InsertPasswordResetTokenParams{
		TokenHash: hex.EncodeToString(hash[:]),
		AccountID: account.ID,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(resetTokenTTL), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("insert reset token: %w", err)
	}

	fmt.Fprintf(os.Stdout, "%s/reset-password?token=%s\n", publicOrigin, raw)
	return nil
}

// generateToken returns a cryptographically random, URL-safe raw reset
// token. Unpadded (RawURLEncoding) since this value is embedded directly in
// a URL query string, unlike db.NewCookieToken's padded encoding for cookie
// values.
func generateToken() (string, error) {
	b := make([]byte, resetTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
