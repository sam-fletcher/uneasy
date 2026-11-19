// ws.ts — WebSocket client with automatic reconnection + resync-on-connect.
//
// Usage:
//   const conn = createConnection(gameID, (msg) => { ... }, loadState);
//   await conn.ready;   // first resync done
//   // later:
//   conn.disconnect();

export interface WSMessage {
	type: string;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	payload: Record<string, any>;
}

type MessageHandler = (msg: WSMessage) => void;

// All known server→client event types for Phase 2.
export const EventTypes = {
	// Presence & typing (Phase 1, carried forward)
	PresenceSnapshot: 'presence.snapshot',
	TypingUpdate: 'typing.update',
	PlayerJoined: 'player.joined',

	// Phase transitions
	PhaseChanged: 'phase.changed',

	// Tone setting
	ToneUpdated: 'tone.updated',

	// Rankings & seating
	RankingsUpdated: 'rankings.updated',

	// Focus player & row advancement
	FocusChanged: 'focus.changed',
	RowAdvanced: 'row.advanced',
	SceneEnded: 'scene.ended',
	SceneStarted: 'scene.started',
	ScenePeerClaimed: 'scene.peer_claimed',
	// RowStateChanged carries the server-authoritative RowState (which
	// rulebook step the row is in). The client renders directly off this
	// instead of inferring from focus/scene/plan events. See lib/api/tables.ts
	// for the RowState type and routes/table/[id]/+page.svelte for the
	// handler.
	RowStateChanged: 'row_state.changed',

	// Scene posts & entries (replaces post.created)
	ScenePostCreated: 'scene_post.created',
	SceneEntryCreated: 'scene_entry.created',

	// Assets & marginalia
	AssetCreated: 'asset.created',
	AssetUpdated: 'asset.updated',
	AssetTaken: 'asset.taken',
	AssetLeveraged: 'asset.leveraged',
	AssetRefreshed: 'asset.refreshed',
	AssetDestroyed: 'asset.destroyed',
	MarginaliaAdded: 'marginalia.added',
	MarginaliaUpdated: 'marginalia.updated',
	MarginaliaTorn: 'marginalia.torn',

	// Plans
	PlanPrepared: 'plan.prepared',
	PlanResolving: 'plan.resolving',
	PlanResolved: 'plan.resolved',
	PlanChoiceApplied: 'plan.choice_applied',
	PlanUpdated: 'plan.updated',
	PlanDelayedArrival: 'plan.delayed_arrival',

	// Simultaneous reveals
	RevealSubmitted: 'reveal.submitted',
	RevealComplete: 'reveal.complete',

	// Clandestinely Liaise
	LiaisePhaseChanged: 'liaise.phase_changed',
	LiaiseChoicesRevealed: 'liaise.choices_revealed',
	LiaiseKeepSecretSubmitted: 'liaise.keep_secret_submitted',

	// Propose Decree
	DecreeCouncilJoined: 'decree.council_joined',
	DecreeCouncilDeclined: 'decree.council_declined',
	DecreeDebateStarted: 'decree.debate_started',

	// Propose Duel
	DuelChampionElected: 'duel.champion_elected',
	DuelStakesSelected: 'duel.stakes_selected',
	DuelBoutDeclared: 'duel.bout_declared',
	DuelBoutResolved: 'duel.bout_resolved',
	DuelBoutsComplete: 'duel.bouts_complete',

	// Host Festivity
	FestivityGuestRolled: 'festivity.guest_rolled',
	FestivityGuestChose: 'festivity.guest_chose',
	FestivityHostChose: 'festivity.host_chose',
	FestivityInsistHostMar: 'festivity.insist_host_mar',
	FestivityUpdated: 'festivity.updated',
	FestivityChallengeIssued: 'festivity.challenge_issued',
	FestivityChallengeDeclined: 'festivity.challenge_declined',
	FestivityDuelTriggered: 'festivity.duel_triggered',

	// Make War
	WarDeclared: 'war.declared',
	WarPlayerJoined: 'war.player_joined',
	WarBattleCostDue: 'war.battle_cost_due',
	WarBattleCostPaid: 'war.battle_cost_paid',
	WarPlayerSurrendered: 'war.player_surrendered',
	WarAssetSeized: 'war.asset_seized',
	WarEntryCompleted: 'war.entry_completed',
	WarPeaceProposed: 'war.peace_proposed',
	WarPeaceVote: 'war.peace_vote',
	WarEnded: 'war.ended',

	// Make Demands
	DemandPrepared: 'demand.prepared',
	DemandResolved: 'demand.resolved',
	DemandDraftPick: 'demand.draft_pick',
	DemandCounterPending: 'demand.counter_pending',
	DemandCounterPlaced: 'demand.counter_placed',
	DemandLeverageSet: 'demand.leverage_set',
	DemandRetargeted: 'demand.retargeted',

	// Laws & rumors (long-form narrative records)
	LawEnacted: 'law.enacted',
	LawUpdated: 'law.updated',
	RumorCreated: 'rumor.created',
	RumorUpdated: 'rumor.updated',
	RumorTakeConsentRequested: 'rumor.take_consent_requested',
	RumorTakeConsentResolved: 'rumor.take_consent_resolved',

	// Secrets
	SecretCreated: 'secret.created',
	SecretVisibilityGrant: 'secret.visibility_grant',

	// Shake-Up (Phase 4c)
	ShakeUpStepChanged: 'shake_up.step_changed',
	ShakeUpRolled: 'shake_up.rolled',
	ShakeUpSpendOpened: 'shake_up.spend_opened',
	ShakeUpAdjusted: 'shake_up.adjusted',
	ShakeUpSpendCommitted: 'shake_up.spend_committed',
	ShakeUpSpendAbandoned: 'shake_up.spend_abandoned',
	ShakeUpPassed: 'shake_up.passed',
	ShakeUpEnded: 'shake_up.ended',

	// Endgame mode selection (Phase 4d)
	EndgameModeSet: 'endgame.mode_set',
	// One cast or changed vote in the row 7 → 8 ending-mode vote. Votes are
	// public, so the payload names the player and the mode. The vote *opening*
	// has no event of its own — the row_state.changed the blocked advance
	// already broadcasts (kind await_endgame_vote) is the signal.
	EndgameVoteCast: 'endgame.vote_cast',

	// Ephemeral: focus player's in-flight scene-setup selections
	SceneSetupDraft: 'scene_setup.draft',
	// Ephemeral: focus player's currently-highlighted plan card during prep
	PreparePlanDraft: 'prepare_plan.draft',

	// Structured prologue (Phase 4b)
	PrologueChoiceClaimed: 'prologue.choice_claimed',
	PrologueTurnAdvanced: 'prologue.turn_advanced',
	PrologueRankingStepChanged: 'prologue.ranking_step_changed',
	PrologueTrackRanked: 'prologue.track_ranked',
	PrologueSetAsidesPlaced: 'prologue.set_asides_placed',
	PrologueCommittedHeartsChanged: 'prologue.committed_hearts_changed',
	PrologueDoneChanged: 'prologue.done_changed',
	PrologueExtraPeerCreated: 'prologue.extra_peer_created',
	PrologueClosingReadyChanged: 'prologue.closing_ready_changed',

	// Dice rolls
	RollCreated: 'roll.created',
	RollLeverageAdded: 'roll.leverage_added',
	RollVoteCast: 'roll.vote_cast',
	RollVoteResolved: 'roll.vote_resolved',
	RollResolved: 'roll.resolved',
	RollStageChanged: 'roll.stage_changed',
	RollIntentSet: 'roll.intent_set',
	RollReadyChanged: 'roll.ready_changed',
} as const;

