package game

import "encoding/json"

// prologue_sheets.go — Reference data for the structured prologue (Phase 4b).
//
// The 36 prologue boxes (12 titles + 12 origins + 12 laws-and-rumors) and
// their associated card pairs are static and game-rule-defined, so they
// live as Go constants rather than DB rows. SheetType ↔ ChoiceAssetType is
// 1:1, so we encode the asset type at the sheet level instead of duplicating
// it on every choice.
//
// Source for card pairs and title descriptions: PROLOGUE_CARDS.md.
// Hailing From and Laws & Rumors choices have no rulebook description; only
// titles do.

// Sheet type constants. Match the CHECK constraint on prologue_choices.sheet_type.
const (
	PrologueSheetTitles      = "titles"
	PrologueSheetHailingFrom = "hailing_from"
	PrologueSheetLawsRumors  = "laws_rumors"
)

// Track / suit constants used by the ranking algorithm.
const (
	PrologueTrackPower     = "power"
	PrologueTrackKnowledge = "knowledge"
	PrologueTrackEsteem    = "esteem"
)

const (
	SuitClubs    = 'C'
	SuitDiamonds = 'D'
	SuitSpades   = 'S'
	SuitHearts   = 'H'
)

// SuitForTrack returns the natural-fit suit for a given ranking track.
// Hearts are wild and must be assigned via player declarations; this
// helper returns only the non-wild correspondence.
func SuitForTrack(track string) rune {
	switch track {
	case PrologueTrackPower:
		return SuitClubs
	case PrologueTrackKnowledge:
		return SuitDiamonds
	case PrologueTrackEsteem:
		return SuitSpades
	}
	return 0
}

// PrologueRanking step constants for games.prologue_ranking_step.
const (
	PrologueStepDeclarePower         = "declare_power"
	PrologueStepPlaceSetAsidesPower  = "place_set_asides_power"
	PrologueStepDeclareKnowledge     = "declare_knowledge"
	PrologueStepPlaceSetAsidesKnow   = "place_set_asides_knowledge"
	PrologueStepDeclareEsteem        = "declare_esteem"
	PrologueStepPlaceSetAsidesEsteem = "place_set_asides_esteem"
	PrologueStepExtraPeers           = "extra_peers"
)

// Card represents a single playing card. Suit is one of 'C','D','S','H'.
// Value is "A","2"–"10","J","Q","K".
type Card struct {
	Suit  rune   `json:"suit"`
	Value string `json:"value"`
}

// MarshalJSON emits Suit as a single-character string (e.g. "H") rather than
// the rune's numeric code point, so JSON consumers see "H" not 72.
func (c Card) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Suit  string `json:"suit"`
		Value string `json:"value"`
	}{
		Suit:  string(c.Suit),
		Value: c.Value,
	})
}

// PrologueChoice is one box on a prologue sheet. The two linked cards drive
// make-or-take of card-asset linkage; their order is presentation-only.
//
// ID is the stable title id (ADR-007) for boxes on the titles sheet — it is
// stamped onto the claim marginalia and is the canonical role. It is empty for
// the hailing-from and laws-rumors sheets, which carry no title.
type PrologueChoice struct {
	Name        string  `json:"name"`
	ID          string  `json:"id,omitempty"`
	Description string  `json:"description"`
	Cards       [2]Card `json:"cards"`
}

// PrologueSheet groups 12 choices that share an asset type (titles →
// artifact, hailing_from → holding, laws_rumors → resource).
type PrologueSheet struct {
	Type            string           `json:"type"`
	DisplayName     string           `json:"display_name"`
	ChoiceAssetType string           `json:"choice_asset_type"`
	Choices         []PrologueChoice `json:"choices"`
}

// Asset-type constants used at the sheet level. Mirror model.AssetType
// values without a model import (game/ does not depend on model/ for these
// strings; sheets are pure rule data).
const (
	prologueAssetArtifact = "artifact"
	prologueAssetHolding  = "holding"
	prologueAssetResource = "resource"
)

