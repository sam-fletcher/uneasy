// Free-text length caps for `maxlength` attributes. Mirrors the tier
// constants in handler/helpers.go (maxUsernameLen, maxEmailLen,
// maxAssetNameLen, maxMarginaliaLen, maxNarrativeLen, maxLongTextLen) so
// honest users get browser-side feedback instead of a 400.
//
// A few fields intentionally cap stricter than these tiers (SuggestionPicker's
// 280 default, RetinueView's rename input at 80, SceneSetupForm's
// custom-location at 80, tone topics at 120) — those are deliberate local
// choices predating this file and are not meant to import from here.
export const TEXT_LIMITS = {
	USERNAME: 40,
	EMAIL: 254,
	NAME: 120,
	MARGINALIA: 300,
	NARRATIVE: 1000,
	LONG_TEXT: 5000,
} as const;