// Grouped event names (without the `uneasy:` prefix) for window-event
// subscriptions. Pair with `useWindowEvents` from $lib/useWindowEvents.
export const WAR_EVENTS = [
	'war.declared', 'war.player_joined', 'war.battle_cost_due',
	'war.battle_cost_paid', 'war.player_surrendered', 'war.asset_seized',
	'war.entry_completed', 'war.peace_proposed', 'war.peace_vote',
	'war.ended',
] as const;

export const REVEAL_EVENTS = [
	'reveal.submitted', 'reveal.complete',
] as const;

export const DEMAND_EVENTS = [
	'demand.prepared', 'demand.resolved', 'demand.draft_pick',
	'demand.counter_pending', 'demand.counter_placed',
	'demand.leverage_set', 'demand.retargeted',
] as const;

// createConnection opens a WebSocket to /api/tables/:id/ws.
//
// On every (re)connection it calls `onResync`, which should refetch the full
// game state. While that fetch is in flight, incoming WS events are buffered
// and then replayed in order once the snapshot is applied. This preserves
// two invariants:
//
//   1. No event is dropped — even on the initial connect, or after a brief
//      disconnect (server restart, HMR, screen lock, network blip), the
//      snapshot brings us back to a known-good baseline.
//   2. No event is clobbered by a stale snapshot — the snapshot reflects
//      server state at some moment T; any event arriving while the snapshot
//      is in flight is newer than T, so we replay it *on top of* the
//      snapshot rather than letting the snapshot overwrite it.
//
// `onResync` must say whether the snapshot landed: it resolves true when the
// board has been replaced, and false (or rejects) when it could not be. The
// caller has to say, because a resync that fails leaves the old board on screen
// and nothing here can see that. A failed resync closes its socket, so the
// retry loop below connects again and resyncs again, and the open socket does
// not reset the backoff: a server that accepts sockets but fails the fetch is
// retried at the growing delay, not every second.
//
// Returns { disconnect, ready, retryNow }. Await `ready` to block until the
// first resync has settled, whether or not it succeeded (useful in onMount when
// you need state loaded before reading from it; a failure retries by itself).
//
// ── Telling the player the table has gone quiet ──────────────────────────
// The retry loop below heals most drops without anyone noticing, and for those
// the right amount of UI is none. The ones worth a notice are the drops that
// last: a laptop that slept, a phone in a tunnel, a server that is down. For
// those `options.onStatus` is called with 'down' once the table has been
// without a *live* connection for DISCONNECT_NOTICE_DELAY_MS, and with 'live'
// when it is back.
//
// "Live" means the socket is open AND its resync has succeeded, not merely that
// the socket opened. The notice says "this table isn't updating", and until
// the snapshot is applied that is still true: the board on screen is the one
// from before the drop. So when the notice goes away the board is current, and
// a player who sees no notice can trust what they see. (A hole this doesn't
// close: a socket the browser still thinks is open after the network has
// silently gone, with no close event, is never marked down. See
// docs/work/next/ws-heartbeat.md.)
export type ConnectionStatus = 'live' | 'down';

