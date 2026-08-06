package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrackResolved(t *testing.T) {
	cases := []struct {
		track, currentStep string
		want               bool
	}{
		// Power has not yet been resolved at the start.
		{"power", "declare_power", false},
		{"knowledge", "declare_power", false},
		{"esteem", "declare_power", false},

		// After power is finalized and set-asides are being placed.
		{"power", "place_set_asides_power", false},
		{"knowledge", "place_set_asides_power", false},

		// Mid-knowledge: power is resolved.
		{"power", "declare_knowledge", true},
		{"knowledge", "declare_knowledge", false},
		{"esteem", "declare_knowledge", false},

		// Mid-esteem: power and knowledge resolved.
		{"power", "declare_esteem", true},
		{"knowledge", "declare_esteem", true},
		{"esteem", "declare_esteem", false},

		// closing / past all tracks → all resolved.
		{"power", "closing", true},
		{"esteem", "closing", true},
	}
	for _, c := range cases {
		got := TrackResolved(c.track, c.currentStep)
		assert.Equal(t, c.want, got, "track=%s step=%s", c.track, c.currentStep)
	}
}

// wild is an ANY card in a player's hand.
func wild(playerID int64, value string) PlayerCard {
	return PlayerCard{PlayerID: playerID, Suit: SuitHearts, Value: value}
}

// suited is a natural (non-ANY) card, which never counts as placeable.
func suited(playerID int64, suit rune, value string) PlayerCard {
	return PlayerCard{PlayerID: playerID, Suit: suit, Value: value}
}

func spent(playerID int64, track, value string, cardID int64) CommittedHeart {
	return CommittedHeart{PlayerID: playerID, Track: track, CardID: cardID, Value: value}
}

func TestPlaceableHeartCount(t *testing.T) {
	t.Run("a player holding no ANY card has nothing to place", func(t *testing.T) {
		cards := []PlayerCard{suited(1, SuitClubs, "K"), suited(1, SuitSpades, "3")}
		assert.Equal(t, 0, PlaceableHeartCount(1, PrologueTrackPower, "declare_power", cards, nil))
	})

	t.Run("unspent ANY cards are placeable", func(t *testing.T) {
		cards := []PlayerCard{wild(1, "K"), wild(1, "4"), suited(1, SuitClubs, "9")}
		assert.Equal(t, 2, PlaceableHeartCount(1, PrologueTrackPower, "declare_power", cards, nil))
	})

	t.Run("a card locked into a resolved track is not placeable", func(t *testing.T) {
		cards := []PlayerCard{wild(1, "K"), wild(1, "4")}
		// The K went bright on power and stayed committed; only the 4 is left.
		committed := []CommittedHeart{spent(1, PrologueTrackPower, "K", 11)}
		assert.Equal(t, 1, PlaceableHeartCount(1, PrologueTrackKnowledge, "declare_knowledge", cards, committed))
	})

	t.Run("every card locked away leaves nothing to place", func(t *testing.T) {
		cards := []PlayerCard{wild(1, "K"), wild(1, "4")}
		committed := []CommittedHeart{
			spent(1, PrologueTrackPower, "K", 11),
			spent(1, PrologueTrackKnowledge, "4", 12),
		}
		assert.Equal(t, 0, PlaceableHeartCount(1, PrologueTrackEsteem, "declare_esteem", cards, committed))
	})

	t.Run("a card on the live track still counts — it can be retracted", func(t *testing.T) {
		cards := []PlayerCard{wild(1, "K")}
		committed := []CommittedHeart{spent(1, PrologueTrackEsteem, "K", 11)}
		assert.Equal(t, 1, PlaceableHeartCount(1, PrologueTrackEsteem, "declare_esteem", cards, committed))
	})

	t.Run("another player's cards and commitments are ignored", func(t *testing.T) {
		cards := []PlayerCard{wild(1, "K"), wild(2, "Q"), wild(2, "3")}
		committed := []CommittedHeart{spent(2, PrologueTrackPower, "Q", 21)}
		assert.Equal(t, 1, PlaceableHeartCount(1, PrologueTrackKnowledge, "declare_knowledge", cards, committed))
		assert.Equal(t, 1, PlaceableHeartCount(2, PrologueTrackKnowledge, "declare_knowledge", cards, committed))
	})
}

func TestAnyPlayerCanDeclare(t *testing.T) {
	t.Run("false when nobody at the table holds a spendable ANY card", func(t *testing.T) {
		cards := []PlayerCard{
			suited(1, SuitClubs, "K"), wild(2, "9"), suited(3, SuitSpades, "2"),
		}
		// Player 2's only ANY card is locked into power.
		committed := []CommittedHeart{spent(2, PrologueTrackPower, "9", 21)}
		assert.False(t, AnyPlayerCanDeclare(
			[]int64{1, 2, 3}, PrologueTrackEsteem, "declare_esteem", cards, committed))
	})

	t.Run("true while one player still holds one", func(t *testing.T) {
		cards := []PlayerCard{suited(1, SuitClubs, "K"), wild(2, "9"), wild(3, "2")}
		committed := []CommittedHeart{spent(2, PrologueTrackPower, "9", 21)}
		assert.True(t, AnyPlayerCanDeclare(
			[]int64{1, 2, 3}, PrologueTrackEsteem, "declare_esteem", cards, committed))
	})

	t.Run("false for an empty table", func(t *testing.T) {
		assert.False(t, AnyPlayerCanDeclare(nil, PrologueTrackPower, "declare_power", nil, nil))
	})
}
