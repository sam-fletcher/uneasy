// Package db provides typed query functions for the Uneasy database.
//
// For Phase 1 these were written by hand using pgx directly. Phase 2 continues
// this pattern — when we run `sqlc generate`, the generated code in db/gen/
// will replace these. The function signatures are designed to match what sqlc
// would produce so the refactor is small.
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

// gameColumns is the SELECT list matching model.Game field order.
const gameColumns = `id, join_code, created_at, facilitator_id, phase, current_row, focus_player_id, ending_mode, dummy_token_mode`

func scanGame(row pgx.Row) (model.Game, error) {
	var g model.Game
	err := row.Scan(
		&g.ID, &g.JoinCode, &g.CreatedAt, &g.FacilitatorID,
		&g.Phase, &g.CurrentRow, &g.FocusPlayerID,
		&g.EndingMode, &g.DummyTokenMode,
	)
	return g, err
}

// CreateGame inserts a new game row with a unique join code.
func CreateGame(ctx context.Context, pool *pgxpool.Pool) (model.Game, error) {
	code, err := generateJoinCode()
	if err != nil {
		return model.Game{}, fmt.Errorf("db: generate join code: %w", err)
	}

	g, err := scanGame(pool.QueryRow(ctx,
		`INSERT INTO games (join_code) VALUES ($1) RETURNING `+gameColumns, code))
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
	g, err := scanGame(pool.QueryRow(ctx,
		`SELECT `+gameColumns+` FROM games WHERE id = $1`, id))
	if err != nil {
		return model.Game{}, fmt.Errorf("db: get game by id: %w", err)
	}
	return g, nil
}

// GetGameByJoinCode returns a game by its join code.
func GetGameByJoinCode(ctx context.Context, pool *pgxpool.Pool, code string) (model.Game, error) {
	g, err := scanGame(pool.QueryRow(ctx,
		`SELECT `+gameColumns+` FROM games WHERE join_code = $1`, code))
	if err != nil {
		return model.Game{}, fmt.Errorf("db: get game by join code: %w", err)
	}
	return g, nil
}

// SetGamePhase updates the game's phase.
func SetGamePhase(ctx context.Context, pool *pgxpool.Pool, gameID int64, phase model.GamePhase) error {
	_, err := pool.Exec(ctx, `UPDATE games SET phase = $2 WHERE id = $1`, gameID, phase)
	if err != nil {
		return fmt.Errorf("db: set game phase: %w", err)
	}
	return nil
}

// SetFocusPlayer sets the focus player for a game.
func SetFocusPlayer(ctx context.Context, pool *pgxpool.Pool, gameID int64, playerID *int64) error {
	_, err := pool.Exec(ctx, `UPDATE games SET focus_player_id = $2 WHERE id = $1`, gameID, playerID)
	if err != nil {
		return fmt.Errorf("db: set focus player: %w", err)
	}
	return nil
}

// SetCurrentRow sets the current row for a game.
func SetCurrentRow(ctx context.Context, pool *pgxpool.Pool, gameID int64, row int16) error {
	_, err := pool.Exec(ctx, `UPDATE games SET current_row = $2 WHERE id = $1`, gameID, row)
	if err != nil {
		return fmt.Errorf("db: set current row: %w", err)
	}
	return nil
}

// CountPlayersInGame returns the number of players in a game.
func CountPlayersInGame(ctx context.Context, pool *pgxpool.Pool, gameID int64) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `SELECT count(*) FROM players WHERE game_id = $1`, gameID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("db: count players: %w", err)
	}
	return count, nil
}

// ── Players ───────────────────────────────────────────────────────────────────

// playerColumns is the SELECT list matching model.Player field order.
const playerColumns = `id, game_id, display_name, cookie_token, joined_at, is_facilitator, token_color, seat_order`

func scanPlayer(row pgx.Row) (model.Player, error) {
	var p model.Player
	err := row.Scan(
		&p.ID, &p.GameID, &p.DisplayName, &p.CookieToken,
		&p.JoinedAt, &p.IsFacilitator, &p.TokenColor, &p.SeatOrder,
	)
	return p, err
}

