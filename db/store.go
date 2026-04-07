// Package db provides typed query functions for the Uneasy database.
//
// For Phase 1 these are written by hand using pgx directly. When we add sqlc
// (run `sqlc generate`), the generated code in db/gen/ will replace these.
// The function signatures are designed to match what sqlc would produce so the
// refactor is small.
package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/model"
)

// ── User tokens (pre-game identity) ──────────────────────────────────────────

// UpsertUserToken creates or updates the display name for a cookie token.
func UpsertUserToken(ctx context.Context, pool *pgxpool.Pool, token, displayName string) (model.UserToken, error) {
	var u model.UserToken
	err := pool.QueryRow(ctx, `
		INSERT INTO user_tokens (token, display_name)
		VALUES ($1, $2)
		ON CONFLICT (token) DO UPDATE SET display_name = EXCLUDED.display_name
		RETURNING token, display_name, created_at
	`, token, displayName).Scan(&u.Token, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return model.UserToken{}, fmt.Errorf("db: upsert user token: %w", err)
	}
	return u, nil
}

// GetUserToken looks up a token row. Returns pgx.ErrNoRows if not found.
func GetUserToken(ctx context.Context, pool *pgxpool.Pool, token string) (model.UserToken, error) {
	var u model.UserToken
	err := pool.QueryRow(ctx, `
		SELECT token, display_name, created_at FROM user_tokens WHERE token = $1
	`, token).Scan(&u.Token, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return model.UserToken{}, fmt.Errorf("db: get user token: %w", err)
	}
	return u, nil
}

// ── Games ─────────────────────────────────────────────────────────────────────

// CreateGame inserts a new game row with a unique join code.
func CreateGame(ctx context.Context, pool *pgxpool.Pool) (model.Game, error) {
	code, err := generateJoinCode()
	if err != nil {
		return model.Game{}, fmt.Errorf("db: generate join code: %w", err)
	}

	var g model.Game
	err = pool.QueryRow(ctx, `
		INSERT INTO games (join_code) VALUES ($1)
		RETURNING id, join_code, created_at, facilitator_id
	`, code).Scan(&g.ID, &g.JoinCode, &g.CreatedAt, &g.FacilitatorID)
	if err != nil {
		return model.Game{}, fmt.Errorf("db: create game: %w", err)
	}
	return g, nil
}

// SetFacilitator sets the facilitator_id on a game.
func SetFacilitator(ctx context.Context, pool *pgxpool.Pool, gameID, playerID int64) error {
	_, err := pool.Exec(ctx,
		`UPDATE games SET facilitator_id = $1 WHERE id = $2`,
		playerID, gameID,
	)
	if err != nil {
		return fmt.Errorf("db: set facilitator: %w", err)
	}
	return nil
}

// GetGameByID returns a game by its primary key.
func GetGameByID(ctx context.Context, pool *pgxpool.Pool, id int64) (model.Game, error) {
	var g model.Game
	err := pool.QueryRow(ctx, `
		SELECT id, join_code, created_at, facilitator_id FROM games WHERE id = $1
	`, id).Scan(&g.ID, &g.JoinCode, &g.CreatedAt, &g.FacilitatorID)
	if err != nil {
		return model.Game{}, fmt.Errorf("db: get game by id: %w", err)
	}
	return g, nil
}

// GetGameByJoinCode returns a game by its join code.
func GetGameByJoinCode(ctx context.Context, pool *pgxpool.Pool, code string) (model.Game, error) {
	var g model.Game
	err := pool.QueryRow(ctx, `
		SELECT id, join_code, created_at, facilitator_id FROM games WHERE join_code = $1
	`, code).Scan(&g.ID, &g.JoinCode, &g.CreatedAt, &g.FacilitatorID)
	if err != nil {
		return model.Game{}, fmt.Errorf("db: get game by join code: %w", err)
	}
	return g, nil
}

// ── Players ───────────────────────────────────────────────────────────────────

// CreatePlayer inserts a new player seat for a game.
func CreatePlayer(ctx context.Context, pool *pgxpool.Pool, gameID int64, displayName, cookieToken string, isFacilitator bool) (model.Player, error) {
	var p model.Player
	err := pool.QueryRow(ctx, `
		INSERT INTO players (game_id, display_name, cookie_token, is_facilitator)
		VALUES ($1, $2, $3, $4)
		RETURNING id, game_id, display_name, cookie_token, joined_at, is_facilitator
	`, gameID, displayName, cookieToken, isFacilitator).
		Scan(&p.ID, &p.GameID, &p.DisplayName, &p.CookieToken, &p.JoinedAt, &p.IsFacilitator)
	if err != nil {
		return model.Player{}, fmt.Errorf("db: create player: %w", err)
	}
	return p, nil
}

