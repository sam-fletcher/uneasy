package model

// Severity ranks chat-feed entries on a 0–100 ordered scale. Player messages
// always carry severity 0 (and are filtered separately by author_id, never by
// threshold). System posts pick the named tier that fits the event.
//
// The named tiers leave gaps so new tiers can slot in without renumbering.
// Filtering in the chat is a single threshold ("show severity >= N").
//
// See PUBLIC_RECORD_SIDEBAR_SPEC.md, Part 2.
const (
	SeverityPlayerMessage int32 = 0   // Player messages; never filtered by threshold.
	SeverityTrace         int32 = 10  // Reserved (text edits, marginalia tweaks).
	SeverityMinor         int32 = 25  // Routine state changes (asset refreshed).
	SeverityDefault       int32 = 50  // Notable player actions (plan cancelled).
	SeverityImportant     int32 = 75  // Outcomes (plan resolved, ranking update).
	SeverityBoundary      int32 = 100 // Structural anchors (row/scene/plan starts).
)