func scanPlayers(rows pgx.Rows) ([]model.Player, error) {
	var players []model.Player
	for rows.Next() {
		var p model.Player
		if err := rows.Scan(
			&p.ID, &p.GameID, &p.DisplayName, &p.CookieToken,
			&p.JoinedAt, &p.IsFacilitator, &p.TokenColor, &p.SeatOrder,
		); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	if players == nil {
		players = []model.Player{}
	}
	return players, rows.Err()
}

// CreatePlayer inserts a new player seat for a game.
func CreatePlayer(ctx context.Context, pool *pgxpool.Pool, gameID int64, displayName, cookieToken string, isFacilitator bool) (model.Player, error) {
	p, err := scanPlayer(pool.QueryRow(ctx, `
		INSERT INTO players (game_id, display_name, cookie_token, is_facilitator)
		VALUES ($1, $2, $3, $4)
		RETURNING `+playerColumns,
		gameID, displayName, cookieToken, isFacilitator))
	if err != nil {
		return model.Player{}, fmt.Errorf("db: create player: %w", err)
	}
	return p, nil
}

// GetPlayerByToken returns the player row for a given cookie token.
// Returns pgx.ErrNoRows if the token hasn't joined any game yet.
func GetPlayerByToken(ctx context.Context, pool *pgxpool.Pool, token string) (model.Player, error) {
	p, err := scanPlayer(pool.QueryRow(ctx,
		`SELECT `+playerColumns+` FROM players WHERE cookie_token = $1`, token))
	if err != nil {
		return model.Player{}, fmt.Errorf("db: get player by token: %w", err)
	}
	return p, nil
}

// GetPlayerByID returns a player by primary key.
func GetPlayerByID(ctx context.Context, pool *pgxpool.Pool, id int64) (model.Player, error) {
	p, err := scanPlayer(pool.QueryRow(ctx,
		`SELECT `+playerColumns+` FROM players WHERE id = $1`, id))
	if err != nil {
		return model.Player{}, fmt.Errorf("db: get player by id: %w", err)
	}
	return p, nil
}

// GetPlayersByGame returns all players in a game ordered by join time.
func GetPlayersByGame(ctx context.Context, pool *pgxpool.Pool, gameID int64) ([]model.Player, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+playerColumns+` FROM players WHERE game_id = $1 ORDER BY joined_at`, gameID)
	if err != nil {
		return nil, fmt.Errorf("db: get players by game: %w", err)
	}
	defer rows.Close()
	return scanPlayers(rows)
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

// SetPlayerSeatOrder sets the seat order for a player.
func SetPlayerSeatOrder(ctx context.Context, pool *pgxpool.Pool, playerID int64, seatOrder int16) error {
	_, err := pool.Exec(ctx, `UPDATE players SET seat_order = $2 WHERE id = $1`, playerID, seatOrder)
	if err != nil {
		return fmt.Errorf("db: set seat order: %w", err)
	}
	return nil
}

// SetPlayerTokenColor sets the token color for a player.
func SetPlayerTokenColor(ctx context.Context, pool *pgxpool.Pool, playerID int64, color string) error {
	_, err := pool.Exec(ctx, `UPDATE players SET token_color = $2 WHERE id = $1`, playerID, color)
	if err != nil {
		return fmt.Errorf("db: set token color: %w", err)
	}
	return nil
}

// ── Tone Topics ──────────────────────────────────────────────────────────────

// CreateToneTopic inserts a tone topic.
func CreateToneTopic(ctx context.Context, pool *pgxpool.Pool, gameID int64, topic string, status model.ToneTopicStatus) (model.ToneTopic, error) {
	var t model.ToneTopic
	err := pool.QueryRow(ctx, `
		INSERT INTO tone_topics (game_id, topic, status)
		VALUES ($1, $2, $3)
		RETURNING id, game_id, topic, status
	`, gameID, topic, status).Scan(&t.ID, &t.GameID, &t.Topic, &t.Status)
	if err != nil {
		return model.ToneTopic{}, fmt.Errorf("db: create tone topic: %w", err)
	}
	return t, nil
}

// ListToneTopics returns all tone topics for a game.
func ListToneTopics(ctx context.Context, pool *pgxpool.Pool, gameID int64) ([]model.ToneTopic, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, topic, status FROM tone_topics
		WHERE game_id = $1 ORDER BY id
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("db: list tone topics: %w", err)
	}
	defer rows.Close()

	var topics []model.ToneTopic
	for rows.Next() {
		var t model.ToneTopic
		if err := rows.Scan(&t.ID, &t.GameID, &t.Topic, &t.Status); err != nil {
			return nil, fmt.Errorf("db: scan tone topic: %w", err)
		}
		topics = append(topics, t)
	}
	if topics == nil {
		topics = []model.ToneTopic{}
	}
	return topics, rows.Err()
}

