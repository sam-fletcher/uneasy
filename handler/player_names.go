package handler

// player_names.go — request-scoped cache of player rows already in hand.
//
// The chat-log emitters (system_posts.go) resolve player ids to display names
// with GetPlayerByID. Against a remote database that is one ~25ms round trip
// per name, and a single request routinely names the same player several
// times (the actor as creator, taker, and marginalia author). Handlers that
// have already loaded the relevant player rows — the acting player from auth,
// the whole roster from a turn-order check — seed them here so those lookups
// never reach the database.
//
// The cache is read-only once seeded: playerPlainName consults it but never
// fills it, so it is safe to share across the fan-out goroutines in phases.go
// without a lock. Seed with everything you know up front.

import (
	"context"
	"maps"

	dbgen "uneasy/db/gen"
)

type knownPlayersKey struct{}

// withKnownPlayers returns ctx carrying players' display names for
// playerPlainName. Layering calls merge: a later seeding sees the earlier one.
func withKnownPlayers(ctx context.Context, players ...dbgen.Player) context.Context {
	if len(players) == 0 {
		return ctx
	}
	prev, _ := ctx.Value(knownPlayersKey{}).(map[int64]string)
	names := make(map[int64]string, len(prev)+len(players))
	maps.Copy(names, prev)
	for _, p := range players {
		names[p.ID] = p.DisplayName
	}
	return context.WithValue(ctx, knownPlayersKey{}, names)
}

// knownPlayerName reports the seeded display name for playerID, if any.
func knownPlayerName(ctx context.Context, playerID int64) (string, bool) {
	names, _ := ctx.Value(knownPlayersKey{}).(map[int64]string)
	name, ok := names[playerID]
	return name, ok
}
