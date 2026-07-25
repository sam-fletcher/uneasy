// Free-text length caps for `maxlength` attributes. Mirrors the tier
// constants in handler/helpers.go (maxUsernameLen, maxEmailLen,
// maxAssetNameLen, maxMarginaliaLen, maxNarrativeLen, maxLongTextLen), plus
// PASSWORD which mirrors handler/accounts.go's maxPasswordBytes (bcrypt's
// 72-byte hard limit — a byte count, not a rune count like the others, but
// close enough for a `maxlength` hint since honest passwords are ASCII-ish)
// so honest users get browser-side feedback instead of a 400.
//
// Two fields still cap stricter than these tiers, both deliberate: the tone
// topic and the scene time-note stay at 120. A tone topic is allowed to grow
// its tile — at 120 chars it wraps to 9 lines and roughly triples the 44px
// tile height, and since .tone-grid is `repeat(3, 1fr)` the whole row grows
// with it. That's accepted: a topic someone needs to spell out is worth the
// space. Everything else imports from here.
export const TEXT_LIMITS = {
	// 20, not a generic "generous" cap: the player name is rendered in the
	// table header's pill strip, where every character is shared between up
	// to five players. 20 clears every real first name (the longest common
	// ones run 11-12) with room for a handle, and is the widest cap where
	// five max-length names still fit a laptop with no scrolling. Raising it
	// only trades header legibility for names nobody can read anyway.
	USERNAME: 20,
	EMAIL: 254,
	// 50, read off the expanded asset card. AssetCardSelectable's collapsed
	// rows ellipsize on one line (~31 chars on a phone) and that's intended —
	// they're a density surface and open on tap. The expanded card is where
	// the name is promised in full ("engaging with a card should reveal the
	// full name", AssetCardSelectable.svelte), so the cap is whatever its
	// 2-line clamp reliably holds.
	//
	// Measured in the real card (210px name box, 390px viewport) against
	// eight realistic long names: 45/48/50/52 all fit two lines 8 times out
	// of 8; 55 clips 2 of 8 and 60 clips 5 of 8. The boundary is 52 — where a
	// name lands depends on its word breaks, so the cap has to clear the
	// unluckiest breaks, not the luckiest. 50 sits just under it.
	//
	// Desktop doesn't relax this: the column band caps at 440
	// (docs/STYLE_GUIDE.md "Layout widths"), so the phone is the binding
	// case. For scale, the longest name in PROLOGUE_CARDS.md is 34 characters.
	NAME: 50,
	// 160, read off the same expanded card, where marginalia wrap freely
	// rather than clamp — so the cost of a long one is line count, not
	// clipping. 160 is the most that still fits 4 lines at 271px on a phone
	// (180 tips into 5, the old 300 took 8), and it keeps a chat log entry
	// for a marginalia edit down to ~5 lines instead of 7. Note this does NOT
	// make RetinueView's 2x2 tiles lossless — those clamp at ~47 chars on a
	// phone and are layout-bound, not cap-bound. For scale, the longest entry
	// in EXAMPLE_MARGINALIA.md is 34 characters.
	MARGINALIA: 160,
	NARRATIVE: 1000,
	LONG_TEXT: 5000,
	PASSWORD: 72,
} as const;