// UpdateToneTopicStatus updates a topic's status.
func UpdateToneTopicStatus(ctx context.Context, pool *pgxpool.Pool, topicID int64, status model.ToneTopicStatus) error {
	_, err := pool.Exec(ctx, `UPDATE tone_topics SET status = $2 WHERE id = $1`, topicID, status)
	if err != nil {
		return fmt.Errorf("db: update tone topic: %w", err)
	}
	return nil
}

// GetToneTopic returns a single tone topic by ID.
func GetToneTopic(ctx context.Context, pool *pgxpool.Pool, id int64) (model.ToneTopic, error) {
	var t model.ToneTopic
	err := pool.QueryRow(ctx, `
		SELECT id, game_id, topic, status FROM tone_topics WHERE id = $1
	`, id).Scan(&t.ID, &t.GameID, &t.Topic, &t.Status)
	if err != nil {
		return model.ToneTopic{}, fmt.Errorf("db: get tone topic: %w", err)
	}
	return t, nil
}

// SeedDefaultToneTopics inserts the standard topics from the rulebook.
func SeedDefaultToneTopics(ctx context.Context, pool *pgxpool.Pool, gameID int64) error {
	topics := []string{
		"Romance", "Sex", "Marriage", "Adultery",
		"Pregnancy and childbirth", "Religion", "Heresy",
		"Torture", "Graphic violence", "War crimes",
		"Slavery", "Addiction", "Mental illness",
		"Child endangerment", "Animal cruelty",
		"Betrayal", "Assassination", "Conspiracy",
		"Famine and poverty", "Disease and plague",
		"Supernatural elements", "Magic", "Prophecy",
		"Racism and discrimination", "Colonialism",
	}
	for _, t := range topics {
		_, err := pool.Exec(ctx, `
			INSERT INTO tone_topics (game_id, topic, status) VALUES ($1, $2, 'default')
			ON CONFLICT (game_id, topic) DO NOTHING
		`, gameID, t)
		if err != nil {
			return fmt.Errorf("db: seed tone topic %q: %w", t, err)
		}
	}
	return nil
}

// ── Rankings ─────────────────────────────────────────────────────────────────

// UpsertRanking sets a ranking position, replacing whoever was there.
func UpsertRanking(ctx context.Context, pool *pgxpool.Pool, gameID int64, playerID *int64, category model.RankingCategory, rank int16) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO rankings (game_id, player_id, category, rank)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (game_id, category, rank)
		DO UPDATE SET player_id = EXCLUDED.player_id
	`, gameID, playerID, category, rank)
	if err != nil {
		return fmt.Errorf("db: upsert ranking: %w", err)
	}
	return nil
}

// ListRankingsByGame returns all rankings for a game.
func ListRankingsByGame(ctx context.Context, pool *pgxpool.Pool, gameID int64) ([]model.Ranking, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, player_id, category, rank FROM rankings
		WHERE game_id = $1 ORDER BY category, rank
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("db: list rankings: %w", err)
	}
	defer rows.Close()

	var rankings []model.Ranking
	for rows.Next() {
		var r model.Ranking
		if err := rows.Scan(&r.ID, &r.GameID, &r.PlayerID, &r.Category, &r.Rank); err != nil {
			return nil, fmt.Errorf("db: scan ranking: %w", err)
		}
		rankings = append(rankings, r)
	}
	if rankings == nil {
		rankings = []model.Ranking{}
	}
	return rankings, rows.Err()
}

// DeleteRankingsByGame removes all rankings for a game (used before re-setting).
func DeleteRankingsByGame(ctx context.Context, pool *pgxpool.Pool, gameID int64) error {
	_, err := pool.Exec(ctx, `DELETE FROM rankings WHERE game_id = $1`, gameID)
	if err != nil {
		return fmt.Errorf("db: delete rankings: %w", err)
	}
	return nil
}

// ── Public Record Rows ───────────────────────────────────────────────────────

// CreatePublicRecordRows seeds rows 1–13 for a game.
func CreatePublicRecordRows(ctx context.Context, pool *pgxpool.Pool, gameID int64) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO public_record_rows (game_id, row_number)
		SELECT $1, generate_series(1, 13)
	`, gameID)
	if err != nil {
		return fmt.Errorf("db: create public record rows: %w", err)
	}
	return nil
}

