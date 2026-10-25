package main

import "fmt"

// allPlans is every plan type, in public-record order (power, knowledge,
// esteem; by delay within each).
var allPlans = []string{
	"make_demands", "propose_decree", "exchange_courtiers", "make_war",
	"make_introductions", "seek_answers", "chronicle_histories", "clandestinely_liaise",
	"spread_propaganda", "spread_rumors", "propose_duel", "host_festivity",
}

// rollsImmediately holds the plans that create their dice roll the moment
// resolution opens. The other nine park in a pre-roll step first and have no
// roll to force yet — their pre-roll has to be driven through the UI.
var rollsImmediately = map[string]bool{
	"spread_propaganda": true,
	"spread_rumors":     true,
	"make_demands":      true,
}

// prepNotes gives each plan a plausible preparation note, so a tester is
// reading something in the fiction's register rather than "test test".
var prepNotes = map[string]string{
	"spread_propaganda":    "The crown is illegitimate.",
	"spread_rumors":        "A debt was never paid.",
	"seek_answers":         "Who really holds the western holdfast?",
	"chronicle_histories":  "Why was the old treaty broken?",
	"propose_decree":       "No private armies within the capital.",
	"propose_duel":         "You impugned my house.",
	"host_festivity":       "A midwinter feast.",
	"make_introductions":   "Two useful people from the provinces.",
	"exchange_courtiers":   "I want your man in my household.",
	"make_war":             "Your border raids end now.",
	"clandestinely_liaise": "We should not be seen together.",
	"make_demands":         "That plan is mine to direct.",
}

// nonMCPeer returns a peer in owner's retinue that isn't their main character.
//
// A dev-seeded retinue holds exactly one peer — the main character — so plans
// that target a peer end up aimed at an MC unless one has been added. That is
// legal but atypical: taking an MC clears the main-character flag and gates
// the owner out of play until they replace it, which is its own sub-flow and
// probably not what the tester came to look at. Callers warn when they fall back.
func nonMCPeer(assets []asset, owner int64) *asset {
	for i, a := range assets {
		if a.OwnerID == owner && a.AssetType == "peer" && !a.IsMainCharacter && !a.IsDestroyed {
			return &assets[i]
		}
	}
	return nil
}

func mainCharacter(assets []asset, owner int64) (*asset, error) {
	for i, a := range assets {
		if a.OwnerID == owner && a.IsMainCharacter && !a.IsDestroyed {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("player %d has no main character", owner)
}

func firstOfType(assets []asset, owner int64, assetType string) *asset {
	for i, a := range assets {
		if a.OwnerID == owner && a.AssetType == assetType && !a.IsDestroyed {
			return &assets[i]
		}
	}
	return nil
}

// peerOrMC returns owner's non-MC peer, falling back to their main character
// and recording a warning when it has to.
func peerOrMC(assets []asset, owner int64, label string, warn func(string)) (*asset, error) {
	if p := nonMCPeer(assets, owner); p != nil {
		return p, nil
	}
	warn(fmt.Sprintf("%s is targeting a MAIN CHARACTER peer — the seed gives no other. "+
		"Taking or breaking an MC trips the main-character replacement gate, which "+
		"is its own sub-flow.", label))
	return mainCharacter(assets, owner)
}

// buildPrep returns the prepare-plan body for one plan type, with players[0]
// preparing and players[1] as the usual target.
//
// Encoding these is most of this tool's value: each plan validates a different
// subset of PreparePlanRequest, and the 400s and 422s are not always
// self-explanatory when you are guessing at a payload by hand.
func buildPrep(planType string, players []player, assets []asset, warn func(string)) (map[string]any, error) {
	notes, ok := prepNotes[planType]
	if !ok {
		return nil, fmt.Errorf("unknown plan type %q", planType)
	}
	me, them := players[0].ID, players[1].ID
	body := map[string]any{"plan_type": planType, "preparation_notes": notes}

	switch planType {
	case "make_introductions":
		body["peer_count"] = 2

	case "exchange_courtiers":
		peer, err := peerOrMC(assets, them, "Exchange Courtiers", warn)
		if err != nil {
			return nil, err
		}
		// Both ids are required: the asset alone leaves the handler guessing
		// whose retinue it is being pulled out of (a bare target_asset_id 400s).
		body["target_player_id"] = them
		body["target_asset_id"] = peer.ID

	case "spread_rumors":
		// Target a non-MC asset so the difficulty comes from the preparer's own
		// esteem rank rather than the target's status — the commoner case.
		target := firstOfType(assets, them, "artifact")
		if target == nil {
			mc, err := mainCharacter(assets, them)
			if err != nil {
				return nil, err
			}
			target = mc
		}
		body["target_asset_id"] = target.ID

	case "propose_duel":
		body["target_player_id"] = them
		body["duel_type"] = "arms"

	case "make_war":
		body["enemy_player_ids"] = []int64{them}

	case "clandestinely_liaise":
		mine, err := peerOrMC(assets, me, "Clandestinely Liaise (your side)", warn)
		if err != nil {
			return nil, err
		}
		theirs, err := peerOrMC(assets, them, "Clandestinely Liaise (partner's side)", warn)
		if err != nil {
			return nil, err
		}
		body["target_player_id"] = them
		body["preparer_peer_id"] = mine.ID
		body["partner_peer_id"] = theirs.ID
	}
	return body, nil
}