export interface ConnectionOptions {
	/** Called with 'down' after DISCONNECT_NOTICE_DELAY_MS without a live
	 *  connection, then with 'live' when it recovers. Never called for a drop
	 *  that heals sooner, and never called after disconnect(). */
	onStatus?: (status: ConnectionStatus) => void;
	/** Called when the player comes back to this page while the connection is
	 *  live: the tab becomes visible, or its window gains focus. For work that
	 *  events can't do, chiefly looking again at state the server keeps per
	 *  player and never pushes (the "seen" markers: another device may have moved
	 *  them). Not called when a reconnect is already under way, because its
	 *  resync reads everything; not called before the first resync has landed;
	 *  and at most once per FOREGROUND_MIN_GAP_MS, so one return that raises both
	 *  events (or a player flicking between apps) costs one call. A throw or a
	 *  rejection is swallowed: whatever this does is best effort. */
	onForeground?: () => void | Promise<void>;
}

/** The shortest gap between two onForeground calls. Returning to a tab raises
 *  `visibilitychange`, and on a desktop usually `focus` as well, within
 *  milliseconds of each other; this folds the pair into one. Short on purpose:
 *  a longer window would hide a real second return (read Laws on the laptop,
 *  pick up the phone, put it down and pick it up again), and the call it guards
 *  is one small request. */
export const FOREGROUND_MIN_GAP_MS = 2_000;

