// lib/tableBell.ts — what the profile card's reminder bell shows, if anything.
//
// Pure over a MyTable so the rules are unit-testable away from the card's
// layout, and so "when does the bell exist?" is one readable answer rather than
// a chain of conditions inside the markup.

import type { MyTable } from '$lib/api';

export type TableBell = {
	/** Draw the struck variant — this table is currently sending nothing. */
	struck: boolean;
	/** Accessible name for the button. */
	label: string;
	/** Short line for the tooltip / title. */
	title: string;
	/** What a tap should ask the server for. Null when the bell is not a
	 *  control — an exhausted wait has already stopped on its own, and
	 *  offering to silence it would promise an effect it cannot have. */
	setMutedTo: boolean | null;
};

/**
 * The bell appears only while the table is waiting on this player, because
 * that is exactly when a reminder can be pending — the mute is per-wait, not
 * per-table (migration 056), so outside a wait there is nothing to silence and
 * nothing that could be silenced.
 *
 * That also means a mute never becomes invisible: it lasts precisely as long as
 * the wait whose card is showing it, and the reconciler clears it when the
 * table moves on.
 */
export function tableBell(t: MyTable): TableBell | null {
	if (t.phase === 'ended') return null;
	if (!t.waiting_on_player_ids.includes(t.player_id)) return null;

	// Their own choice outranks the give-up: both mean silence, but only one of
	// them is undoable from here, and that is the one worth offering.
	if (t.reminder_muted) {
		return {
			struck: true,
			label: `Turn reminders back on for table ${t.join_code}`,
			title: 'Reminders silenced for this turn',
			setMutedTo: false,
		};
	}
	if (t.reminder_exhausted) {
		return {
			struck: true,
			label: `Reminders have stopped for table ${t.join_code}`,
			title: "This table has been waiting so long that reminders stopped on their own",
			setMutedTo: null,
		};
	}
	return {
		struck: false,
		label: `Stop reminding me about table ${t.join_code}`,
		title: 'Stop reminding me about this turn',
		setMutedTo: true,
	};
}
