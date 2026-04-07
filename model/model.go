// Package model contains the core data types shared across the application.
// These mirror the database schema in db/migrations/001_phase1.up.sql.
package model

import "time"

// UserToken represents a browser identity before (and independent of) any
// game membership. One row per cookie — the "who are you" layer.
type UserToken struct {
	Token       string    `json:"token"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// Game represents a single play session ("table").
type Game struct {
	ID            int64     `json:"id"`
	JoinCode      string    `json:"join_code"`
	CreatedAt     time.Time `json:"created_at"`
	FacilitatorID *int64    `json:"facilitator_id"`
}

// Player represents a person's seat at a specific game table.
// One cookie_token maps to at most one Player row in Phase 1.
type Player struct {
	ID            int64     `json:"id"`
	GameID        int64     `json:"game_id"`
	DisplayName   string    `json:"display_name"`
	CookieToken   string    `json:"-"` // never send the raw token to the client
	JoinedAt      time.Time `json:"joined_at"`
	IsFacilitator bool      `json:"is_facilitator"`
}

// Post is a message in a table's play-by-post feed.
type Post struct {
	ID        int64     `json:"id"`
	GameID    int64     `json:"game_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
