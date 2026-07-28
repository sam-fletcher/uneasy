package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// tallyEndingVote is the whole tie-break rule, and it is the one part of the
// endgame vote with no player-visible action attached — it just happens. So it
// gets pure unit coverage of all three clauses, including clause 3, which
// CANNOT fire with today's two modes (see below) but is written now so Long
// Campaign inherits a tested tie-break rather than an untested one.

// facilitator is the player id every case below uses for the facilitator, so a
// case's intent reads off the ballot rather than off an argument.
const facilitator int64 = 1

func TestTallyEndingVote_Clause1_HighestCountWins(t *testing.T) {
	cases := []struct {
		name   string
		ballot []endingVote
		want   string
	}{
		{
			name: "unanimous smooth landing",
			ballot: []endingVote{
				{PlayerID: facilitator, Mode: EndingModeSmoothLanding},
				{PlayerID: 2, Mode: EndingModeSmoothLanding},
				{PlayerID: 3, Mode: EndingModeSmoothLanding},
			},
			want: EndingModeSmoothLanding,
		},
		{
			name: "unanimous explosive finale",
			ballot: []endingVote{
				{PlayerID: facilitator, Mode: EndingModeExplosiveFinale},
				{PlayerID: 2, Mode: EndingModeExplosiveFinale},
			},
			want: EndingModeExplosiveFinale,
		},
		{
			name: "2–1 for explosive, facilitator on the losing side",
			ballot: []endingVote{
				{PlayerID: facilitator, Mode: EndingModeSmoothLanding},
				{PlayerID: 2, Mode: EndingModeExplosiveFinale},
				{PlayerID: 3, Mode: EndingModeExplosiveFinale},
			},
			want: EndingModeExplosiveFinale,
		},
		{
			name: "3–2 for smooth, facilitator on the losing side",
			ballot: []endingVote{
				{PlayerID: facilitator, Mode: EndingModeExplosiveFinale},
				{PlayerID: 2, Mode: EndingModeExplosiveFinale},
				{PlayerID: 3, Mode: EndingModeSmoothLanding},
				{PlayerID: 4, Mode: EndingModeSmoothLanding},
				{PlayerID: 5, Mode: EndingModeSmoothLanding},
			},
			want: EndingModeSmoothLanding,
		},
		{
			name:   "single player",
			ballot: []endingVote{{PlayerID: facilitator, Mode: EndingModeExplosiveFinale}},
			want:   EndingModeExplosiveFinale,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tallyEndingVote(tc.ballot, facilitator),
				"a clear majority must win outright, whichever side the facilitator is on")
		})
	}
}

// Clause 2: on a tie, the tied option the facilitator voted for wins. With two
// modes this is the ONLY tie-break that ever fires — a tie requires an even
// roster split down the middle, so the facilitator is necessarily on one side.
func TestTallyEndingVote_Clause2_FacilitatorBreaksTheTie(t *testing.T) {
	t.Run("4-player 2–2, facilitator voted explosive", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeExplosiveFinale},
			{PlayerID: 2, Mode: EndingModeExplosiveFinale},
			{PlayerID: 3, Mode: EndingModeSmoothLanding},
			{PlayerID: 4, Mode: EndingModeSmoothLanding},
		}
		assert.Equal(t, EndingModeExplosiveFinale, tallyEndingVote(ballot, facilitator))
	})

	t.Run("4-player 2–2, facilitator voted smooth", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeSmoothLanding},
			{PlayerID: 2, Mode: EndingModeSmoothLanding},
			{PlayerID: 3, Mode: EndingModeExplosiveFinale},
			{PlayerID: 4, Mode: EndingModeExplosiveFinale},
		}
		assert.Equal(t, EndingModeSmoothLanding, tallyEndingVote(ballot, facilitator))
	})

	t.Run("2-player 1–1, facilitator voted explosive", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeExplosiveFinale},
			{PlayerID: 2, Mode: EndingModeSmoothLanding},
		}
		assert.Equal(t, EndingModeExplosiveFinale, tallyEndingVote(ballot, facilitator),
			"clause 2 decides even the smallest possible tie")
	})

	t.Run("clause 2 beats clause 3's ordering", func(t *testing.T) {
		// Explosive Finale asks MORE of the table than Smooth Landing, so
		// clause 3 alone would pick smooth. Clause 2 must win first.
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeExplosiveFinale},
			{PlayerID: 2, Mode: EndingModeExplosiveFinale},
			{PlayerID: 3, Mode: EndingModeSmoothLanding},
			{PlayerID: 4, Mode: EndingModeSmoothLanding},
		}
		assert.Equal(t, EndingModeExplosiveFinale, tallyEndingVote(ballot, facilitator))
	})
}