// ── Scene Posts ──────────────────────────────────────────────────────────────

// CreateScenePost inserts a new scene post.
func CreateScenePost(ctx context.Context, pool *pgxpool.Pool, gameID int64, rowNumber *int16, planID *int64, authorID int64, body string) (model.ScenePost, error) {
	var p model.ScenePost
	err := pool.QueryRow(ctx, `
		INSERT INTO scene_posts (game_id, row_number, plan_id, author_id, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, game_id, row_number, plan_id, author_id, body, created_at
	`, gameID, rowNumber, planID, authorID, body).
		Scan(&p.ID, &p.GameID, &p.RowNumber, &p.PlanID, &p.AuthorID, &p.Body, &p.CreatedAt)
	if err != nil {
		return model.ScenePost{}, fmt.Errorf("db: create scene post: %w", err)
	}
	return p, nil
}

// ListScenePostsByRow returns all scene posts for a row, optionally filtered
// by plan_id. If planID is nil, returns posts for the open scene (plan_id IS NULL).
func ListScenePostsByRow(ctx context.Context, pool *pgxpool.Pool, gameID int64, rowNumber int16, planID *int64) ([]model.ScenePost, error) {
	var query string
	var args []any

	if planID != nil {
		query = `SELECT id, game_id, row_number, plan_id, author_id, body, created_at
			FROM scene_posts WHERE game_id = $1 AND row_number = $2 AND plan_id = $3
			ORDER BY created_at ASC`
		args = []any{gameID, rowNumber, *planID}
	} else {
		query = `SELECT id, game_id, row_number, plan_id, author_id, body, created_at
			FROM scene_posts WHERE game_id = $1 AND row_number = $2 AND plan_id IS NULL
			ORDER BY created_at ASC`
		args = []any{gameID, rowNumber}
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: list scene posts: %w", err)
	}
	defer rows.Close()

	var posts []model.ScenePost
	for rows.Next() {
		var p model.ScenePost
		if err := rows.Scan(&p.ID, &p.GameID, &p.RowNumber, &p.PlanID, &p.AuthorID, &p.Body, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan scene post: %w", err)
		}
		posts = append(posts, p)
	}
	if posts == nil {
		posts = []model.ScenePost{}
	}
	return posts, rows.Err()
}

// ListScenePostsAfter returns posts after a given ID (for reconnect catch-up).
func ListScenePostsAfter(ctx context.Context, pool *pgxpool.Pool, gameID int64, afterID int64) ([]model.ScenePost, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, game_id, row_number, plan_id, author_id, body, created_at
		FROM scene_posts WHERE game_id = $1 AND id > $2
		ORDER BY created_at ASC
	`, gameID, afterID)
	if err != nil {
		return nil, fmt.Errorf("db: list scene posts after: %w", err)
	}
	defer rows.Close()

	var posts []model.ScenePost
	for rows.Next() {
		var p model.ScenePost
		if err := rows.Scan(&p.ID, &p.GameID, &p.RowNumber, &p.PlanID, &p.AuthorID, &p.Body, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: scan scene post: %w", err)
		}
		posts = append(posts, p)
	}
	if posts == nil {
		posts = []model.ScenePost{}
	}
	return posts, rows.Err()
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
