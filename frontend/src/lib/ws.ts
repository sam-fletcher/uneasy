// ws.ts — WebSocket client with automatic reconnection.
//
// Usage:
//   const disconnect = createConnection(gameID, (msg) => { ... });
//   // later:
//   disconnect();

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
	PlanDelayedArrival: 'plan.delayed_arrival',

	// Simultaneous reveals
	RevealSubmitted: 'reveal.submitted',
	RevealComplete: 'reveal.complete',

	// Clandestinely Liaise
	LiaisePhaseChanged: 'liaise.phase_changed',
	LiaiseChoicesRevealed: 'liaise.choices_revealed',

	// Propose Duel
	DuelChampionElected: 'duel.champion_elected',
	DuelStakesRevealed: 'duel.stakes_revealed',
	DuelBoutResolved: 'duel.bout_resolved',
	DuelBoutsComplete: 'duel.bouts_complete',

	// Host Festivity
	FestivityGuestJoined: 'festivity.guest_joined',
	FestivityGuestRolled: 'festivity.guest_rolled',
	FestivityGuestChose: 'festivity.guest_chose',
	FestivityHostChose: 'festivity.host_chose',
	FestivityInsistHostMar: 'festivity.insist_host_mar',
	FestivityPhaseChanged: 'festivity.phase_changed',
	FestivityChallengeIssued: 'festivity.challenge_issued',
	FestivityChallengeDeclined: 'festivity.challenge_declined',
	FestivityDuelTriggered: 'festivity.duel_triggered',

	// Make War
	WarDeclared: 'war.declared',
	WarPlayerJoined: 'war.player_joined',
	WarBattleCostDue: 'war.battle_cost_due',
	WarBattleCostPaid: 'war.battle_cost_paid',
	WarPeaceProposed: 'war.peace_proposed',
	WarEnded: 'war.ended',

	// Make Demands
	DemandDraftPick: 'demand.draft_pick',
	DemandCounterPlaced: 'demand.counter_placed',

	// Laws & rumors (long-form narrative records)
	LawEnacted: 'law.enacted',
	LawUpdated: 'law.updated',
	RumorCreated: 'rumor.created',
	RumorUpdated: 'rumor.updated',

	// Dice rolls
	RollCreated: 'roll.created',
	RollLeverageAdded: 'roll.leverage_added',
	RollVoteCalled: 'roll.vote_called',
	RollVoteCast: 'roll.vote_cast',
	RollVoteResolved: 'roll.vote_resolved',
	RollResolved: 'roll.resolved',
} as const;

// createConnection opens a WebSocket to /api/tables/:id/ws and returns a
// cleanup function. Reconnects automatically with exponential backoff if the
// connection drops.
export function createConnection(gameID: string | number, onMessage: MessageHandler): () => void {
	let ws: WebSocket | null = null;
	let stopped = false;
	let retryDelay = 1000; // ms; doubles on each failed attempt, capped at 30s

	// Listen for typing events dispatched by the page component.
	function onTyping(e: Event) {
		const { typing } = (e as CustomEvent<{ typing: boolean }>).detail;
		send({ type: typing ? 'typing.start' : 'typing.stop' });
	}
	window.addEventListener('uneasy:typing', onTyping);

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
		};

		ws.onmessage = (event) => {
			let msg: WSMessage;
			try {
				msg = JSON.parse(event.data as string) as WSMessage;
			} catch {
				return;
			}
			onMessage(msg);
		};

		ws.onclose = () => {
			if (stopped) return;
			console.log(`[ws] disconnected — retrying in ${retryDelay}ms`);
			setTimeout(() => {
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

	return () => {
		stopped = true;
		window.removeEventListener('uneasy:typing', onTyping);
		ws?.close();
	};
}
