// lib/playerActivity.ts — the words for the Retinue header's two status
// lines: how long since a player had the table on screen, and whether a turn
// reminder can actually reach them.
//
// Pure functions over a supplied `now`, so both are unit-testable without
// faking timers. The components pass Date.now().

import type { PlayerActivity, ReminderState } from '$lib/api';

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/**
 * How long ago this player last had the table on screen.
 *
 * Buckets are coarse on purpose. The server throttles the write to at most
 * once an hour (playerActivityThrottle), so the stored value can lag the truth
 * by up to that much — the smallest bucket here is deliberately wider than the
 * throttle, so we never render more precision than we have. The error also
 * only ever runs one way (a stored timestamp is never *newer* than the real
 * visit), which is why "recently" rather than a specific sub-hour figure: it
 * absorbs the slack without ever overstating how present someone is.
 *
 * `online` short-circuits everything — a live WebSocket is a better answer to
 * "are they here?" than any timestamp, and it's the one case where the
 * present tense is honest.
 */
export function lastActiveLabel(
	input: { lastActiveAt: string | null; online: boolean },
	now: number
): string {
	if (input.online) return 'here now';
	if (!input.lastActiveAt) return 'not arrived yet';

	const then = Date.parse(input.lastActiveAt);
	if (Number.isNaN(then)) return 'not arrived yet';

	// A clock skew between server and browser can put the stored time slightly
	// in the future. Clamp rather than render "in -3 hours".
	const elapsed = Math.max(0, now - then);

	if (elapsed < 1.5 * HOUR) return 'last here recently';
	if (elapsed < DAY) return `last here ${Math.round(elapsed / HOUR)}h ago`;
	if (elapsed < 2 * DAY) return 'last here yesterday';
	if (elapsed < 7 * DAY) return `last here ${Math.floor(elapsed / DAY)} days ago`;
	return 'last here over a week ago';
}

/** What the reminder line says, or null when it should not be rendered. */
export type ReminderLine = {
	text: string;
	/** True for the two states that mean "no ping will reach them" — the
	 *  caller styles these as a warning and leaves the rest plain. */
	unreachable: boolean;
};

/**
 * Whether a turn reminder will reach this player, phrased for a tablemate
 * rather than for the player themselves.
 *
 * Only meaningful while the table is waiting on them, and the caller enforces
 * that (see RetinueView) — a persistent "reminders on" label on every seat
 * would be noise, and the answer only changes what anyone would *do* at the
 * moment someone owes the table a move.
 *
 * The copy never claims delivery. The server knows what is set up, not what an
 * OS chose to show or whether anyone looked (see model.ReminderState), so
 * "a reminder is due" is the strongest true statement available.
 */
export function reminderLine(
	input: { reminder: ReminderState; reminderDueAt: string | null },
	now: number
): ReminderLine | null {
	switch (input.reminder) {
		case 'off':
			return { text: 'Reminders off — reach them another way', unreachable: true };
		case 'no_device':
			return { text: 'No device set up for reminders', unreachable: true };
		case 'scheduled':
			return { text: `Reminder due ${duePhrase(input.reminderDueAt, now)}`, unreachable: false };
		case 'ready':
			// Reachable, but nothing pending yet — the reconcile ticker runs
			// once a minute, so this is the brief window after someone becomes
			// a waitee, and it shouldn't read as a problem.
			return { text: 'Reminders on', unreachable: false };
		default:
			return null;
	}
}

/** "in ~6h" / "in ~40m" / "shortly" for an already-due or imminent timer. */
function duePhrase(dueAt: string | null, now: number): string {
	if (!dueAt) return 'shortly';
	const due = Date.parse(dueAt);
	if (Number.isNaN(due)) return 'shortly';

	const remaining = due - now;
	// Past due means the ticker hasn't picked it up yet (it runs each minute),
	// so it is genuinely about to send rather than overdue in any real sense.
	if (remaining < 5 * MINUTE) return 'shortly';
	if (remaining < HOUR) return `in ~${Math.round(remaining / MINUTE)}m`;
	if (remaining < DAY) return `in ~${Math.round(remaining / HOUR)}h`;
	return `in ~${Math.round(remaining / DAY)}d`;
}

/** Finds one seat's entry. Returns null when the payload predates this
 *  feature or the fetch hasn't landed, so callers render nothing rather than
 *  guessing. */
export function activityFor(
	activity: PlayerActivity[] | undefined,
	playerID: number
): PlayerActivity | null {
	return activity?.find((a) => a.player_id === playerID) ?? null;
}
