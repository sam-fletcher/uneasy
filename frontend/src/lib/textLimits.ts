// Free-text length caps for `maxlength` attributes. Mirrors the tier
// constants in handler/helpers.go (maxUsernameLen, maxEmailLen,
// maxAssetNameLen, maxMarginaliaLen, maxNarrativeLen, maxLongTextLen), plus
// PASSWORD which mirrors handler/accounts.go's maxPasswordBytes (bcrypt's
// 72-byte hard limit — a byte count, not a rune count like the others, but
// close enough for a `maxlength` hint since honest passwords are ASCII-ish)
// so honest users get browser-side feedback instead of a 400.
//
// A few fields intentionally cap stricter than these tiers (SuggestionPicker's
// 280 default, RetinueView's rename input at 80, SceneSetupForm's
// custom-location at 80, tone topics at 120) — those are deliberate local
// choices predating this file and are not meant to import from here.
export const TEXT_LIMITS = {
	// 20, not a generic "generous" cap: the player name is rendered in the
	// table header's pill strip, where every character is shared between up
	// to five players. 20 clears every real first name (the longest common
	// ones run 11-12) with room for a handle, and is the widest cap where
	// five max-length names still fit a laptop with no scrolling. Raising it
	// only trades header legibility for names nobody can read anyway.
	USERNAME: 20,
	EMAIL: 254,
	NAME: 120,
	MARGINALIA: 300,
	NARRATIVE: 1000,
	LONG_TEXT: 5000,
	PASSWORD: 72,
} as const;
