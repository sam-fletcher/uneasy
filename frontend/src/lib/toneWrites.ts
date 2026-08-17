// toneWrites.ts — the client half of the tone-tile write path.
//
// Two problems live here, both of them the same bug seen from different ends:
// a rapidly-cycled tone tile used to repaint instantly and then visibly replay
// the whole cycle in slow motion, one step per round trip.
//
//  1. Writes were chained per topic, so tap N's PUT was not even sent until
//     tap N-1's response came back. But a tone PUT sets an *absolute* status,
//     not a delta, so only the value the player stopped on ever has to reach
//     the server. We keep at most one request per topic on the wire and hold
//     just the newest status behind it; the statuses cycled past are dropped.
//
//  2. UpdateToneTopic broadcasts tone.updated to every client *including the
//     sender*, and ToneUpdatedPayload (model/ws.go) carries no originating
//     player id — so a client cannot tell its own echo from another player's
//     edit. Applying every echo is what walked the tile backwards. While a
//     topic has a write in flight we drop its echoes; `seen` remembers which
//     statuses were ours so a genuine edit by someone else, arriving in that
//     window, still triggers one corrective refetch when the topic drains.
//
// Same shape as chatFeed.ts / ws-handlers.ts: plain functions over a context
// the component implements with $state-backed accessors, so the logic is
// unit-testable without a component harness.

import { updateToneTopic, listToneTopics } from '$lib/api';
import type { ToneTopic, ToneTopicStatus } from '$lib/api';

/** The order a tile cycles through on each tap. */
export const TONE_CYCLE: ToneTopicStatus[] = ['default', 'include', 'avoid_detail', 'never'];

export function nextToneStatus(current: ToneTopicStatus): ToneTopicStatus {
	return TONE_CYCLE[(TONE_CYCLE.indexOf(current) + 1) % TONE_CYCLE.length];
}

/** One topic's in-flight episode: from the first tap that finds the wire
 *  clear, until the last queued value has been acknowledged. */
export interface ToneWrite {
	/** The status currently on the wire. */
	sent: ToneTopicStatus;
	/** The newest status the player has cycled to since `sent` went out, or
	 *  undefined when they have stopped on `sent`. Each further tap
	 *  overwrites it — the values in between are deliberately never sent. */
	queued?: ToneTopicStatus;
	/** Every status this client has put on the wire this episode. Bounded by
	 *  the four cycle values, so it cannot grow with tap count. */
	seen: Set<ToneTopicStatus>;
	/** Set when a dropped echo carried a status we never sent — i.e. another
	 *  player edited this topic while we held the wire. Triggers one refetch
	 *  when the episode ends. */
	sawForeignEcho?: boolean;
}

export interface ToneWriteContext {
	readonly gameID: string | number;
	/** Paints one topic's status locally (optimistic update). */
	setStatus: (topicID: number, status: ToneTopicStatus) => void;
	/** Replaces the local topic list wholesale (resync). */
	replaceTopics: (topics: ToneTopic[]) => void;
	/** Surfaces a failure inside the Tones sheet. */
	setError: (message: string) => void;
	/** In-flight episodes keyed by topic id. Shared with ws-handlers.ts,
	 *  which reads it through acceptToneEcho. Deliberately a plain Map, not
	 *  reactive state: nothing renders from it. */
	readonly inFlight: Map<number, ToneWrite>;
}

/**
 * Decides whether an incoming tone.updated should be applied to local state,
 * recording (as a side effect) that a foreign edit was dropped so the write
 * loop can correct itself afterwards.
 *
 * Returns true whenever this client has nothing in flight for the topic —
 * which is every echo from every other player at any other time, and every
 * echo for a topic someone just added.
 */
export function acceptToneEcho(
	inFlight: Map<number, ToneWrite>,
	topicID: number,
	status: ToneTopicStatus
): boolean {
	const write = inFlight.get(topicID);
	if (!write) return true;
	// One of ours coming back. The tile already shows the value the player
	// stopped on; applying this would walk it back a step.
	if (write.seen.has(status)) return false;
	// Someone else moved this tile while our PUT was in flight. Dropping it
	// too, because our own write is about to overwrite it server-side and
	// painting it would flash a value nobody ends on — but the episode owes
	// the player a refetch once it drains.
	write.sawForeignEcho = true;
	return false;
}

/**
 * Advances one tone topic to the next status in the cycle: paints locally,
 * then coalesces the write as described at the top of this file.
 *
 * `current` is read by the caller off the live topic list rather than off a
 * render snapshot, since taps can outrun the array the tile was rendered from.
 */
export async function cycleToneStatus(
	ctx: ToneWriteContext,
	topicID: number,
	current: ToneTopicStatus
): Promise<void> {
	const next = nextToneStatus(current);

	// Recolour first, ask the server after. The tile used to change only when
	// the tone.updated broadcast came back, which is several sequential DB
	// round trips and two network hops away — and because `.tone-tile:active`
	// fires instantly, that gap read as the tile registering the tap and then
	// ignoring it.
	ctx.setStatus(topicID, next);
	ctx.setError('');

	const open = ctx.inFlight.get(topicID);
	if (open) {
		// A PUT for this tile is already on the wire. Keep only the newest
		// value; the episode below will send it when the wire clears.
		open.queued = next;
		return;
	}

	const write: ToneWrite = { sent: next, seen: new Set([next]) };
	ctx.inFlight.set(topicID, write);
	try {
		for (;;) {
			try {
				await updateToneTopic(ctx.gameID, topicID, write.sent);
			} catch (e) {
				ctx.setError(e instanceof Error ? e.message : 'Could not update topic.');
				// Resync rather than roll back. A rollback would be guesswork
				// once taps have been coalesced or another player's broadcast
				// has landed mid-flight; the server's list is the one answer
				// that is right in every case. Any value still queued behind
				// this failure is abandoned with it — the refetch, not the
				// player's last tap, is the truth now.
				await resyncTopics(ctx);
				return;
			}
			if (write.queued === undefined) break;
			write.sent = write.queued;
			write.queued = undefined;
			write.seen.add(write.sent);
		}
		if (write.sawForeignEcho) await resyncTopics(ctx);
	} finally {
		ctx.inFlight.delete(topicID);
	}
}

/** Best-effort refetch. If this fails too the player is offline, and either
 *  the error already shown or the next reconnect resync covers it. */
async function resyncTopics(ctx: ToneWriteContext): Promise<void> {
	try {
		ctx.replaceTopics((await listToneTopics(ctx.gameID)).topics);
	} catch { /* keep the optimistic value; the next resync corrects it */ }
}
