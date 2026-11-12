// lawsRumorsSeen.ts — "is this law or rumor new to me?", in one place.
//
// The redesigned table page puts a pale dot on the Laws and Rumors sub-tabs, on
// the Public Record tab above them, and on each entry the player hasn't looked
// at yet; an edit counts as new. The server keeps a marker per player and list,
// the highest `rev` they have seen (migration 058), and each entry carries a
// `rev` that is drawn again on every edit. So an entry is new exactly when its
// rev is above the marker. It is the same shape as chat's unread marker
// (isUnreadPost in chatFeed.ts), without the "not authored by me" clause: the
// server catches an author's own marker up when they write or edit, and the
// client could not apply that clause for a rumor anyway, since a hidden source
// is null.
//
// Pure: no Svelte, no fetches. The caller holds two numbers per list, the marker
// it loaded (which decides the dots on screen) and what it last reported. It
// keeps showing dots against the loaded marker while the player is on the list
// and reports with seenThrough, so an entry doesn't lose its dot under their
// eyes (chat's unread divider does the same).

/** What this file needs of a law or rumor: Law and Rumor both satisfy it. */
export interface Revisioned {
	rev: number;
}

/** True when the player hasn't seen this entry in its current wording: it was
 *  created or edited after the marker. */
export function isNewEntry(entry: Revisioned, seenRev: number): boolean {
	return entry.rev > seenRev;
}

/** True when anything in the list is new: what the Laws or Rumors tab's dot
 *  shows, and, for the Public Record tab above them, `hasAnyNew` of either. */
export function hasNewEntries(entries: readonly Revisioned[], seenRev: number): boolean {
	return entries.some((e) => isNewEntry(e, seenRev));
}

/** How many entries are new. For the list's own words, never the tab bar: the
 *  redesign puts no numbers in the chrome. */
export function newEntryCount(entries: readonly Revisioned[], seenRev: number): number {
	return entries.reduce((n, e) => n + (isNewEntry(e, seenRev) ? 1 : 0), 0);
}

/** The marker to report once the player has had the list on screen: the highest
 *  rev shown, or null if there is nothing to report (an empty list, or one
 *  already at or below the marker, where a request would change nothing). The
 *  server clamps and never moves a marker back, so reporting more than needed
 *  is harmless; reporting less than shown would leave a dot behind. */
export function seenThrough(entries: readonly Revisioned[], seenRev: number): number | null {
	let top = seenRev;
	for (const e of entries) if (e.rev > top) top = e.rev;
	return top > seenRev ? top : null;
}