/** How long the table may go without a live connection before it says so.
 *
 *  The retry schedule is 1s, then 2s, then 4s after the drop, so retries start
 *  at about 1s and 3s. A reconnect on this schedule isn't instant: the
 *  handshake and the resync (several requests, ~0.4s each against production)
 *  take 1–2s after the retry fires. A recovery on the first retry therefore
 *  lands at about 2–3s, comfortably inside five seconds, which covers a server
 *  restart, a wifi handoff, and the page waking from a lock screen (which
 *  reconnects at once). A recovery on the *second* retry lands at about
 *  4–5s, right at the edge: on production it can still show the notice for a
 *  moment. Whether to move the edge out to 6s is the owner's call
 *  (docs/work/questions/connection-notice-look.md). A drop that survives two
 *  retries is not a blip, and the notice then stays up until a later retry
 *  (7s, 15s…) or the player's "Try now" brings the table back. It is short
 *  next to what it guards against, a board that has been wrong for hours. */
export const DISCONNECT_NOTICE_DELAY_MS = 5_000;

export function createConnection(
	gameID: string | number,
	onMessage: MessageHandler,
	onResync: () => Promise<boolean>,
	options: ConnectionOptions = {},
): { disconnect: () => void; ready: Promise<void>; retryNow: () => Promise<void> } {
	let ws: WebSocket | null = null;
	let stopped = false;
	let retryDelay = 1000; // ms; doubles on each failed attempt, capped at 30s
	// Pending reconnect timer, so a returning player can pre-empt it (see
	// onVisibility below). Null whenever no retry is scheduled.
	let retryTimer: ReturnType<typeof setTimeout> | null = null;

	// Live = open socket whose resync succeeded (see the note above
	// createConnection). It starts false: the first connect is an attempt too,
	// so a table that can't be reached on first load is told about the same way
	// as one that dropped later.
	let live = false;
	// Armed whenever we are not live; fires once to report 'down'. At most one
	// is pending, and it is not re-armed by each failed retry, so the delay is
	// measured from the drop and not from the latest attempt.
	let downTimer: ReturnType<typeof setTimeout> | null = null;
	let reportedDown = false;
	// Callers of retryNow() waiting to hear how that attempt went.
	let attemptWaiters: Array<() => void> = [];

	function settleAttempts() {
		const waiting = attemptWaiters;
		attemptWaiters = [];
		for (const done of waiting) done();
	}
	function markLive() {
		live = true;
		if (downTimer !== null) {
			clearTimeout(downTimer);
			downTimer = null;
		}
		if (reportedDown) {
			reportedDown = false;
			options.onStatus?.('live');
		}
		settleAttempts();
	}
	function markNotLive() {
		live = false;
		if (downTimer === null && !reportedDown && !stopped) {
			downTimer = setTimeout(() => {
				downTimer = null;
				reportedDown = true;
				options.onStatus?.('down');
			}, DISCONNECT_NOTICE_DELAY_MS);
		}
		settleAttempts();
	}
	// The first attempt starts the clock, before connect() below.
	markNotLive();

	// While `syncing` is true, queue incoming events instead of dispatching
	// them. We flush the queue when the resync resolves.
	let syncing = false;
	let buffer: WSMessage[] = [];

	// `ready` resolves once the first resync has settled, so onMount can wait
	// for initial state. It settles on failure too, deliberately: a caller
	// that waited forever on a failed first load would never get to show its
	// error, and the failure now retries by itself. Subsequent resyncs (after
	// reconnects) don't re-create this Promise — callers only care about the
	// first one.
	let firstSyncDone = false;
	let resolveReady!: () => void;
	const ready = new Promise<void>((resolve) => { resolveReady = resolve; });

	// Listen for typing events dispatched by the page component.
	function onTyping(e: Event) {
		const { typing } = (e as CustomEvent<{ typing: boolean }>).detail;
		send({ type: typing ? 'typing.start' : 'typing.stop' });
	}
	window.addEventListener('uneasy:typing', onTyping);

	// Listen for scene-setup draft snapshots dispatched by SceneSetupForm.
	// The payload is forwarded verbatim; the server stamps player_id.
	function onSceneSetupDraft(e: Event) {
		const detail = (e as CustomEvent<Record<string, unknown>>).detail;
		send({ type: 'scene_setup.draft', payload: detail });
	}
	window.addEventListener('uneasy:scene_setup_draft', onSceneSetupDraft);

	// Listen for prepare-plan draft snapshots dispatched by PlanPanel.
	function onPreparePlanDraft(e: Event) {
		const detail = (e as CustomEvent<Record<string, unknown>>).detail;
		send({ type: 'prepare_plan.draft', payload: detail });
	}
	window.addEventListener('uneasy:prepare_plan_draft', onPreparePlanDraft);

	// A backgrounded tab's socket usually dies — laptop sleep, phone lock, an
	// OS suspending the connection — and by the time the player looks again
	// the backoff may have grown to its 30s cap, so the table sits stale in
	// front of them. Treat "the page became visible" as a human waiting:
	// reset the backoff and pre-empt any scheduled retry.
	//
	// This is deliberately not a shorter global cap. Retries nobody is
	// watching buy nothing, and every reconnect that *succeeds* runs a full
	// resync (game state, assets, rolls, record) — real DB work on a
	// metered free tier. Spending it when someone is looking is the trade
	// worth making; spending it every 5s into a sleeping tab is not.
	function onVisibility() {
		if (stopped || document.visibilityState !== 'visible') return;
		retryDelay = 1000;
		// Only when a retry is actually pending: if the socket is OPEN
		// there's nothing to do, and if it's CONNECTING an attempt is
		// already in flight — connecting again would leak a second socket.
		if (retryTimer !== null) {
			clearTimeout(retryTimer);
			retryTimer = null;
			connect();
		}
		// After a reconnect above this is a no-op: the connection is not live
		// until the new socket's resync lands, and that resync re-reads everything,
		// the per-player markers included. (A pending retry always means "not
		// live": the retry is only ever scheduled from `close`.)
		notifyForeground();
	}
	document.addEventListener('visibilitychange', onVisibility);

	// A window that was already visible and gains focus (two windows side by
	// side, a second monitor) raises `focus` and not `visibilitychange`, and is
	// just as much "the player came back". It only asks for the foreground call:
	// reconnecting stays tied to the page becoming visible, as before.
	function onFocus() {
		if (stopped) return;
		notifyForeground();
	}
	window.addEventListener('focus', onFocus);

	// What the page wants done when the player returns. Only on a live
	// connection: a socket that is down, still connecting or mid-resync is about
	// to run (or is running) a resync that reads everything, so a second read
	// would only race it. Folded to one call per FOREGROUND_MIN_GAP_MS.
	let lastForegroundAt = -Infinity;
	function notifyForeground() {
		if (!options.onForeground || !live) return;
		const now = Date.now();
		if (now - lastForegroundAt < FOREGROUND_MIN_GAP_MS) return;
		lastForegroundAt = now;
		try {
			// Best effort by contract: neither a throw nor a rejection may reach
			// the event dispatch that called us.
			void Promise.resolve(options.onForeground()).catch(() => {});
		} catch {
			/* see above */
		}
	}

	// The "Try now" button's action: a human asking for the retry that
	// onVisibility gives a returning one. It makes the same two moves (reset
	// the backoff, pre-empt a pending retry) for the same reason, and it is
	// written out here rather than shared with onVisibility so that function
	// stays free to change on its own.
	//
	// Resolves once that attempt has settled: the table is live again, or the
	// attempt failed (or was already in flight and failed), so the button can
	// show "Trying…" for exactly that long instead of looking dead when the
	// network is still down. Resolves at once if the table is already live.
	function retryNow(): Promise<void> {
		if (stopped || live) return Promise.resolve();
		const settled = new Promise<void>((resolve) => attemptWaiters.push(resolve));
		retryDelay = 1000;
		// Connect now if a retry is pending (pre-empting it), for the reason
		// onVisibility gives: an attempt already in flight must not be doubled.
		// Its outcome still settles `settled`.
		//
		// Also connect when the socket is CLOSED and no retry is pending. That
		// is a loop that has died: the loop only continues from onclose, and a
		// socket the browser failed without ever firing `close` (a connection
		// blocked outright, say) never reaches it. Left alone, "Try now" would
		// wait for an event that is not coming and read "Trying…" forever, so
		// the button restarts the loop. (CLOSING is not that: its close event is
		// still due, and will schedule the retry itself.)
		const loopDied = retryTimer === null && ws !== null && ws.readyState === WebSocket.CLOSED;
		if (retryTimer !== null || loopDied) {
			if (retryTimer !== null) clearTimeout(retryTimer);
			retryTimer = null;
			connect();
		}
		return settled;
	}

	function send(msg: object) {
		if (ws?.readyState === WebSocket.OPEN) {
			ws.send(JSON.stringify(msg));
		}
	}

	function connect() {
		if (stopped) return;

		const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${protocol}//${location.host}/api/tables/${gameID}/ws`;
		const sock = new WebSocket(url);
		ws = sock;

		// Every handler below ignores a socket that is no longer `ws`. connect()
		// replaces `ws` while the old socket may still have events coming: a socket
		// the browser reports CLOSED can fire its `close` a moment later, and
		// "Try now" can reconnect in that gap. Without the check that late close
		// would schedule a retry beside the new, live socket (two sockets, every
		// event twice), and the old socket's resync would flip `syncing` under the
		// new one's.
		sock.onopen = () => {
			if (sock !== ws) {
				sock.close();
				return;
			}
			console.log('[ws] connected');
			// No backoff reset here: an open socket proves nothing until its
			// resync has landed (below). A server that accepts sockets but fails
			// the fetch must be retried at the growing delay, not every second.

			// Start buffering before we kick off the resync, so that any
			// event arriving between now and the snapshot's arrival is held.
			syncing = true;
			buffer = [];

			// A rejection counts as a failed resync, the same as `false`: either
			// way the snapshot did not land.
			onResync()
				.then((landed) => landed === true, () => false)
				.then((landed) => {
					// A newer socket has taken over; its own resync owns the buffer
					// and the flag now.
					if (sock !== ws) return;

					if (landed) {
						syncing = false;
						// The snapshot is applied, so the board is current. Only if
						// the socket is still open: if it dropped while the fetch was in
						// flight, onclose has already marked us down and a late resync
						// must not claim otherwise.
						if (sock.readyState === WebSocket.OPEN) {
							retryDelay = 1000; // reset the backoff on a real success
							markLive();
						}
						// Drain the buffer in arrival order. JS is single-threaded,
						// so no new events can sneak in between these two lines.
						const pending = buffer;
						buffer = [];
						for (const msg of pending) onMessage(msg);
					} else {
						// The old board is still on screen, so nothing may claim it is
						// current. The buffered events were newer than a snapshot we
						// never got: applying them to the old board would half-update
						// it, and the next snapshot covers them. `syncing` stays true so
						// anything arriving while this socket closes is held and then
						// discarded by the next onopen. Closing the socket is what makes
						// the loop below connect and resync again.
						buffer = [];
						sock.close();
					}

					// Settles on failure too (see `ready` above).
					if (!firstSyncDone) {
						firstSyncDone = true;
						resolveReady();
					}
				});
		};

		sock.onmessage = (event) => {
			if (sock !== ws) return;
			let msg: WSMessage;
			try {
				msg = JSON.parse(event.data as string) as WSMessage;
			} catch {
				return;
			}
			if (syncing) {
				buffer.push(msg);
			} else {
				onMessage(msg);
			}
		};

		sock.onclose = () => {
			if (stopped || sock !== ws) return;
			markNotLive();
			console.log(`[ws] disconnected — retrying in ${retryDelay}ms`);
			retryTimer = setTimeout(() => {
				retryTimer = null;
				retryDelay = Math.min(retryDelay * 2, 30_000);
				connect();
			}, retryDelay);
		};

		sock.onerror = () => {
			// onerror is always followed by onclose, which handles reconnect.
			sock.close();
		};
	}

	connect();

	return {
		disconnect: () => {
			stopped = true;
			if (retryTimer !== null) {
				clearTimeout(retryTimer);
				retryTimer = null;
			}
			// A player leaving the page is not a dropped connection, so no 'down'
			// may fire after this. Anyone awaiting retryNow() is released.
			if (downTimer !== null) {
				clearTimeout(downTimer);
				downTimer = null;
			}
			settleAttempts();
			window.removeEventListener('uneasy:typing', onTyping);
			window.removeEventListener('uneasy:scene_setup_draft', onSceneSetupDraft);
			window.removeEventListener('uneasy:prepare_plan_draft', onPreparePlanDraft);
			document.removeEventListener('visibilitychange', onVisibility);
			window.removeEventListener('focus', onFocus);
			ws?.close();
		},
		ready,
		retryNow,
	};
}