// Clause 3: the facilitator is not among the tied leaders, so the tied leader
// that asks LEAST of the table wins — smooth_landing → explosive_finale →
// long_campaign.
//
// This is UNREACHABLE with two modes. Reaching it needs a third option, which is
// exactly why it is tested against a hypothetical three-option ballot: when Long
// Campaign lands, the vote needs no tie-break work, only the mode itself. The DB
// CHECK on ending_votes.mode refuses 'long_campaign' today, so only a pure test
// can construct these ballots.
func TestTallyEndingVote_Clause3_LeastAskedOfTheTable(t *testing.T) {
	t.Run("the plan's own worked example: 2 smooth / 2 explosive / 1 long by the facilitator", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeLongCampaign},
			{PlayerID: 2, Mode: EndingModeSmoothLanding},
			{PlayerID: 3, Mode: EndingModeSmoothLanding},
			{PlayerID: 4, Mode: EndingModeExplosiveFinale},
			{PlayerID: 5, Mode: EndingModeExplosiveFinale},
		}
		assert.Equal(t, EndingModeSmoothLanding, tallyEndingVote(ballot, facilitator),
			"a deadlocked table falls to the status quo, not the largest obligation")
	})

	t.Run("explosive beats long when smooth is not a leader", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: facilitator, Mode: EndingModeSmoothLanding},
			{PlayerID: 2, Mode: EndingModeExplosiveFinale},
			{PlayerID: 3, Mode: EndingModeExplosiveFinale},
			{PlayerID: 4, Mode: EndingModeLongCampaign},
			{PlayerID: 5, Mode: EndingModeLongCampaign},
		}
		assert.Equal(t, EndingModeExplosiveFinale, tallyEndingVote(ballot, facilitator),
			"clause 3 walks the preference order, it does not jump to smooth unconditionally")
	})

	t.Run("a three-way tie falls all the way to smooth", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: 2, Mode: EndingModeSmoothLanding},
			{PlayerID: 3, Mode: EndingModeExplosiveFinale},
			{PlayerID: 4, Mode: EndingModeLongCampaign},
		}
		// facilitator (id 1) is not in this ballot at all.
		assert.Equal(t, EndingModeSmoothLanding, tallyEndingVote(ballot, facilitator))
	})

	t.Run("a facilitator with no vote on record cannot break the tie", func(t *testing.T) {
		ballot := []endingVote{
			{PlayerID: 2, Mode: EndingModeExplosiveFinale},
			{PlayerID: 3, Mode: EndingModeSmoothLanding},
		}
		assert.Equal(t, EndingModeSmoothLanding, tallyEndingVote(ballot, facilitator),
			"clause 2 must not match a facilitator whose mode is empty")
	})
}

func TestTallyEndingVote_EmptyBallot(t *testing.T) {
	assert.Empty(t, tallyEndingVote(nil, facilitator),
		"an empty ballot has no answer; callers gate on every player having voted")
	assert.Empty(t, tallyEndingVote([]endingVote{}, facilitator))
}

// The function must be total and deterministic even for a mode outside
// endingModePreferenceOrder. That can't happen through the route (which
// validates) or the DB (whose CHECK bounds the column), but falling out of a map
// range would make the outcome depend on Go's randomized iteration order — a
// non-reproducible game outcome is worse than an arbitrary but fixed one.
func TestTallyEndingVote_UnknownModesTieDeterministically(t *testing.T) {
	ballot := []endingVote{
		{PlayerID: facilitator, Mode: EndingModeSmoothLanding},
		{PlayerID: 2, Mode: "zeta"},
		{PlayerID: 3, Mode: "zeta"},
		{PlayerID: 4, Mode: "alpha"},
		{PlayerID: 5, Mode: "alpha"},
	}
	for range 20 {
		assert.Equal(t, "alpha", tallyEndingVote(ballot, facilitator))
	}
}

// The preference order is the one part of clause 3 a table might argue with, so
// pin it: least-asked first, and every mode the schema knows about present.
func TestEndingModePreferenceOrder(t *testing.T) {
	assert.Equal(t, []string{
		EndingModeSmoothLanding,
		EndingModeExplosiveFinale,
		EndingModeLongCampaign,
	}, endingModePreferenceOrder)
}

func TestEndingModeLabelAndConsequence(t *testing.T) {
	for _, mode := range endingModePreferenceOrder {
		assert.NotEqual(t, mode, endingModeLabel(mode),
			"every known mode gets a prose label, not its raw value")
		assert.NotEmpty(t, endingModeConsequence(mode))
	}
	// Unknown modes degrade to something readable rather than vanishing.
	assert.Equal(t, "whatever", endingModeLabel("whatever"))
	assert.NotEmpty(t, endingModeConsequence("whatever"))
}
