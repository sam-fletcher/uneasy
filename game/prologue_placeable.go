package game

// prologue_placeable.go — "Can this player still act on this track?" for the
// prologue's declare_X steps.
//
// A declare step asks every player to commit ANY cards to the live track and
// then tap Done. A player holding no ANY card they could still place has no
// move to make there: nothing to commit, and nothing to retract either —
// a card committed to an already-resolved track is spent for good, and a card
// on the LIVE track means they are holding one, so a count of zero implies an
// empty live track for them too. Waiting on their tap is pure play-by-post
// latency, not a decision, so the server treats them as done (see
// allPlayersDoneForTrack) and resolves the track outright when nobody at the
// table can act (autoResolveIfNobodyCanDeclare).
//
// Mirrored in frontend/src/lib/prologue/refund.ts (trackResolved /
// placeableHeartCount). The two must agree: the frontend decides from it
// whether to render the Done button at all, and the server decides from it
// whether that button's press is still needed.

import "slices"

// prologueTrackSequence is the order the three tracks resolve in.
var prologueTrackSequence = []string{
	PrologueTrackPower,
	PrologueTrackKnowledge,
	PrologueTrackEsteem,
}

// TrackResolved reports whether `track` has already been finalized given the
// current ranking step. Hearts committed to a resolved track are spent for
// good; hearts anywhere else are still the player's to place. A step outside
// the declare/place machine (closing, or no ranking step at all) reads as
// every track resolved.
func TrackResolved(track, currentStep string) bool {
	currentIdx := -1
	for i, t := range prologueTrackSequence {
		if currentStep == "declare_"+t || currentStep == "place_set_asides_"+t {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		return true // past all tracks (closing / done)
	}
	idx := slices.Index(prologueTrackSequence, track)
	if idx == -1 {
		return false
	}
	return idx < currentIdx
}

// PlaceableHeartCount returns how many ANY (heart) cards `playerID` could
// still put on `track` at ranking step `step`: every heart they hold, minus
// the ones locked into a track that has already resolved. Hearts already
// committed to `track` itself count — they can be retracted, so their holder
// still has a move to make.
//
// Counted rather than matched by card ID because a committed heart is always
// one of its own player's cards (CommitTrackHearts validates ownership) and
// cards cannot change hands during the ranking steps — tiles only move during
// the choosing turns, which end before prologue_ranking_step goes non-NULL.
func PlaceableHeartCount(
	playerID int64,
	track, step string,
	cards []PlayerCard,
	committed []CommittedHeart,
) int {
	held := 0
	for _, c := range cards {
		if c.PlayerID == playerID && c.Suit == SuitHearts {
			held++
		}
	}
	locked := 0
	for _, h := range committed {
		if h.PlayerID != playerID {
			continue
		}
		if h.Track != track && TrackResolved(h.Track, step) {
			locked++
		}
	}
	return held - locked
}

// AnyPlayerCanDeclare reports whether at least one of `playerIDs` still has an
// ANY card to place on (or retract from) `track`. False means the declare step
// holds no decision for anyone and can be resolved on the spot.
func AnyPlayerCanDeclare(
	playerIDs []int64,
	track, step string,
	cards []PlayerCard,
	committed []CommittedHeart,
) bool {
	for _, pid := range playerIDs {
		if PlaceableHeartCount(pid, track, step, cards, committed) > 0 {
			return true
		}
	}
	return false
}
