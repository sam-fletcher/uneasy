import { describe, it, expect } from 'vitest';
import { lastActiveLabel, reminderLine, activityFor } from './playerActivity';
import type { PlayerActivity } from '$lib/api';

const NOW = Date.parse('2026-08-16T12:00:00Z');
const ago = (ms: number) => new Date(NOW - ms).toISOString();
const ahead = (ms: number) => new Date(NOW + ms).toISOString();

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

describe('lastActiveLabel', () => {
	it('says "here now" for a live socket, whatever the timestamp says', () => {
		// The stored value lags by up to an hour, so a connected player can
		// easily look stale on paper. The socket is the better answer.
		const label = lastActiveLabel({ lastActiveAt: ago(3 * HOUR), online: true }, NOW);
		expect(label).toBe('here now');
	});

	it('says "here now" for a live socket that has never been recorded', () => {
		expect(lastActiveLabel({ lastActiveAt: null, online: true }, NOW)).toBe('here now');
	});

	it('distinguishes "never recorded" from "long ago"', () => {
		expect(lastActiveLabel({ lastActiveAt: null, online: false }, NOW)).toBe('not arrived yet');
	});

	it('stays vague inside the throttle window rather than inventing precision', () => {
		// The server writes at most hourly, so anything under ~1.5h could be
		// off by most of its own value. "recently" is the honest width.
		expect(lastActiveLabel({ lastActiveAt: ago(2 * MINUTE), online: false }, NOW)).toBe(
			'last here recently'
		);
		expect(lastActiveLabel({ lastActiveAt: ago(70 * MINUTE), online: false }, NOW)).toBe(
			'last here recently'
		);
	});

	it('reports hours within the first day', () => {
		expect(lastActiveLabel({ lastActiveAt: ago(3 * HOUR), online: false }, NOW)).toBe(
			'last here 3h ago'
		);
		expect(lastActiveLabel({ lastActiveAt: ago(23 * HOUR), online: false }, NOW)).toBe(
			'last here 23h ago'
		);
	});

	it('reports yesterday, then whole days, then gives up at a week', () => {
		expect(lastActiveLabel({ lastActiveAt: ago(30 * HOUR), online: false }, NOW)).toBe(
			'last here yesterday'
		);
		expect(lastActiveLabel({ lastActiveAt: ago(3 * DAY), online: false }, NOW)).toBe(
			'last here 3 days ago'
		);
		expect(lastActiveLabel({ lastActiveAt: ago(30 * DAY), online: false }, NOW)).toBe(
			'last here over a week ago'
		);
	});

	it('clamps a future timestamp instead of rendering negative time', () => {
		// Server/browser clock skew, not a bug worth crashing a header over.
		expect(lastActiveLabel({ lastActiveAt: ahead(HOUR), online: false }, NOW)).toBe(
			'last here recently'
		);
	});

	it('treats an unparseable timestamp as no record', () => {
		expect(lastActiveLabel({ lastActiveAt: 'not-a-date', online: false }, NOW)).toBe(
			'not arrived yet'
		);
	});
});

describe('reminderLine', () => {
	it('flags reminders-off as unreachable and names the remedy', () => {
		const line = reminderLine({ reminder: 'off', reminderDueAt: null }, NOW);
		expect(line).toEqual({
			text: 'Reminders off — reach them another way',
			unreachable: true
		});
	});

	it('flags the silent failure: a cadence set with no subscribed device', () => {
		const line = reminderLine({ reminder: 'no_device', reminderDueAt: null }, NOW);
		expect(line?.unreachable).toBe(true);
		expect(line?.text).toBe('No device set up for reminders');
	});

	it('reports a pending timer without claiming delivery', () => {
		const line = reminderLine(
			{ reminder: 'scheduled', reminderDueAt: ahead(6 * HOUR) },
			NOW
		);
		expect(line).toEqual({ text: 'Reminder due in ~6h', unreachable: false });
	});

	it('uses minutes under an hour and days beyond one', () => {
		expect(
			reminderLine({ reminder: 'scheduled', reminderDueAt: ahead(40 * MINUTE) }, NOW)?.text
		).toBe('Reminder due in ~40m');
		expect(
			reminderLine({ reminder: 'scheduled', reminderDueAt: ahead(3 * DAY) }, NOW)?.text
		).toBe('Reminder due in ~3d');
	});

	it('says "shortly" for an already-due timer rather than a negative one', () => {
		// The reconcile ticker runs each minute, so past-due means "about to
		// send", not "overdue".
		expect(
			reminderLine({ reminder: 'scheduled', reminderDueAt: ago(10 * MINUTE) }, NOW)?.text
		).toBe('Reminder due shortly');
	});

	it('reads as fine, not broken, in the gap before the ticker inserts a row', () => {
		const line = reminderLine({ reminder: 'ready', reminderDueAt: null }, NOW);
		expect(line).toEqual({ text: 'Reminders on', unreachable: false });
	});
});

describe('activityFor', () => {
	const rows: PlayerActivity[] = [
		{ player_id: 1, last_active_at: null, reminder: 'off', reminder_due_at: null },
		{ player_id: 2, last_active_at: ago(HOUR), reminder: 'ready', reminder_due_at: null }
	];

	it('finds a seat by id', () => {
		expect(activityFor(rows, 2)?.reminder).toBe('ready');
	});

	it('returns null for an unknown seat rather than guessing', () => {
		expect(activityFor(rows, 99)).toBeNull();
	});

	it('returns null when the payload never arrived', () => {
		expect(activityFor(undefined, 1)).toBeNull();
	});
});