// titlesSheet — 12 boxes, each creates an artifact representing a title.
// Card pairs and descriptions transcribed from PROLOGUE_CARDS.md.
var titlesSheet = []PrologueChoice{
	{
		Name: "The Monarch",
		ID:   TitleMonarch,
		Description: "You are the head of the kingdom. All members of the court are your subjects, " +
			"and all decisions are yours.",
		Cards: [2]Card{{SuitClubs, "K"}, {SuitDiamonds, "K"}},
	},
	{
		Name: "The Consort",
		ID:   TitleConsort,
		Description: "You are the legal spouse of the monarch or another member of the court, " +
			"but you hold greater ambition.",
		Cards: [2]Card{{SuitHearts, "K"}, {SuitDiamonds, "Q"}},
	},
	{
		Name: "The True Heir",
		ID:   TitleTrueHeir,
		Description: "By the law, you shall inherit the title of monarch whenever the time comes. " +
			"You will surely rule one day.",
		Cards: [2]Card{{SuitHearts, "K"}, {SuitClubs, "A"}},
	},
	{
		Name: "The Favored Heir",
		ID:   TitleFavoredHeir,
		Description: "You might not legally be first in line, but you are favored to inherit the throne " +
			"by the current monarch.",
		Cards: [2]Card{{SuitHearts, "J"}, {SuitDiamonds, "A"}},
	},
	{
		Name: "The Claimant",
		ID:   TitleClaimant,
		Description: "You have a legitimate claim to the title of monarch, but for some reason " +
			"you're on the outside looking in.",
		Cards: [2]Card{{SuitDiamonds, "K"}, {SuitClubs, "A"}},
	},
	{
		Name: "The Paramour",
		ID:   TitleParamour,
		Description: "You are having an affair with the monarch or another member of the court. " +
			"Perhaps it's love, perhaps it's not.",
		Cards: [2]Card{{SuitDiamonds, "Q"}, {SuitClubs, "Q"}},
	},
	{
		Name: "The Absolver",
		ID:   TitleAbsolver,
		Description: "You are the head of church. You speak for your religion and aim to ensure its practice " +
			"throughout the realm.",
		Cards: [2]Card{{SuitHearts, "A"}, {SuitDiamonds, "J"}},
	},
	{
		Name: "The Heretic",
		ID:   TitleHeretic,
		Description: "You don't abide the standard religion of the realm. You might practice another faith, " +
			"or reject religion completely.",
		Cards: [2]Card{{SuitHearts, "A"}, {SuitHearts, "Q"}},
	},
	{
		Name: "The Visiting Royal",
		ID:   TitleVisitingRoyal,
		Description: "You are the head of another kingdom. You might be visiting for peace talks, " +
			"or just as a show of good faith.",
		Cards: [2]Card{{SuitClubs, "J"}, {SuitHearts, "Q"}},
	},
	{
		Name: "The General",
		ID:   TitleGeneral,
		Description: "You are a master of arms and commander of armies. When things come to blows, " +
			"it's unwise to cross you.",
		Cards: [2]Card{{SuitClubs, "Q"}, {SuitClubs, "J"}},
	},
	{
		Name: "The Lawyer",
		ID:   TitleLawyer,
		Description: "You know the laws of the realm better than anybody. You will bend the law " +
			"to shape the realm.",
		Cards: [2]Card{{SuitClubs, "K"}, {SuitHearts, "J"}},
	},
	{
		Name: "The Spymaster",
		ID:   TitleSpymaster,
		Description: "You have an ear in every room, and eyes on every document. With knowledge, " +
			"all things are possible.",
		Cards: [2]Card{{SuitDiamonds, "A"}, {SuitDiamonds, "J"}},
	},
}

// hailingFromSheet — 12 boxes, each creates a holding in the chosen region.
// Card pairs transcribed from PROLOGUE_CARDS.md. This sheet has no rulebook
// descriptions.
//
//nolint:dupl // 12-row card-pair literal; same shape as lawsRumorsSheet by design
var hailingFromSheet = []PrologueChoice{
	{Name: "In the Capital", Cards: [2]Card{{SuitHearts, "K"}, {SuitDiamonds, "K"}}},
	{Name: "By the Seaside", Cards: [2]Card{{SuitHearts, "J"}, {SuitHearts, "A"}}},
	{Name: "Near the Enemy", Cards: [2]Card{{SuitDiamonds, "A"}, {SuitHearts, "Q"}}},
	{Name: "Somewhere Lush", Cards: [2]Card{{SuitDiamonds, "A"}, {SuitDiamonds, "Q"}}},
	{Name: "In the Mountains", Cards: [2]Card{{SuitSpades, "Q"}, {SuitDiamonds, "J"}}},
	{Name: "Somewhere Far Away", Cards: [2]Card{{SuitHearts, "K"}, {SuitHearts, "Q"}}},
	{Name: "In the Desert", Cards: [2]Card{{SuitDiamonds, "Q"}, {SuitSpades, "K"}}},
	{Name: "Somewhere Inland", Cards: [2]Card{{SuitHearts, "J"}, {SuitSpades, "A"}}},
	{Name: "The Plains", Cards: [2]Card{{SuitSpades, "Q"}, {SuitSpades, "J"}}},
	{Name: "The River Delta", Cards: [2]Card{{SuitHearts, "A"}, {SuitSpades, "J"}}},
	{Name: "The Frozen Lands", Cards: [2]Card{{SuitDiamonds, "K"}, {SuitSpades, "A"}}},
	{Name: "Upon the Cliffs", Cards: [2]Card{{SuitDiamonds, "J"}, {SuitSpades, "K"}}},
}

