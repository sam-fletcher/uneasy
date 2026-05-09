// Severity tiers for chat-feed entries. Mirrors model/severity.go on the
// server. Player messages always carry PLAYER_MESSAGE (0) and are filtered
// separately by author_id, never by threshold.
//
// See PUBLIC_RECORD_SIDEBAR_SPEC.md, Part 2.
export const SEVERITY = {
	PLAYER_MESSAGE: 0,
	TRACE: 10,
	MINOR: 25,
	DEFAULT: 50,
	IMPORTANT: 75,
	BOUNDARY: 100,
} as const;

export type SeverityLevel = typeof SEVERITY[keyof typeof SEVERITY];