// GetPlayerByToken returns the player row for a given cookie token.
// Returns pgx.ErrNoRows if the token hasn't joined any game yet.
func GetPlayerByToken(ctx context.Context, pool *pgxpool.Pool, token string) (model.Player, error) {
	var p model.Player
	err := pool.QueryRow(ctx, `
		SELECT id, game_id, display_name, cookie_token, joined_at, is_facilitator
		FROM players WHERE cookie_token = $1
	`, token).Scan(&p.ID, &p.GameID, &p.DisplayName, &p.CookieToken, &p.JoinedAt, &p.IsFacilitator)
	if err != nil {
		return model.Player{}, fmt.Errorf("db: get player by token: %w", err)
	}
	return p, nil
}

// GetPlayersByGame returns all players in a game ordered by join time.
func GetPlayersByGame(ctx context.Context, pool *pgxpool.Pool, gameID int64) ([]model.Player, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, display_name, cookie_token, joined_at, is_facilitator
		FROM players WHERE game_id = $1 ORDER BY joined_at
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("db: get players by game: %w", err)
	}
	defer rows.Close()

	players, err := pgx.CollectRows(rows, pgx.RowToStructByPos[model.Player])
	if err != nil {
		return nil, fmt.Errorf("db: scan players: %w", err)
	}
	return players, nil
}

// IsPlayerInGame reports whether the cookie token belongs to a player in the
// given game.
func IsPlayerInGame(ctx context.Context, pool *pgxpool.Pool, gameID int64, token string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM players WHERE game_id = $1 AND cookie_token = $2
		)
	`, gameID, token).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("db: check membership: %w", err)
	}
	return exists, nil
}

// ── Posts ─────────────────────────────────────────────────────────────────────

// CreatePost inserts a new post and returns the full row.
func CreatePost(ctx context.Context, pool *pgxpool.Pool, gameID, authorID int64, body string) (model.Post, error) {
	var post model.Post
	err := pool.QueryRow(ctx, `
		INSERT INTO posts (game_id, author_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, game_id, author_id, body, created_at
	`, gameID, authorID, body).
		Scan(&post.ID, &post.GameID, &post.AuthorID, &post.Body, &post.CreatedAt)
	if err != nil {
		return model.Post{}, fmt.Errorf("db: create post: %w", err)
	}
	return post, nil
}

// ListPosts returns all posts for a game in chronological order.
func ListPosts(ctx context.Context, pool *pgxpool.Pool, gameID int64) ([]model.Post, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, author_id, body, created_at
		FROM posts WHERE game_id = $1 ORDER BY created_at ASC
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("db: list posts: %w", err)
	}
	defer rows.Close()

	posts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[model.Post])
	if err != nil {
		return nil, fmt.Errorf("db: scan posts: %w", err)
	}
	return posts, nil
}

// ListPostsAfter returns posts created after the given post ID (for catch-up
// on WebSocket reconnect).
func ListPostsAfter(ctx context.Context, pool *pgxpool.Pool, gameID, afterID int64) ([]model.Post, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, author_id, body, created_at
		FROM posts WHERE game_id = $1 AND id > $2 ORDER BY created_at ASC
	`, gameID, afterID)
	if err != nil {
		return nil, fmt.Errorf("db: list posts after: %w", err)
	}
	defer rows.Close()

	posts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[model.Post])
	if err != nil {
		return nil, fmt.Errorf("db: scan posts after: %w", err)
	}
	return posts, nil
}

// ── Token generation ─────────────────────────────────────────────────────────

// NewCookieToken generates a cryptographically random URL-safe token for use
// as a player's browser cookie.
func NewCookieToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("db: generate cookie token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// joinCodeAlphabet excludes ambiguous characters (0/O, 1/I/L) so codes are
// easy to read and share verbally.
const joinCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// generateJoinCode returns a random 6-character join code.
func generateJoinCode() (string, error) {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(joinCodeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = joinCodeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// Ensure time is imported (used by model types referenced indirectly).
var _ = time.Now