// lawsRumorsSheet — 12 boxes (6 laws + 6 rumors), each creates a resource
// representing the benefit your character gains from the law/rumor. Card
// pairs transcribed from PROLOGUE_CARDS.md. No rulebook descriptions.
//
//nolint:dupl // 12-row card-pair literal; same shape as hailingFromSheet by design
var lawsRumorsSheet = []PrologueChoice{
	{Name: "Selfish Law", Cards: [2]Card{{SuitHearts, "Q"}, {SuitSpades, "A"}}},
	{Name: "Popular Law", Cards: [2]Card{{SuitClubs, "Q"}, {SuitSpades, "Q"}}},
	{Name: "Righteous Law", Cards: [2]Card{{SuitSpades, "K"}, {SuitSpades, "J"}}},
	{Name: "Religious Law", Cards: [2]Card{{SuitClubs, "K"}, {SuitHearts, "A"}}},
	{Name: "Useless Law", Cards: [2]Card{{SuitHearts, "K"}, {SuitHearts, "J"}}},
	{Name: "Ancient Law", Cards: [2]Card{{SuitHearts, "J"}, {SuitClubs, "A"}}},
	{Name: "True Rumor", Cards: [2]Card{{SuitClubs, "J"}, {SuitClubs, "Q"}}},
	{Name: "Flattering Rumor", Cards: [2]Card{{SuitHearts, "Q"}, {SuitSpades, "J"}}},
	{Name: "Insulting Rumor", Cards: [2]Card{{SuitHearts, "A"}, {SuitHearts, "K"}}},
	{Name: "False Rumor", Cards: [2]Card{{SuitSpades, "K"}, {SuitClubs, "K"}}},
	{Name: "Old Rumor", Cards: [2]Card{{SuitClubs, "A"}, {SuitSpades, "Q"}}},
	{Name: "Fresh Rumor", Cards: [2]Card{{SuitSpades, "A"}, {SuitHearts, "J"}}},
}

// PrologueSheets is the canonical ordered list of all three sheets.
var PrologueSheets = []PrologueSheet{
	{Type: PrologueSheetTitles, DisplayName: "Titles", ChoiceAssetType: prologueAssetArtifact, Choices: titlesSheet},
	{
		Type:            PrologueSheetHailingFrom,
		DisplayName:     "Hailing From",
		ChoiceAssetType: prologueAssetHolding,
		Choices:         hailingFromSheet,
	},
	{
		Type:            PrologueSheetLawsRumors,
		DisplayName:     "Laws & Rumors",
		ChoiceAssetType: prologueAssetResource,
		Choices:         lawsRumorsSheet,
	},
}

// FindPrologueChoice returns the choice with the given sheet/name, or nil.
func FindPrologueChoice(sheetType, name string) *PrologueChoice {
	for _, s := range PrologueSheets {
		if s.Type != sheetType {
			continue
		}
		for i := range s.Choices {
			if s.Choices[i].Name == name {
				return &s.Choices[i]
			}
		}
	}
	return nil
}

// AssetTypeForSheet returns the asset type a claim on the given sheet
// creates ("artifact"/"holding"/"resource"), or "" if unknown.
func AssetTypeForSheet(sheetType string) string {
	for _, s := range PrologueSheets {
		if s.Type == sheetType {
			return s.ChoiceAssetType
		}
	}
	return ""
}

// AssetTypeForSuit returns the asset type a card-claim of the given suit
// creates: ♣→holding, ♦→resource, ♠→artifact, ♥→peer.
func AssetTypeForSuit(suit rune) string {
	switch suit {
	case SuitClubs:
		return prologueAssetHolding
	case SuitDiamonds:
		return prologueAssetResource
	case SuitSpades:
		return prologueAssetArtifact
	case SuitHearts:
		return "peer"
	}
	return ""
}
