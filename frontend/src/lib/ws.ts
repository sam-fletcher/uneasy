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
// Returns { disconnect, ready }. Await `ready` to block until the first
// resync has finished (useful in onMount when you need state loaded before
// reading from it).
export function createConnection(
	gameID: string | number,
	onMessage: MessageHandler,
	onResync: () => Promise<void>,
): { disconnect: () => void; ready: Promise<void> } {
	let ws: WebSocket | null = null;
	let stopped = false;
	let retryDelay = 1000; // ms; doubles on each failed attempt, capped at 30s
	// Pending reconnect timer, so a returning player can pre-empt it (see
	// onVisibility below). Null whenever no retry is scheduled.
	let retryTimer: ReturnType<typeof setTimeout> | null = null;

	// While `syncing` is true, queue incoming events instead of dispatching
	// them. We flush the queue when the resync resolves.
	let syncing = false;
	let buffer: WSMessage[] = [];

	// `ready` resolves after the first successful resync, so onMount can
	// wait for initial state. Subsequent resyncs (after reconnects) don't
	// re-create this Promise — callers only care about the first one.
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
	}
	document.addEventListener('visibilitychange', onVisibility);

	function send(msg: object) {
		if (ws?.readyState === WebSocket.OPEN) {
			ws.send(JSON.stringify(msg));
		}
	}

	function connect() {
		if (stopped) return;

		const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${protocol}//${location.host}/api/tables/${gameID}/ws`;
		ws = new WebSocket(url);

		ws.onopen = () => {
			console.log('[ws] connected');
			retryDelay = 1000; // reset backoff on success

			// Start buffering before we kick off the resync, so that any
			// event arriving between now and the snapshot's arrival is held.
			syncing = true;
			buffer = [];

			// `.finally` so we release the buffer even if the resync
			// rejected — otherwise a transient fetch failure would freeze
			// the UI from ever applying live events again.
			onResync().finally(() => {
				syncing = false;
				// Drain the buffer in arrival order. JS is single-threaded,
				// so no new events can sneak in between these two lines.
				const pending = buffer;
				buffer = [];
				for (const msg of pending) onMessage(msg);

				if (!firstSyncDone) {
					firstSyncDone = true;
					resolveReady();
				}
			});
		};

		ws.onmessage = (event) => {
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

		ws.onclose = () => {
			if (stopped) return;
			console.log(`[ws] disconnected — retrying in ${retryDelay}ms`);
			retryTimer = setTimeout(() => {
				retryTimer = null;
				retryDelay = Math.min(retryDelay * 2, 30_000);
				connect();
			}, retryDelay);
		};

		ws.onerror = () => {
			// onerror is always followed by onclose, which handles reconnect.
			ws?.close();
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
			window.removeEventListener('uneasy:typing', onTyping);
			window.removeEventListener('uneasy:scene_setup_draft', onSceneSetupDraft);
			window.removeEventListener('uneasy:prepare_plan_draft', onPreparePlanDraft);
			document.removeEventListener('visibilitychange', onVisibility);
			ws?.close();
		},
		ready,
	};
}
