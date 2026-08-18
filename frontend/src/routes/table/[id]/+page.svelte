<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/modalShell.css';
	import '$lib/components/shared/rankChip.css';
	import '$lib/components/shared/statusText.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, onDestroy, tick } from 'svelte';
	import {
		getGameState, getMe, touchActivity,
		addToneTopic,
		listAssets, getFullRecord,
		getActiveRollForGame, listBankedDice,
		listPlans, listPlanTokens,
		getVisibleSecrets,
		getActiveScene,
		type Scene,
		type ScenePeerView,
		type SceneSetupDraft,
		type PreparePlanDraft,
		type RowState,
		type AnchorRequest,
		type Account,
	} from '$lib/api';
	import { createConnection, type WSMessage } from '$lib/ws';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { handleWSMessage as runWSMessage, type WSContext } from './ws-handlers';
	import {
		reconnectResync, resolveAnchor, enterHistoryMode, type ChatFeedContext,
	} from '$lib/chatFeed';
	import {
		cycleToneStatus, type ToneWrite, type ToneWriteContext,
	} from '$lib/toneWrites';
	import type {
		Game, Player, ToneTopic, Ranking, Asset, Marginalium,
		Law, Rumor,
		ChatPost, SceneEntry, RecordRow, PresenceMember, PlayerActivity,
		DiceRoll, DiceRollDie, VoteView, RollParticipant, BankedDie,
		Plan, PlanToken, Secret,
	} from '$lib/api';
	import MainEventView from '$lib/components/phases/MainEventView.svelte';
	import PublicRecord from '$lib/components/PublicRecord.svelte';
	import LobbyView from '$lib/components/phases/LobbyView.svelte';
	import PrologueView from '$lib/components/phases/PrologueView.svelte';
	import ShakeUpView from '$lib/components/phases/ShakeUpView.svelte';
	import EndedView from '$lib/components/phases/EndedView.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import LawsRumors from '$lib/components/LawsRumors.svelte';
	import RetinueView from '$lib/components/RetinueView.svelte';
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import HelpButton from '$lib/components/HelpButton.svelte';
	import FeedbackForm from '$lib/components/FeedbackForm.svelte';
	import WaitingOnBar, { type WaitingOnState } from '$lib/components/WaitingOnBar.svelte';
	import PhaseBadge from '$lib/components/shared/PhaseBadge.svelte';
	import { playerColorByID } from '$lib/playerColor';
	import { warDrawerOpen, activeWarCount, pendingWarCount } from '$lib/warDrawer';
	import { provideSecretCounts } from '$lib/secretCountsContext';
	import { provideSuccession } from '$lib/successionContext';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import {
		rankTriplesByPlayer, topRanks, atRiskCountByPlayer, typingIndicatorLabel,
	} from '$lib/tableHeader';

	const gameID = $derived(page.params.id as string);

	// ── Core state ────────────────────────────────────────────────────────────
	let game = $state<Game | null>(null);
	let players = $state<Player[]>([]);
	let toneTopics = $state<ToneTopic[]>([]);
	let rankings = $state<Ranking[]>([]);
	let assets = $state<Asset[]>([]);
	let laws = $state<Law[]>([]);
	let rumors = $state<Rumor[]>([]);
	let members = $state<PresenceMember[]>([]);
	// Durable per-seat presence ("last here 3h ago") and reminder reachability.
	// Distinct from `members`, which is live socket state: this survives a
	// redeploy and answers "has anyone been here since?", which is the question
	// a stalled async game actually raises.
	let playerActivity = $state<PlayerActivity[]>([]);
	let secrets = $state<Secret[]>([]);
	let currentPlayerID = $state<number | null>(null);
	// Load errors only — the reason the page has no data, or stale data. It is
	// sticky (it explains an empty screen), it renders under the header where
	// the missing content would be, and it is cleared *only* by a successful
	// load. Action failures get their own state next to the control that
	// raised them; they must never land here. See adr/ERROR_HANDLING_PLAN.md:
	// this used to be one variable for both, so the resync clear below wiped
	// action errors seconds after they appeared, and action errors rendered
	// under the header — behind whichever modal had raised them.
	let loadError = $state('');
	let loading = $state(true);

	// Derived helpers
	// One consumer, deliberately: the lobby, where the facilitator is the only
	// player who can start the prologue. The flag reaches no other view — it
	// gates nothing else a player can see, and the app names the facilitator in
	// prose where it matters (the lobby verdict, the ending vote's tie-break)
	// rather than tagging their seat (owner, 2026-08-16).
	const isFacilitator = $derived(
		currentPlayerID != null && players.some(p => p.id === currentPlayerID && p.is_facilitator)
	);

	// ── Typing indicators ─────────────────────────────────────────────────────
	let typingNames = $state<string[]>([]);
	let typingMap = new Map<number, string>();
	let typingTimeouts = new Map<number, ReturnType<typeof setTimeout>>();

	const typingLabel = $derived(typingIndicatorLabel(typingNames));

	// ── Public record + unified chat feed ─────────────────────────────────────
	let recordRows = $state<RecordRow[]>([]);

	// Chat Overhaul Phase 2: the feed's state lives here (owned by the page,
	// like every other piece of table state), wired into a ChatFeedContext
	// (see $lib/chatFeed) whose fields are get/set accessors over these runes
	// — the same pattern ws-handlers.ts uses for the rest of WSContext. Pass
	// `chatFeed` itself down to ChatPanel and into wsCtx rather than threading
	// each field as a separate prop.
	let chatFeedPosts = $state<ChatPost[]>([]);
	let chatFeedMode = $state<'live' | 'history'>('live');
	let chatHasMoreBefore = $state(false);
	let chatHasMoreAfter = $state(false);
	let chatLastReadPostID = $state(0);
	let chatInitialReadMarker = $state(0);
	let chatLoadingOlder = $state(false);
	const chatFeed: ChatFeedContext = {
		get gameID() { return gameID; },
		get posts() { return chatFeedPosts; }, set posts(v) { chatFeedPosts = v; },
		get mode() { return chatFeedMode; }, set mode(v) { chatFeedMode = v; },
		get hasMoreBefore() { return chatHasMoreBefore; }, set hasMoreBefore(v) { chatHasMoreBefore = v; },
		get hasMoreAfter() { return chatHasMoreAfter; }, set hasMoreAfter(v) { chatHasMoreAfter = v; },
		get lastReadPostID() { return chatLastReadPostID; }, set lastReadPostID(v) { chatLastReadPostID = v; },
		get initialReadMarker() { return chatInitialReadMarker; }, set initialReadMarker(v) { chatInitialReadMarker = v; },
		get loadingOlder() { return chatLoadingOlder; }, set loadingOlder(v) { chatLoadingOlder = v; },
	};

	// ── Row state ─────────────────────────────────────────────────────────────
	// Server-authoritative "which step of the row are we in?" — see
	// model/row_state.go. The client never infers this from individual
	// events; it is set from the snapshot at load time and updated by
	// row_state.changed events. While loading, null; outside main_event
	// the server sends kind='phase_not_main_event'.
	let rowState = $state<RowState | null>(null);

	// ── Scene state (SCENES_PLAN.md) ──────────────────────────────────────────
	// activeScene is the currently-running scene (location/time/peers), or
	// null between scenes. Loaded on mount and kept in sync via WS events.
	let activeScene = $state<Scene | null>(null);
	let activeScenePeers = $state<ScenePeerView[]>([]);

	// Ephemeral mirror of the focus player's in-flight scene-setup
	// selections, fanned out so non-focus players can watch the form fill
	// in. Reset on scene start so a stale draft doesn't reappear next round.
	let sceneSetupDraft = $state<SceneSetupDraft | null>(null);

	// Ephemeral mirror of the focus player's currently-highlighted plan card
	// during the post-scene prep step. Cleared when a plan is prepared or
	// the row advances, so a stale highlight doesn't reappear next turn.
	let preparePlanDraft = $state<PreparePlanDraft | null>(null);

	async function refreshActiveScene() {
		if (!game || game.phase !== 'main_event') {
			activeScene = null;
			activeScenePeers = [];
			return;
		}
		try {
			const data = await getActiveScene(gameID);
			activeScene = data.scene;
			activeScenePeers = data.peers;
		} catch {
			activeScene = null;
			activeScenePeers = [];
		}
	}

	// ── Dice roll state ───────────────────────────────────────────────────────
	// activeRoll is the current unresolved dice roll for this game (or null).
	// It's set by roll.created WS events and on page load (via getRoll).
	let activeRoll = $state<DiceRoll | null>(null);
	let activeRollDice = $state<DiceRollDie[]>([]);
	let activeRollVotes = $state<VoteView[]>([]);
	let activeRollParticipants = $state<RollParticipant[]>([]);
	let bankedDice = $state<BankedDie[]>([]);

	// ── Retinue sheet ─────────────────────────────────────────────────────────
	let retinueOpenForPlayer = $state<number | null>(null);
	let tonesOpen = $state(false);
	let lawsOpen = $state(false);
	let rumorsOpen = $state(false);
	// Separate from HelpButton's own Feedback sheet — this one's trigger lives
	// in the lobby phase's inline (unsheeted) HelpContent, not behind the "?".
	let lobbyFeedbackOpen = $state(false);
	let prologueActivePlayerID = $state<number | null>(null);

	// ── Lobby push soft-ask ──────────────────────────────────────────────────
	// vapidPublicKey is fetched once here (from getMe, alongside the rest of
	// onMount's auth check) and handed to LobbyView, which owns the rest of
	// the push-enable flow (its own pushState/dismissal/enable logic).
	let vapidPublicKey = $state('');

	// ── Mobile chat sheet ─────────────────────────────────────────────────────
	// Bound to ChatPanel's `expanded`. Kept here so the page can enforce one
	// full-screen surface at a time on mobile: opening any header panel closes
	// the chat (so the panel doesn't render behind the higher-z chat sheet),
	// and tapping the header bar dismisses the chat.
	let chatExpanded = $state(false);
	function closeChatSheet() {
		if (chatExpanded) chatExpanded = false;
	}

	// ── Player-pill strip (mobile) ────────────────────────────────────────────
	// .members can overflow at 4-5 players (or long names) on phone widths.
	// Track scroll position so the header can show a fade hint on whichever
	// edge is clipped, and scroll the viewer's own pill into view once on
	// load — the strip never auto-follows whoever's turn it is afterward, so
	// pill order stays stable and predictable.
	let membersEl = $state<HTMLDivElement | null>(null);
	let membersFadeLeft = $state(false);
	let membersFadeRight = $state(false);
	let ownPillScrolled = false;

	// A few px of tolerance absorbs sub-pixel rounding between scrollWidth and
	// clientWidth (fractional flex-grow widths routinely differ by 1-3px with
	// nothing actually clipped) so the fade doesn't flicker on for a row that
	// in fact fits.
	const MEMBERS_FADE_SLOP_PX = 4;
	function updateMembersFade() {
		if (!membersEl) return;
		const { scrollLeft, scrollWidth, clientWidth } = membersEl;
		membersFadeLeft = scrollLeft > MEMBERS_FADE_SLOP_PX;
		membersFadeRight = scrollLeft + clientWidth < scrollWidth - MEMBERS_FADE_SLOP_PX;
	}

	$effect(() => {
		void members.length;
		if (!membersEl) return;
		void tick().then(updateMembersFade);
		window.addEventListener('resize', updateMembersFade);
		return () => window.removeEventListener('resize', updateMembersFade);
	});

	$effect(() => {
		if (ownPillScrolled || !membersEl || currentPlayerID == null || members.length === 0) return;
		ownPillScrolled = true;
		void tick().then(() => {
			const el = membersEl?.querySelector<HTMLElement>(`[data-member-id="${currentPlayerID}"]`);
			el?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
		});
	});

	const blockingPlayerID = $derived.by(() => {
		if (!game) return null;
		if (game.phase === 'prologue') return prologueActivePlayerID;
		if (game.phase === 'main_event') return game.focus_player_id;
		return null;
	});

	// Each phase view writes its WaitingOnState here; the page renders the
	// bar from this single source. Lobby has no phase-view component, so
	// the page computes its lobby state inline below.
	let waitingOn = $state<WaitingOnState>({ waitees: [] });
	const lobbyWaitingOn = $derived.by<WaitingOnState>(() => {
		if (!game || game.phase !== 'lobby') return { waitees: [] };
		if (players.length < 2) {
			const need = 2 - players.length;
			return {
				waitees: [{ kind: 'label', text: `${need} more player${need === 1 ? '' : 's'} to join` }],
				stepLabel: 'Gathering players',
			};
		}
		const facilitator = players.find(p => p.is_facilitator);
		return {
			waitees: facilitator ? [{ kind: 'player', playerID: facilitator.id }] : [],
			// Sentence case, same as the branch above — the two used to disagree.
			stepLabel: 'Gathering players',
		};
	});
	$effect(() => {
		if (game?.phase === 'lobby') waitingOn = lobbyWaitingOn;
		else if (game?.phase === 'ended') waitingOn = { waitees: [] };
	});

	// Every player the game is currently waiting on — drives the gold ring on
	// the header chips. Derived from `waitingOn`, NOT blockingPlayerID: the
	// latter is phase-specific and single-player, while a row can be waiting on
	// several people at once (see waitingOn.ts / ResolvingWaitees). Sharing the
	// bar's source is the point — the ring and the "Waiting On:" list can never
	// disagree.
	//   'everyone' rings everyone: going dark there would read as "not you".
	//   'label' waitees (e.g. "facilitator to start") name no player → ring none.
	const waitingPlayerIDs = $derived.by<Set<number>>(() => {
		const ws = waitingOn.waitees;
		if (ws.some(w => w.kind === 'everyone')) return new Set(players.map(p => p.id));
		return new Set(ws.flatMap(w => (w.kind === 'player' ? [w.playerID] : [])));
	});
	const tonesLocked = $derived(
		game != null && (game.phase === 'main_event' || game.phase === 'shake_up' || game.phase === 'ended')
	);
	// Topics the table has actually taken a position on — the lobby's "N set"
	// chip. 'default' is the untouched state, so it doesn't count.
	const tonesSetCount = $derived(toneTopics.filter(t => t.status !== 'default').length);
	// The Public Record sidebar covers main_event (the timeline itself) and
	// shake_up/ended (its sealed "Shake-Up" pseudo-row continuity — see
	// PublicRecord.svelte). Rows are static in the latter two, so
	// loadGameState fetches them once and never repolls.
	const hasRecord = $derived(
		game != null && (game.phase === 'main_event' || game.phase === 'shake_up' || game.phase === 'ended')
	);

	// ── Plan state ────────────────────────────────────────────────────────────
	// Loaded on mount for main_event, then kept in sync by plan.* WS events.
	let plans = $state<Plan[]>([]);
	// Plan tokens (one per plan_type/player) drive the prep-grid pips. Refetched
	// on plan.prepared (a token appears) and rankings.updated (tokens may clear).
	let planTokens = $state<PlanToken[]>([]);

	// Player name map passed to MainEventView for attribution.
	const playerNameMap = $derived(new Map(players.map(p => [p.id, p.display_name])));

	// Publish the per-viewer "known secret" lookup to the asset-card seams
	// (CardPicker, scene + dice panels) via context, so they don't each thread
	// the visible-secrets array. The asset's public secret_count minus this is
	// the hidden ("struck eye") remainder. Backed by the live `secrets` state.
	provideSecretCounts(() => secrets);

	// Publish the line-of-succession crown lookup to the same asset-card + retinue
	// surfaces (ADR-007, Phase D). Crown role is a whole-game computation over all
	// live title marginalia, so the surfaces can't derive it from their own props.
	provideSuccession(() => assets, () => game?.throne_established ?? false);

	// Per-player rank triple (Power/Knowledge/Esteem), shown on the header chips
	// so relative standing is visible at all times. rank 1 = top, 5 = bottom;
	// null while rankings haven't been set yet (lobby/early prologue).
	// Header-chip derivations live in $lib/tableHeader (pure + unit-tested).
	// rank 1 = top, 5 = bottom; null while rankings haven't been set yet.
	const ranksByPlayer = $derived(rankTriplesByPlayer(rankings));
	// The best (lowest-numbered) rank a *player* actually holds on each track —
	// not always rank 1, since a dummy token can occupy the top slot. Whoever
	// holds the player-best is highlighted gold.
	const topRankByCategory = $derived(topRanks(ranksByPlayer));
	// Per-player count of "needlessly at-risk" assets — the warning badge on
	// each header chip (the avoidable case; see isNeedlesslyAtRisk).
	// Suppressed in the lobby: every joiner starts with a bare [Main Character]
	// peer, so the badge would fire on all chips at once and greet a new player
	// with three alarms on the screen that means "relax". Main-character
	// authoring stays available there (the retinue's marginalia "+" buttons
	// work) — it just isn't advertised until the prologue, where the player has
	// the context for it (ADR LOBBY_AND_CHECKLIST R3/D7).
	const atRiskByPlayer = $derived(
		game?.phase === 'lobby' ? new Map<number, number>() : atRiskCountByPlayer(assets)
	);

	// ── Public Record → Chat jump bridge ──────────────────────────────────────
	// Tapping a row/plan/scene in the expanded sidebar resolves the anchoring
	// system post (Chat Overhaul Phase 2e: the loaded window first, cheap;
	// then the Phase 1d anchor endpoint) and pushes a request to ChatPanel,
	// which scrolls there (and on mobile, expands itself first). If the
	// anchor wasn't already loaded, this also switches the feed into history
	// mode via an around-fetch — ChatPanel's "Return to now" button gets the
	// player back to the live tail afterward.
	let chatJumpRequest = $state<{ postID: number; key: number } | null>(null);
	let jumpKey = 0;
	function jumpTo(postID: number) {
		chatJumpRequest = { postID, key: ++jumpKey };
	}
	async function jumpToAnchor(req: AnchorRequest) {
		const resolved = await resolveAnchor(chatFeed, req);
		if (!resolved) return;
		if (!resolved.inWindow) await enterHistoryMode(chatFeed, resolved.postID);
		jumpTo(resolved.postID);
	}
	async function jumpToRow(rowNumber: number) {
		if (rowNumber === 1) {
			// Row 1 has no row.advanced post — best-effort fall back to
			// whatever's currently the oldest loaded post (there's no anchor
			// endpoint for "the very first post of the game").
			if (chatFeedPosts.length > 0) jumpTo(chatFeedPosts[0].id);
			return;
		}
		await jumpToAnchor({ code: 'row.advanced', row: rowNumber });
	}
	function jumpToPlan(planID: number, status: Plan['status']) {
		// A plan chip lives at the plan's *resolution* row (plan.RowNumber), not
		// the row it was prepared on (prepared_at_row, which the record never
		// surfaces), so anything past 'pending' should land on the resolution —
		// which is also the plan-resolution container's opening post, so the
		// jump lands on a card header rather than mid-card
		// (adr/CHAT_VISUAL_HIERARCHY_PLAN.md S3). A pending plan has no
		// plan.resolving post yet; plan.prepared is all there is to jump to.
		const code = status === 'pending' ? 'plan.prepared' : 'plan.resolving';
		void jumpToAnchor({ code, planID });
	}
	function jumpToScene(rowNumber: number) {
		// SceneEntry doesn't carry scene_id — anchor by row's first scene.started.
		void jumpToAnchor({ code: 'scene.started', row: rowNumber });
	}

	// ── WebSocket ─────────────────────────────────────────────────────────────
	let disconnect: (() => void) | null = null;

	// Reactive context handed to the extracted WS dispatcher (ws-handlers.ts).
	// Each accessor is backed by a $state rune above, so the dispatcher's
	// assignments stay reactive here. typingMap/typingTimeouts are shared by
	// reference (mutated in place).
	const wsCtx: WSContext = {
		get gameID() { return gameID; },
		loadGameState,
		typingMap, typingTimeouts,
		get game() { return game; }, set game(v) { game = v; },
		get players() { return players; }, set players(v) { players = v; },
		get members() { return members; }, set members(v) { members = v; },
		get toneTopics() { return toneTopics; }, set toneTopics(v) { toneTopics = v; },
		// A getter, not a by-reference share like typingMap above: the Map is
		// declared down in the tone-setting section, after this object literal
		// is built. Nothing reads it until a message arrives, so laziness is free.
		get toneWrites() { return toneWrites; },
		get rankings() { return rankings; }, set rankings(v) { rankings = v; },
		get assets() { return assets; }, set assets(v) { assets = v; },
		get laws() { return laws; }, set laws(v) { laws = v; },
		get rumors() { return rumors; }, set rumors(v) { rumors = v; },
		get secrets() { return secrets; }, set secrets(v) { secrets = v; },
		chatFeed,
		get recordRows() { return recordRows; }, set recordRows(v) { recordRows = v; },
		get rowState() { return rowState; }, set rowState(v) { rowState = v; },
		get activeScene() { return activeScene; }, set activeScene(v) { activeScene = v; },
		get activeScenePeers() { return activeScenePeers; }, set activeScenePeers(v) { activeScenePeers = v; },
		get sceneSetupDraft() { return sceneSetupDraft; }, set sceneSetupDraft(v) { sceneSetupDraft = v; },
		get preparePlanDraft() { return preparePlanDraft; }, set preparePlanDraft(v) { preparePlanDraft = v; },
		get activeRoll() { return activeRoll; }, set activeRoll(v) { activeRoll = v; },
		get activeRollDice() { return activeRollDice; }, set activeRollDice(v) { activeRollDice = v; },
		get activeRollVotes() { return activeRollVotes; }, set activeRollVotes(v) { activeRollVotes = v; },
		get activeRollParticipants() { return activeRollParticipants; }, set activeRollParticipants(v) { activeRollParticipants = v; },
		get bankedDice() { return bankedDice; }, set bankedDice(v) { bankedDice = v; },
		get plans() { return plans; }, set plans(v) { plans = v; },
		get planTokens() { return planTokens; }, set planTokens(v) { planTokens = v; },
		get prologueActivePlayerID() { return prologueActivePlayerID; }, set prologueActivePlayerID(v) { prologueActivePlayerID = v; },
		get typingNames() { return typingNames; }, set typingNames(v) { typingNames = v; },
	};

	function handleWSMessage(msg: WSMessage) {
		runWSMessage(wsCtx, msg);
	}

	// ── Data loading ──────────────────────────────────────────────────────────
	// Flips true the first time loadGameState completes. Past that point the
	// page is showing real state and a failed refresh must stay silent — see
	// the catch below. Deliberately not $state: nothing renders off it.
	let gameStateLoaded = false;

	// Set once in onMount, before the socket opens, so every resync can match
	// us against the roster it just fetched. Not $state: only currentPlayerID,
	// derived from it below, is ever rendered.
	let me: Account | null = null;

	// The in-flight getMe() call, started in onMount alongside the WebSocket so
	// the two run concurrently rather than back to back. loadGameState awaits
	// THIS rather than reading `me`, because the resync fires from ws.onopen and
	// can therefore win the race against getMe: reading a still-null `me` there
	// would leave the seat unresolved and bounce the player to /profile.
	// Not $state: awaited, never rendered.
	let mePromise: Promise<Account | null> | null = null;

	async function loadGameState() {
		try {
			// One parallel round in place of four sequential ones. These fetches
			// are mutually independent; the old code awaited them in series only
			// because the guards below read `data.game.phase`, which made them
			// look dependent. Production carries a fixed ~390ms per-request cost
			// (measured against /healthz, which touches no DB and still takes that
			// long), so four serial hops burned ~1.6s before a single query ran.
			//
			// Failure semantics are preserved exactly. Game state and assets still
			// reject the load — the page cannot render without them, and their
			// rejection lands in the catch below. Secrets and chat stay
			// best-effort, caught individually so neither can take the table down
			// with it.
			//
			// listAssets is now unconditional: its old guard named all five phases,
			// so it was a tautology. getVisibleSecrets used to be skipped in lobby
			// and no longer is — that costs one cheap empty response in the single
			// phase nobody is waiting on, and saves a round trip in every other.
			// ListVisibleSecretsForGame has no phase gate and returns [] when there
			// is nothing visible.
			const [data, assetData, secretData] = await Promise.all([
				getGameState(gameID),
				listAssets(gameID),
				getVisibleSecrets(gameID).catch(() => null),
				// Chat is available in every phase, so it resyncs unconditionally.
				// reconnectResync (Chat Overhaul Phase 2b) does the right thing for
				// both the very first load (empty window → full initial fetch) and
				// every reconnect after that (live mode: cheap `after` fetch;
				// history mode: no-op — "Return to now" catches that window up on
				// demand). This runs on every (re)connect per createConnection's
				// contract, so it must never refetch the whole feed.
				reconnectResync(chatFeed).catch(() => { /* tolerate; WS events + the next resync keep us eventually consistent */ }),
			]);

			game = data.game;
			players = data.players;
			if (data.tone_topics) toneTopics = data.tone_topics;
			if (data.rankings) rankings = data.rankings;
			if (data.current_prologue_player_id !== undefined) prologueActivePlayerID = data.current_prologue_player_id;
			if (data.laws) laws = data.laws;
			if (data.rumors) rumors = data.rumors;
			members = data.players.map(p => ({
				id: p.id,
				display_name: p.display_name,
				online: false
			}));
			// Assign-only: the payload is best-effort server-side, and a resync
			// that lost the one query shouldn't blank a header line that was
			// already correct.
			if (data.player_activity) playerActivity = data.player_activity;

			// Resolve our seat from whichever roster just arrived, rather than
			// only from the one onMount happened to see. A first load that
			// failed never reaches here, so without this a player who stayed on
			// the page (see onMount) would sit at a table the app didn't know
			// they were sitting at even after a later resync succeeded.
			// Assign-only, never clear: a mid-game roster that somehow omits us
			// is not a reason to pull the seat out from under a live session.
			// Resolved from the in-flight promise rather than the `me` binding, so
			// a resync that beats getMe home waits for it instead of reading null
			// and silently losing the seat. Falls back to `me` for any caller that
			// reaches here without onMount having started the fetch; in practice
			// mePromise is always set, since the connection driving this resync is
			// created right after it. Captured into a local either way so the null
			// check narrows below — TS won't narrow a mutable outer binding across
			// a closure.
			const account = mePromise ? await mePromise : me;
			const seat = account ? data.players.find((p) => p.account_id === account.id) : undefined;
			if (seat) currentPlayerID = seat.id;

			// Assigned here, once every fetch in the round above has landed, rather
			// than from inside the individual promise callbacks. Keeping the
			// original assignment order matters: a derived that reads assets and
			// secrets together must never observe one updated without the other —
			// that is the shape of the asset.taken bug that froze reactivity.
			assets = assetData.assets;
			// null means the secrets fetch failed and was tolerated; leave whatever
			// is already on screen rather than blanking it.
			if (secretData) secrets = secretData.secrets;

			// Public record, plans, active roll, and active scene only matter
			// in main_event.
			if (data.game.phase === 'main_event' && data.game.current_row > 0) {
				const [recordData, rollData, plansData, tokensData, sceneData, bankedData] = await Promise.all([
					getFullRecord(gameID),
					getActiveRollForGame(gameID),
					listPlans(gameID),
					listPlanTokens(gameID).catch(() => ({ tokens: [] as PlanToken[] })),
					getActiveScene(gameID).catch(() => ({ scene: null, peers: [] as ScenePeerView[] })),
					listBankedDice(gameID).catch(() => ({ dice: [] as BankedDie[] })),
				]);
				recordRows = recordData.rows;
				plans = plansData.plans;
				planTokens = tokensData.tokens;
				activeScene = sceneData.scene;
				activeScenePeers = sceneData.peers;
				rowState = data.row_state ?? null;
				bankedDice = bankedData.dice;
				if (rollData.roll) {
					activeRoll = rollData.roll;
					activeRollDice = rollData.dice;
					activeRollVotes = rollData.votes;
					activeRollParticipants = rollData.participants;
				} else {
					// No active roll server-side (none open, and any plan-linked
					// roll's plan has finished resolving). Clear any stale roll so a
					// resync after a resolution doesn't leave the panel up.
					activeRoll = null;
					activeRollDice = [];
					activeRollVotes = [];
					activeRollParticipants = [];
				}
			} else if (data.game.phase === 'shake_up' || data.game.phase === 'ended') {
				// The endgame has no plans/scene, just the current shake-up roll
				// (getActiveRollForGame is shake-up-aware) and the now-static Public
				// Record (rows 1-13 are frozen from here on — one fetch, no
				// polling). Shake-up rolls never enter the voting stage, so
				// activeRollVotes is left alone here — ShakeUpView passes
				// DiceRollPanel a literal empty votes array.
				// Parallel for the same reason as the round above: these two are
				// independent, and serialising them cost an extra ~390ms hop in the
				// endgame. Each stays individually tolerated, so one failing still
				// leaves the other's result on screen.
				const [rollData, endRecord] = await Promise.all([
					getActiveRollForGame(gameID).catch(() => null), /* tolerate; RollCreated/RollResolved WS events keep this in sync */
					getFullRecord(gameID).catch(() => null), /* tolerate; the sidebar just shows what's already loaded */
				]);
				if (rollData) {
					if (rollData.roll) {
						activeRoll = rollData.roll;
						activeRollDice = rollData.dice;
						activeRollParticipants = rollData.participants;
					} else {
						activeRoll = null;
						activeRollDice = [];
						activeRollParticipants = [];
					}
				}
				if (endRecord) recordRows = endRecord.rows;
			}

			gameStateLoaded = true;
			// A resync that reached here has replaced every field it owns, so
			// whatever the banner was complaining about is moot. Clearing on
			// success rather than on entry means a *failed* resync leaves the
			// previous message up instead of blanking it mid-flight.
			// Safe to clear unconditionally now that only load failures live
			// here — this line used to erase action errors too, seconds after
			// the player raised them.
			loadError = '';
		} catch (e) {
			// Only the initial load may raise the banner. This function runs on
			// every WS (re)connect — including the reconnect a returning player
			// triggers by making the tab visible (see ws.ts's onVisibility). If
			// the server was redeployed or the machine slept while the tab sat
			// in the background, the first fetch over a dead pooled connection
			// fails with a bare "Failed to fetch" even though the socket just
			// opened fine and the very next resync will succeed. A failed
			// background refresh must leave the page showing what it already
			// had — the same call profile/+page.svelte's refreshTables makes.
			if (!gameStateLoaded) {
				loadError = e instanceof Error ? e.message : 'Could not load game state.';
			}
		}
	}

	// ── Activity heartbeat ────────────────────────────────────────────────────
	// Tells the server this player has the table on screen, which is what the
	// Retinue header's "last here 3h ago" reads. Only foreground events fire it
	// — mount and tab-becomes-visible — because the whole point is to mean
	// something a socket connection doesn't: a tab left open on a phone in a
	// drawer stays connected for days, and would otherwise report as present.
	//
	// Throttled here as well as server-side (playerActivityThrottle, one hour).
	// The server one is what makes the value honest; this one just avoids a
	// request per tab switch, which on a phone is a lot of requests to have
	// the server decline.
	const ACTIVITY_MIN_GAP_MS = 5 * 60_000;
	let lastActivityPing = 0;
	function pingActivity() {
		const now = Date.now();
		if (now - lastActivityPing < ACTIVITY_MIN_GAP_MS) return;
		lastActivityPing = now;
		// Fire-and-forget by design: this is observational, and a failure must
		// never surface as an error banner over the game.
		void touchActivity(gameID).catch(() => {});
	}

	onMount(() => {
		const onVisible = () => {
			if (document.visibilityState === 'visible') pingActivity();
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => document.removeEventListener('visibilitychange', onVisible);
	});

	onMount(async () => {
		try {
			// getMe and the WebSocket handshake are independent — the upgrade
			// carries the session cookie and needs nothing from the account object
			// — so they run concurrently, saving one ~390ms round trip off the
			// front of every table load.
			mePromise = getMe();
			const conn = createConnection(gameID, handleWSMessage, loadGameState);
			disconnect = conn.disconnect;

			me = await mePromise;
			if (!me) {
				// Stop the socket retrying against a page we are leaving. Opening it
				// before the auth check is what buys the overlap; the server rejects
				// the upgrade for a signed-out visitor, and without this the backoff
				// would keep reconnecting after the redirect.
				disconnect();
				goto(`/?next=/table/${gameID}`);
				return;
			}
			// Independent of the roster — a table we end up staying on after a
			// failed first load still has its push key.
			vapidPublicKey = me.vapid_public_key;

			// createConnection (above) runs loadGameState on every (re)connect —
			// including this initial one — and buffers any events that arrive
			// during the fetch so we never miss a transition. Await `ready` so the
			// seat it resolves is available below.
			await conn.ready;

			// `ready` resolves through a `.finally`, so it settles whether that
			// first resync succeeded or failed — deliberately, so a failed
			// fetch can't leave the socket frozen. The cost is that an
			// unresolved seat here is ambiguous: it means either "no seat at
			// this table" or "the fetch never landed". Only the first is
			// grounds for sending someone away. Reading the second as one
			// bounced players to /profile over a transient blip — exactly what
			// a dead pooled connection after a redeploy produces — and it
			// looked random, because reopening the tab usually worked. The
			// catch in loadGameState has already put the reason on screen;
			// stay put and let the next resync repair it.
			if (!gameStateLoaded) return;

			if (currentPlayerID === null) {
				goto('/profile');
				return;
			}

			// After the seat is confirmed, not before: the endpoint 403s for a
			// non-member, and someone being bounced to /profile has no activity
			// at this table to record.
			pingActivity();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load table.';
		} finally {
			loading = false;
		}
	});

	// The endgame-mode interrupt modal used to live here — a facilitator-only
	// scrim triggered by the `uneasy:endgame_choice_required` window event, with
	// a Cancel button that dropped the table into limbo. Retired with the row
	// 7 → 8 table vote (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7): the mode is
	// settled by everyone, one row before it can matter, in
	// EndgameVotePanel.svelte. The 409 that drove it is now an ordinary error.

	onDestroy(() => {
		disconnect?.();
		typingTimeouts.forEach(clearTimeout);
	});

	// ── Plan helpers ─────────────────────────────────────────────────────────
	/** Re-fetches the plan list and tokens. Passed to MainEventView as
	 *  onPlansChanged; preparing a plan also places a token, so the grid pips
	 *  must stay in sync with the plan list. */
	async function refreshPlans() {
		try {
			// Also refetch assets: many plan actions mutate assets (festivity
			// introduces/takes peers, war seizes, duels/demands break marginalia),
			// but asset deltas otherwise arrive only over the WebSocket. Pulling
			// them here means the actor's own screen (retinue, peer lists) reflects
			// the change even if the socket is momentarily down — plans alone left
			// it stale until a reload.
			const [data, tokensData, assetData] = await Promise.all([
				listPlans(gameID),
				listPlanTokens(gameID).catch(() => ({ tokens: planTokens })),
				listAssets(gameID).catch(() => ({ assets })),
			]);
			plans = data.plans;
			planTokens = tokensData.tokens;
			assets = assetData.assets;
		} catch { /* ignore — WS events will keep us in sync */ }
	}

	// ── Tone-setting ──────────────────────────────────────────────────────────
	let newTopicText = $state('');
	let addingTopic = $state(false);
	// Rendered inside the Tones sheet, not in the page banner. The sheet is a
	// fixed z-index:91 panel over a full-viewport scrim, so a message in the
	// header banner is painted underneath the dialog the player is looking at
	// — they'd see the topic silently fail to change with the dialog still
	// open. Cleared when the sheet opens.
	let toneError = $state('');

	/** Both entry points to the Tones sheet (the header button and the
	 *  prologue's own link) go through here so a message left over from a
	 *  previous visit never greets the player on open. */
	function openTones() {
		toneError = '';
		tonesOpen = true;
	}

	/** In-flight tone writes, keyed by topic — owned by $lib/toneWrites and
	 *  also read by ws-handlers to drop this client's own echoes. */
	const toneWrites = new Map<number, ToneWrite>();

	const toneWriteCtx: ToneWriteContext = {
		get gameID() { return gameID; },
		setStatus: (topicID, status) => {
			toneTopics = toneTopics.map(t => (t.id === topicID ? { ...t, status } : t));
		},
		replaceTopics: (topics) => { toneTopics = topics; },
		setError: (message) => { toneError = message; },
		inFlight: toneWrites,
	};

	function cycleTopicStatus(topic: ToneTopic) {
		// Read the status off the live array rather than the `topic` argument.
		// The argument is a snapshot from render, and taps can outrun it.
		const current = toneTopics.find(t => t.id === topic.id)?.status ?? topic.status;
		void cycleToneStatus(toneWriteCtx, topic.id, current);
	}

	async function submitNewTopic() {
		const text = newTopicText.trim();
		if (!text || addingTopic) return;
		addingTopic = true;
		toneError = '';
		try {
			await addToneTopic(gameID, text);
			newTopicText = '';
		} catch (e) {
			toneError = e instanceof Error ? e.message : 'Could not add topic.';
		} finally {
			addingTopic = false;
		}
	}

</script>

<div class="table-page">
	<!-- Header ──────────────────────────────────────────────────────────────── -->
	<!--
		Tapping the header bar closes the mobile chat sheet. Clicks on the
		header's own buttons (Tones/Laws/Rumors/War, member chips) bubble here
		too, so opening any of those panels also closes the chat — keeping a
		single full-screen surface on mobile and avoiding the panel rendering
		behind the chat sheet.
	-->
	<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
	<header onclick={closeChatSheet}>
		<div class="top-strip">
			<a class="home" href="/profile" aria-label="Home">
				<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
					<path d="M3 11l9-8 9 8" />
					<path d="M5 10v10h14V10" />
				</svg>
			</a>
			<!-- --member-count feeds .member-name's width budget (see its CSS):
			     the names split the strip N ways, so the count has to reach
			     CSS. Falls back to 1 rather than 0 so the budget stays a
			     valid length while presence is still loading. -->
			<div class="members-wrap" class:fade-left={membersFadeLeft} class:fade-right={membersFadeRight} style:--member-count={members.length || 1}>
				<div class="members" bind:this={membersEl} onscroll={updateMembersFade}>
					{#each members as member}
						{@const mr = ranksByPlayer.get(member.id)}
						{@const atRisk = atRiskByPlayer.get(member.id) ?? 0}
						{@const isMe = member.id === currentPlayerID}
						{@const isWaiting = waitingPlayerIDs.has(member.id)}
						<button type="button" class="member" class:mine={isMe} class:waiting={isWaiting} data-member-id={member.id} onclick={() => retinueOpenForPlayer = member.id} title={member.display_name} aria-label={`View ${member.display_name}'s retinue${isMe ? ' (you)' : ''}${isWaiting ? (isMe ? ' — your turn' : ' — their turn') : ''}${atRisk > 0 ? ` — ${atRisk} ${atRisk === 1 ? 'asset needs' : 'assets need'} marginalia` : ''}`} style:--member-color={playerColorByID(member.id, players)}>
							{#if atRisk > 0}
								<span class="risk-badge" title={`${atRisk} ${atRisk === 1 ? 'asset has' : 'assets have'} too few marginalia — ${isMe ? `fill an empty slot to avoid losing ${atRisk === 1 ? 'it' : 'them'}` : `their owner can still shore ${atRisk === 1 ? 'it' : 'them'} up`}`} aria-hidden="true">{atRisk}</span>
							{/if}
							<span class="member-body">
								<span class="member-name-row">
									<span class="dot"></span>
									<!-- Your own chip reads "You", never your name: it's the same
									     convention WaitingOnBar already uses for the current
									     player, it costs less width than any name, and it's the
									     one label no other chip can be confused with when two
									     names truncate to the same prefix. `title` above still
									     carries the full name on hover. -->
									<span class="member-name">{isMe ? 'You' : member.display_name}</span>
								</span>
								{#if mr && (mr.power != null || mr.knowledge != null || mr.esteem != null)}
									<span class="member-ranks" aria-label={`Ranks — Power ${mr.power ?? '—'}, Knowledge ${mr.knowledge ?? '—'}, Esteem ${mr.esteem ?? '—'}`}>
										<span class="mr" class:top={mr.power != null && mr.power === topRankByCategory.power}><span class="mr-cat">P</span>{mr.power ?? '—'}</span>
										<span class="mr" class:top={mr.knowledge != null && mr.knowledge === topRankByCategory.knowledge}><span class="mr-cat">K</span>{mr.knowledge ?? '—'}</span>
										<span class="mr" class:top={mr.esteem != null && mr.esteem === topRankByCategory.esteem}><span class="mr-cat">E</span>{mr.esteem ?? '—'}</span>
									</span>
								{/if}
							</span>
						</button>
					{/each}
				</div>
			</div>
			<HelpButton gameId={gameID} route={page.url.pathname} phase={game?.phase} />
		</div>
		{#if game}
			<div class="game-info" class:has-war={$activeWarCount + $pendingWarCount > 0}>
				<PhaseBadge phase={game.phase} />
				<button class="tones-button" onclick={openTones} aria-label="Open tones">
					<span class="lbl">Tones</span>
				</button>
				<button class="tones-button" onclick={() => lawsOpen = true} aria-label="Open laws">
					<span class="lbl">Laws</span>{#if laws.length > 0}<span class="count">{laws.length}</span>{/if}
				</button>
				<button class="tones-button" onclick={() => rumorsOpen = true} aria-label="Open rumors">
					<span class="lbl">Rumors</span>{#if rumors.length > 0}<span class="count">{rumors.length}</span>{/if}
				</button>
				{#if $activeWarCount + $pendingWarCount > 0}
					<button
						class="tones-button war-button"
						class:war-pending={$activeWarCount === 0}
						onclick={() => warDrawerOpen.set(true)}
						aria-label="Open wars"
					>
						<span class="lbl">War</span>{#if $activeWarCount + $pendingWarCount > 1}<span class="count">{$activeWarCount + $pendingWarCount}</span>{/if}
					</button>
				{/if}
			</div>
		{/if}
	</header>

	{#if loadError}
		<ErrorText message={loadError} extra="error" />
	{/if}

	<!--
		Body: at ≥790px (the chat dock) this becomes a 2-column grid
		(game | chat), or 3-column when the Public Record rail is present.
		Below that it's a single column with the chat panel positioned
		absolutely (strip pinned to bottom, expanded sheet covering the body).
		WaitingOnBar lives inside .phase-column so it only spans the phase
		content's column — not the PublicRecord rail or the Chat column.
	-->
	<div class="table-body" class:has-record={hasRecord}>

	<!-- Public Record sidebar — main_event, plus shake_up/ended for the sealed
	     Shake-Up pseudo-row's continuity. Page-level so it can sit in its own
	     grid column on wide desktop layouts (mirrors ChatPanel). -->
	{#if !loading && hasRecord && game}
		<PublicRecord
			rows={recordRows}
			currentRow={game.current_row}
			phase={game.phase}
			shakeUpCategory={game.shake_up_category}
			playerNames={playerNameMap}
			{players}
			onRowJump={jumpToRow}
			onPlanJump={jumpToPlan}
			onSceneJump={jumpToScene}
		/>
	{/if}

	<div class="phase-column">
	{#if !loading && game}
		<WaitingOnBar state={waitingOn} {currentPlayerID} {players} />
	{/if}

	<!--
		A throw during render used to blank the page with no message: there was no
		boundary anywhere in the app, and the two incidents we've had (the
		asset.taken payload that had to be merged rather than replaced, and the
		optimistic-append/WS duplicate key) both presented as "the UI froze"
		rather than as an error. Scoped to the phase views only — the header,
		WaitingOnBar, Public Record and chat stay alive, so the player can still
		read the table and reach the reload. `reset` re-renders: worth offering
		first, since a bad payload is usually repaired by the next resync.
	-->
	<svelte:boundary>
		{#if loading}
			<div class="center-message">Loading…</div>

		<!-- ── Lobby ──────────────────────────────────────────────────────────── -->
		{:else if game?.phase === 'lobby'}
			<LobbyView
				{gameID}
				{game}
				{players}
				{members}
				{currentPlayerID}
				{waitingPlayerIDs}
				{isFacilitator}
				{vapidPublicKey}
				{tonesSetCount}
				onOpenTones={openTones}
				onFeedback={() => lobbyFeedbackOpen = true}
			/>

		<!-- ── Prologue ───────────────────────────────────────────────────────── -->
		{:else if game?.phase === 'prologue'}
			<PrologueView
				{gameID}
				{game}
				bind:players
				bind:rankings
				bind:assets
				{currentPlayerID}
				bind:waitingOn
				{laws}
				{rumors}
				{tonesSetCount}
				onResync={loadGameState}
				onOpenTones={openTones}
				onOpenRetinue={(playerID) => retinueOpenForPlayer = playerID ?? currentPlayerID}
				onOpenLaws={() => lawsOpen = true}
				onOpenRumors={() => rumorsOpen = true}
			/>

		<!-- ── Main Event ─────────────────────────────────────────────────────── -->
		{:else if game?.phase === 'main_event'}
			<MainEventView
				{game}
				{players}
				{rankings}
				{assets}
				{laws}
				{rumors}
				{currentPlayerID}
				bind:recordRows
				{rowState}
				{playerNameMap}
				bind:activeRoll
				bind:activeRollDice
				bind:activeRollVotes
				bind:activeRollParticipants
				bind:bankedDice
				{plans}
				{planTokens}
				onPlansChanged={refreshPlans}
				{activeScene}
				{activeScenePeers}
				{sceneSetupDraft}
				{preparePlanDraft}
				onSceneRefresh={refreshActiveScene}
				bind:waitingOn
			/>

		<!-- ── Shake-Up ───────────────────────────────────────────────────────── -->
		{:else if game?.phase === 'shake_up'}
			<ShakeUpView
				{gameID}
				{game}
				{players}
				{assets}
				{rankings}
				{currentPlayerID}
				bind:activeRoll
				bind:activeRollDice
				bind:activeRollParticipants
				bind:waitingOn
			/>

		<!-- ── Ended ──────────────────────────────────────────────────────────── -->
		{:else if game?.phase === 'ended'}
			<EndedView {rankings} {players} />

		{:else}
			<div class="center-message">Unknown phase.</div>
		{/if}

		{#snippet failed(_error, reset)}
			<div class="center-message boundary-failed" role="alert">
				<p>Something went wrong displaying this part of the table.</p>
				<div class="boundary-actions">
					<button class="action-btn primary" onclick={reset}>Try again</button>
					<button class="action-btn secondary" onclick={() => location.reload()}>Reload the page</button>
				</div>
			</div>
		{/snippet}
	</svelte:boundary>
	</div><!-- /.phase-column -->

		{#if !loading && currentPlayerID != null && game}
			<ChatPanel
				{gameID}
				feed={chatFeed}
				{players}
				{currentPlayerID}
				{typingLabel}
				{activeScene}
				{activeScenePeers}
				{assets}
				jumpRequest={chatJumpRequest}
				bind:expanded={chatExpanded}
			/>
		{/if}
	</div>

	<RetinueSheet open={tonesOpen} onClose={() => tonesOpen = false}>
		<div class="tones-sheet">
			<!-- Scoped class, not a bare h3 rule: `.tones-sheet` is borrowed by the
			     feedback sheet further down, which wants the default heading. -->
			<h3 class="sheet-title">Tones</h3>
			{#if toneError}
				<ErrorText message={toneError} />
			{/if}
			<!-- The lede reuses SceneSetupForm's `.prompt` recipe — warm fill, gold
			     left rule — which is the app's established "read this" device. This
			     panel badly needed one: the sentence used to be the lowest-contrast
			     text on a screen otherwise filled with 41 high-contrast tiles, so the
			     eye went straight past it to the grid.

			     Deliberately WITHOUT that recipe's small-caps label: the sheet title
			     sits directly above, and two headings stacked back to back read as a
			     stutter. The fill and the gold rule carry the emphasis on their own.

			     It carries only the *what*; the *how* ("tap to cycle") moved down
			     into the legend, where the mechanic it describes actually lives.

			     The claim that no one is told who set a tone is load-bearing and
			     true: tone_topics is keyed on the game with no player column, and
			     handler/tone.go broadcasts a WS event without emitting an action-log
			     post. The tile does change colour for everyone — that is the point —
			     it just carries no name. Don't add attribution or a log entry here
			     without rewriting this line. -->
			<div class="tone-lede" class:locked={tonesLocked}>
				{#if tonesLocked}
					<p>
						Themes and topics the table set to include or avoid. No one is told
						who set them.
					</p>
					<p class="tone-lede-locked">Locked — the main event has begun.</p>
				{:else}
					<p>
						Themes and topics the table wants to include or avoid. Set as many as you like. 
						No one is told who set each.
					</p>
				{/if}
			</div>

			<!-- Sticky, because this is the key to a 41-tile grid and unstuck it was
			     gone after one flick on a phone — leaving you tapping colours with
			     no way left on screen to read them. It sticks to the scrollport of
			     RetinueSheet's `.sheet-body`, so it must stay a DIRECT child of
			     `.tones-sheet`: wrapping it with the title in a header div would
			     bound the stick to that div and it would scroll away again.

			     It also carries the rule that separates the header block from the
			     tile field, so when stuck it reads as a bar over the grid. -->
			<div class="tone-legend" aria-label="Legend">
				<span class="tone-legend-lead">{tonesLocked ? 'Key' : 'Tap to cycle'}</span>
				<!-- The four swatches are tuned to sit on ONE line at 360, the narrow
				     end of the design band (docs/STYLE_GUIDE.md "Layout widths"), which
				     is what lets the lead centre above them as a caption rather than
				     leading a ragged block. They only just make it — 328px of column
				     against ~318px of swatches — so the gaps here and in
				     `.tone-legend-item` are load-bearing, not taste. It still wraps
				     rather than overflows if a fallback font ever runs wider. -->
				<div class="tone-legend-items">
					<span class="tone-legend-item" data-status="default"><span class="swatch"></span>No Opinion</span>
					<span class="tone-legend-item" data-status="include"><span class="swatch"></span>Include</span>
					<span class="tone-legend-item" data-status="avoid_detail"><span class="swatch"></span>Avoid detail</span>
					<span class="tone-legend-item" data-status="never"><span class="swatch"></span>Never</span>
				</div>
			</div>

			<div class="tone-grid">
				{#each toneTopics as topic (topic.id)}
					<button
						type="button"
						class="tone-tile"
						data-status={topic.status}
						disabled={tonesLocked}
						onclick={() => cycleTopicStatus(topic)}
						aria-label={`${topic.topic}: ${topic.status === 'default' ? 'No Opinion' : topic.status === 'avoid_detail' ? 'Avoid detail' : topic.status === 'include' ? 'Include' : 'Never'}.${tonesLocked ? '' : ' Tap to cycle.'}`}
					>
						<span class="tone-tile-topic">{topic.topic}</span>
					</button>
				{/each}

			</div>

			{#if !tonesLocked}
				<form
					class="tone-add-row"
					onsubmit={(e) => { e.preventDefault(); submitNewTopic(); }}
				>
					<input
						type="text"
						class="tone-add-input"
						placeholder="Add a custom topic…"
						bind:value={newTopicText}
						maxlength={TEXT_LIMITS.TONE_TOPIC}
						aria-label="Add a custom topic"
					/>
					<button
						type="submit"
						class="tone-add-button"
						disabled={!newTopicText.trim() || addingTopic}
					>
						{addingTopic ? '…' : '+ Add'}
					</button>
				</form>
			{/if}
		</div>
	</RetinueSheet>

	<RetinueSheet open={lawsOpen} onClose={() => lawsOpen = false}>
		<div class="laws-rumors-sheet">
			<h3 class="sheet-title">Laws</h3> <!--  ({laws.length}) -->
			<LawsRumors
				kind="laws"
				{laws}
				{rumors}
				{plans}
				{players}
				playerNames={playerNameMap}
				{currentPlayerID}
			/>
		</div>
	</RetinueSheet>

	<RetinueSheet open={rumorsOpen} onClose={() => rumorsOpen = false}>
		<div class="laws-rumors-sheet">
			<h3 class="sheet-title">Rumors</h3> <!--  ({rumors.length}) -->
			<LawsRumors
				kind="rumors"
				{laws}
				{rumors}
				{plans}
				{players}
				playerNames={playerNameMap}
				{currentPlayerID}
			/>
		</div>
	</RetinueSheet>

	<RetinueSheet open={lobbyFeedbackOpen} onClose={() => lobbyFeedbackOpen = false}>
		<div class="tones-sheet">
			<h3 class="sheet-title">Send feedback</h3>
			<FeedbackForm gameId={gameID} route={page.url.pathname} phase={game?.phase} />
		</div>
	</RetinueSheet>

	<RetinueSheet open={retinueOpenForPlayer !== null} onClose={() => retinueOpenForPlayer = null}>
		{#if retinueOpenForPlayer !== null}
			<RetinueView
				playerId={retinueOpenForPlayer}
				{players}
				{members}
				{playerActivity}
				{assets}
				{secrets}
				{rankings}
				viewerPlayerId={currentPlayerID}
				focusPlayerId={blockingPlayerID}
				isWaitedOn={waitingPlayerIDs.has(retinueOpenForPlayer)}
				onSecretsChanged={() => getVisibleSecrets(gameID).then(d => { secrets = d.secrets; }).catch(() => {})}
			/>
		{/if}
	</RetinueSheet>

</div>

<style>
	/* ── Layout ─────────────────────────────────────────────────────────────── */

	.table-page {
		/* Single source of truth for the mobile chat strip's height. Read by
		   ChatPanel.svelte (.strip min-height) and by .table-body's reserved
		   padding-bottom below, so the two stay in sync.
		   56px, not the 46px it started at: the bar carries a mark and a label
		   now (see .strip's comment on why it had to), and border-box means
		   this height has to contain its padding — 46 left a ~19px content box
		   that could only hold a single line of text. */
		--chat-strip-height: 56px;

		display: flex;
		flex-direction: column;
		/* Fills main.full-bleed, which the layout sizes to the viewport minus
		   whatever the update banner is taking (see body:has in +layout.svelte).
		   Was a bare 100dvh; that ignored the banner and pushed the
		   bottom-pinned chat strip off-screen whenever one appeared. */
		height: 100%;
		max-width: 100%;
	}

	/*
	 * Body fills the space below the header. ChatPanel is a sibling of the
	 * phase content. On mobile it positions itself absolutely (strip pinned
	 * to bottom; expanded sheet covers the body), so the phase content reads
	 * the body's full size. At ≥790px (the chat dock) the body becomes a
	 * grid: phase content on the left, chat as the permanent right column.
	 */
	.table-body {
		flex: 1;
		min-height: 0;
		position: relative;
		display: flex;
		flex-direction: column;
		/* Keep phase content from being hidden behind the mobile chat strip,
		   including the iOS home-indicator safe area. The extra 0.75rem is
		   breathing room so the last bit of content isn't flush against the
		   strip's top edge (and isn't darkened by its upward box-shadow). */
		padding-bottom: calc(var(--chat-strip-height) + 1rem + env(safe-area-inset-bottom));
	}

	/* Cap-and-center (docs/STYLE_GUIDE.md "Layout widths"): every content
	   column is a phone-width column. Without the record, the phase view's
	   box is at most 440 (the widest mainstream phone) and centers when the
	   viewport has slack; views bring their own inner padding, exactly as
	   they would against a real phone's viewport edge. */
	.table-body:not(.has-record) > .phase-column {
		width: 100%;
		max-width: 440px;
		margin-inline: auto;
	}

	/* Wherever the public record shows (main_event, shake_up, ended) on
	   mobile, its rail sits to the left of the phase view rather than
	   stacking above it (the rail is full-height, so stacking pushes the
	   phase content off-screen). The chat panel is position:absolute on
	   mobile so it stays unaffected. Rail (44) + gutters (8) are the only
	   chrome: on a 360 phone the phase content gets exactly 300, on a 440
	   phone 380 — the record-phase design band. Past 440-equivalent the
	   rail+content block centers as one unit. */
	.table-body.has-record {
		flex-direction: row;
		gap: 8px;
		padding-right: 8px;
		justify-content: center;
	}
	.table-body.has-record > .phase-column {
		flex: 0 1 380px;
		min-width: 0;
		min-height: 0;
	}

	/* Wrapper that groups WaitingOnBar with the active phase view so they
	   occupy a single column in the body's grid/flex layout. Without this,
	   WaitingOnBar would span every column (over the PublicRecord rail and
	   pushing the Chat panel down on desktop). */
	.phase-column {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		/* Named query container: components inside adapt to the COLUMN's
		   width via @container column (…), never to the viewport
		   (docs/STYLE_GUIDE.md "Layout widths"). */
		container: column / inline-size;
	}

	/* Chat dock (790 = 44 rail + 8 + 360 main + 8 + 360 chat + 8): chat
	   becomes a permanent right column. Every column is minmax'd to the
	   phone band (360–440; the record-phase main column caps at 380 = 440
	   minus rail+gutters) and the grid centers once all columns hit their
	   caps — no column ever grows past a phone. */
	@media (min-width: 790px) {
		.table-body {
			display: grid;
			grid-template-columns: minmax(360px, 440px) minmax(360px, 440px);
			justify-content: center;
			gap: 8px;
			padding-inline: 8px;
			padding-bottom: 0;
		}
		/* With the record (main_event, shake_up, ended): rail, phase, chat.
		   The rail stays flush left, as on phones. */
		.table-body.has-record {
			grid-template-columns: 44px minmax(360px, 380px) minmax(360px, 440px);
			padding-left: 0;
		}
		/* In grid mode the base cap-and-center is the track's job. */
		.table-body:not(.has-record) > .phase-column {
			max-width: none;
			margin-inline: 0;
		}
		/* The phase content children are direct children of .table-body; in
		   grid mode they land in source order (record, phase, chat). The
		   min-width: 0 prevents long content from blowing out its column. */
		.table-body > :global(*) { min-width: 0; min-height: 0; }
	}

	/* Record dock (1070 ≥ 8 + 316 record + 8 + 360 main + 8 + 360 chat + 8):
	   the rail/overlay becomes a permanent 316px panel in column 1. */
	@media (min-width: 1070px) {
		.table-body.has-record {
			grid-template-columns: 316px minmax(360px, 380px) minmax(360px, 440px);
			padding-left: 8px;
		}
	}

	header {
		padding: 0.75rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		flex-shrink: 0;
	}

	.game-info {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	/* Wars are rare; when one is active below the chat dock, drop the badge
	   (a shared component, hence :global) so the War button takes its slot —
	   the row never exceeds four items. Header rules are the one place
	   viewport queries are legitimate (the header spans the viewport), and
	   they use the dock literals only. */
	@media (max-width: 789px) {
		.game-info.has-war :global(.phase-badge) { display: none; }
	}

	.tones-button {
		display: inline-flex;
		align-items: center;
		font-family: var(--font-serif);
		font-size: 0.85rem;
		font-weight: 400;
		background: var(--color-surface-2);
		color: var(--color-text);
		padding: 0;
		border-radius: 4px;
		border: 1px solid var(--color-border-warm);
		min-height: 32px;
	}
	.tones-button .lbl { padding: 0.3rem 0.7rem; }
	.tones-button:hover { background: var(--color-border-warm); border-color: var(--color-accent); }
	.tones-button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* Count: a small, dim number behind a subtle divider — a hint, not a headline. */
	.tones-button .count {
		align-self: stretch;
		display: flex;
		align-items: center;
		padding: 0 0.42rem;
		border-left: 1px solid var(--color-border-warm); /* subtle warm hairline */
		color: var(--color-accent-muted);                    /* dimmed gilt */
		font-size: 0.75rem;
		font-variant-numeric: tabular-nums;
	}
	.war-button .count { border-left-color: var(--color-chip-red-border); color: var(--color-chip-red-text); }

	.war-button {
		background: var(--color-chip-red-bg);
		border-color: var(--color-chip-red-border);
		color: var(--color-chip-red-text);
	}
	.war-button:hover { background: color-mix(in srgb, var(--color-chip-red-bg) 92%, white); }

	/* Warning: only pending wars (declared, none active yet). Two states
	   total — red if any war is active, warning otherwise; the war drawer
	   carries the active/pending breakdown (COLOR_ROLES_PLAN ruling). */
	.war-button.war-pending {
		background: var(--color-warning-bg);
		border-color: var(--color-warning-border);
		color: var(--color-warning);
	}
	.war-button.war-pending:hover { background: color-mix(in srgb, var(--color-warning-bg) 92%, white); }


	.top-strip {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
		margin: 0 -1rem;
		padding: 0 1rem;
	}

	/* Below the chat dock the strip is fighting for every pixel: Home + Help
	   are a fixed 88px of the ~256px a 390 phone has, so the pills lose a
	   whole player's worth of room to chrome that isn't carrying any
	   information. Cancel the header's own inset (leaving main.full-bleed's
	   0.2rem hairline as the visual edge) and tighten the button-to-pill
	   gaps, which buys the strip ~32px — about half a pill. The buttons keep
	   their 44px touch targets; only the space around them shrinks.
	   Deliberately NOT taken from the pills' own padding, which reads as
	   cramped long before this does. Header rules are the one legitimate
	   place for viewport queries (the header spans the viewport) and this
	   uses a dock literal. */
	@media (max-width: 789px) {
		.top-strip {
			gap: 0.25rem;
			margin-inline: -0.75rem;
			padding-inline: 0;
		}
	}

	.home {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		flex-shrink: 0;
		color: var(--color-accent);
		border-radius: 6px;
		text-decoration: none;
	}
	.home:hover { color: var(--color-accent-hover); background: var(--color-surface-2); }
	.home:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* Wraps .members so the scroll-fade hint (::before/::after below) can sit
	   over its edges without being clipped or scrolled away by .members'
	   own overflow-x. */
	.members-wrap {
		position: relative;
		flex: 1;
		min-width: 0;
		/* Query container for .member-name's width budget below. The names
		   size against the STRIP, not the viewport — the strip is what they
		   actually have to share. */
		container-type: inline-size;
	}
	.members-wrap::before,
	.members-wrap::after {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		width: 20px;
		pointer-events: none;
		opacity: 0;
		transition: opacity 0.15s ease;
	}
	.members-wrap::before { left: 0; background: linear-gradient(to right, var(--color-bg), transparent); }
	.members-wrap::after { right: 0; background: linear-gradient(to left, var(--color-bg), transparent); }
	.members-wrap.fade-left::before { opacity: 1; }
	.members-wrap.fade-right::after { opacity: 1; }

	.members {
		display: flex;
		gap: var(--chip-gap);
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		scrollbar-width: none;

		/* `overflow-x: auto` makes computed `overflow-y` resolve to `auto` too
		   (CSS Overflow 3 §3: `visible` beside a non-visible axis computes to
		   `auto`), so this box CLIPS vertically even though nothing scrolls
		   that way. .risk-badge rides 6px above each pill and was losing its
		   top 3px to that edge. Reserve the overhang as padding and take it
		   straight back as margin, so the strip's outer box is unchanged and
		   the ::before/::after scroll fades still line up with the pills.
		   8px, not 6px: 2px of slack so the disc never lands flush on the
		   clip edge. Retune with .risk-badge's `top`. */
		padding-block: 8px;
		margin-block: -8px;

		/* Everything in a pill that ISN'T the name: horizontal padding (2x),
		   border (2x1), the colour dot, and the dot-to-name gap. Fed to
		   .member-name's budget below, so the two can't drift apart —
		   retune the padding and you must retune this. */
		--chip-gap: 0.4rem;
		--chip-chrome: 37.2px; /* 11.2*2 + 1*2 + 8 + 4.8 */
	}
	.members::-webkit-scrollbar { display: none; }

	.member {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		flex-shrink: 0;
		min-height: 44px;
		padding: 0.3rem 0.7rem;
		font-size: 0.85rem;
		color: var(--color-text);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border);
		border-radius: 999px;
		cursor: pointer;
	}

	/* Below the chat dock there isn't room for the roomier desktop padding —
	   a phone in main_event can have 3 pills with rank chips fighting the
	   fixed Home/Help buttons for ~240px. Tighten only down here; from the
	   dock up the full-width padding above just reads as breathing room. */
	@media (max-width: 789px) {
		.members {
			--chip-gap: 0.3rem;
			--chip-chrome: 29.2px; /* 7.2*2 + 1*2 + 8 + 4.8 */
		}
		.member { padding: 0.3rem 0.45rem; }
	}

	/* Warning badge: assets that are one tear from destruction but still have
	   empty marginalia slots to fill.
	 *
	 * Built as the marginalia slot it counts, shrunk to a circle: a dark
	 * ground, a 1px rim, and a numeral in the brighter step of the rim's
	 * family — exactly RetinueView's `.m-tile.empty.add.at-risk` (red-500 rim,
	 * red-300 "+"). Owner ruling 2026-07-30: matching the box it points at
	 * matters more than making your own count louder than other players'. Gold
	 * used to sit here and is now gone from the risk role entirely — on this
	 * strip gold means "waiting on" (see .member.waiting), nothing else.
	 *
	 * ONE treatment for every player, deliberately. This badge briefly rendered
	 * red on your own chip and grey on everyone else's; two colours for one
	 * metric read as two different metrics, and invited "why am I the only one
	 * warned?". The count means the same thing on every chip, so it looks the
	 * same on every chip — whose chip it is, is already said by the "You"
	 * label, and doesn't need saying twice.
	 *
	 * The rim is not decoration: the disc's ground is --color-surface-2, the
	 * same as the pill it sits on, so without a rim the two would merge where
	 * they overlap. */
	.risk-badge {
		position: absolute;
		/* -6, not -4: at 20px the disc would otherwise encroach on the name.
		   Pushed to the corner it clears the text entirely. .members reserves
		   8px of padding for this overhang. */
		top: -6px;
		right: -6px;
		z-index: 1;
		min-width: 20px;
		height: 20px;
		padding: 0 5px;
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 999px;
		/* 0.78rem/600 on a 20px disc, up from 0.7rem on 18px. Spectral is a
		   modulated serif: at 0.7rem its cap height is 7.4px and the thin
		   strokes of a numeral go sub-pixel, which reads as "faint" long
		   before the contrast ratio is the binding constraint. The extra
		   pixels do more for legibility here than any recolour. */
		font-size: 0.78rem;
		font-weight: 600;
		line-height: 1;
		font-variant-numeric: tabular-nums;
		background: var(--color-surface-2);
		/* The at-risk red (ruling 3: danger ≡ at-risk), in the same two steps
		   the marginalia slot uses: --color-danger-muted rim, --color-danger
		   numeral. 4.6:1 — AA at this size, and it's legible because the
		   numeral is the BRIGHT step; inverting it (red-500 numeral) or filling
		   the disc with red-500 both land near 3.8:1, under AA with no lighter
		   ink in the palette to rescue them. */
		border: 1px solid var(--color-danger-muted);
		color: var(--color-danger);
	}

	/* Name over a compact P/K/E rank line. The body is a column so the dot
	   stays vertically centred against both lines. */
	.member-body {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.12rem;
		min-width: 0;
	}
	.member-name-row {
		display: inline-flex;
		align-items: center;
		/* 0.3rem, not the 0.4rem used elsewhere: the dot reads as attached to
		   the name rather than as its own column, and across a 5-pill strip
		   the difference is ~8px of name. Mirrored in --chip-chrome. */
		gap: 0.3rem;
		min-width: 0;
	}

	.member-ranks {
		display: flex;
		gap: 0.4rem;
		font-size: 0.62rem;
		line-height: 1;
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.02em;
	}
	.member:hover { background: var(--color-border); }
	.member:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* The gold ring means "the game is waiting on this player" — a state that
	 * changes, not an identity that doesn't. It used to mark "you", which spent
	 * the strip's loudest treatment on the one fact you learn once and never
	 * need again, and read as a turn indicator to everyone who saw it. Identity
	 * moved to the "You" label + the colour dot; the ring says whose move it is.
	 *
	 * Two tiers, one hue: a plain border for other players (information), the
	 * border plus a glow for yourself (act now). Reserving the glow for your
	 * own turn is the whole point — a strip where every waiting chip glowed
	 * would be back to shouting at you about other people's business. */
	.member.waiting {
		border-color: var(--color-accent);
		color: var(--color-text);
	}
	.member.waiting.mine {
		box-shadow: 0 0 0 1px var(--color-accent), 0 0 8px color-mix(in srgb, var(--color-accent) 45%, transparent);
	}

	/*
	 * Name width budget — how much of a name we show before ellipsing.
	 *
	 * The old rule was a flat 10ch, which ignored the room actually
	 * available: on a 1440 desktop the strip has ~1300px, the pills use
	 * ~490, and names were still being clipped with 800px sitting empty.
	 *
	 * Instead, share the strip. `200cqw` is the policy knob: names may claim
	 * up to TWO strip-widths between them, i.e. we accept scrolling up to
	 * one full swipe before any name is cut. That's deliberate — a scrolled
	 * name is one gesture away, a truncated one is only recoverable by
	 * opening that player's retinue, so scrolling is the cheaper loss.
	 * Divide by the player count, subtract each pill's fixed chrome, and
	 * clamp:
	 *   floor 10ch — the old cap; below this a name stops being an
	 *                identifier (only reachable at 5 players on a 360 phone)
	 *   ceiling 26ch — a backstop against pathological wide-glyph names
	 *                (20 'W's is 534px); normal 20-char names sit under it
	 *
	 * Shakes out (390 phone): 2-3 players never truncate, 4 truncate past
	 * ~16 chars, 5 past ~12. Every docked width: never.
	 */
	.member-name {
		max-width: clamp(
			10ch,
			(200cqw - var(--member-count) * var(--chip-chrome)
				- (var(--member-count) - 1) * var(--chip-gap)) / var(--member-count),
			26ch
		);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--member-color, var(--color-text-muted));
		flex-shrink: 0;
	}

	/* :global — see ChatPanel; the element belongs to ErrorText now. */
	.table-page :global(.error) {
		padding: 0.5rem 0;
		flex-shrink: 0;
	}

	/* Error-boundary fallback. Inherits .center-message's fill-and-centre. */
	.boundary-failed {
		flex-direction: column;
		gap: 0.75rem;
		text-align: center;
		padding: 1rem;
	}
	.boundary-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		justify-content: center;
	}

	.center-message {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-text-muted);
	}

	/* ── Tone Setting ─────────────────────────────────────────────────────── */

	/* Lede callout: the `.prompt` recipe from SceneSetupForm. The lift is
	   deliberately all *shape* — a fill, a gold left rule, a labelled box.
	   Not bigger or bolder text: gold is a label and never a fill, and bold
	   is reserved for standalone numeric counters (docs/STYLE_GUIDE.md). */
	.tone-lede {
		background: var(--color-surface-active);
		border: 1px solid var(--color-border-warm);
		border-left: 3px solid var(--color-accent);
		border-radius: 5px;
		padding: 0.55rem 0.7rem;
		/* Short bottom margin on purpose: the legend below re-pays `.sheet-body`'s
		   padding as padding-top of its own (see `.tone-legend`), so the visible
		   gap between the two is that 0.95rem plus this. */
		margin: 0.5rem 0 0.15rem;
	}

	/* Locked, the sheet is reference material rather than something to act on,
	   so the callout goes cold — the same move `.scene-setup.readonly .prompt`
	   makes on the scene form. */
	.tone-lede.locked {
		background: var(--color-bg);
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid var(--color-border-strong);
	}
	.tone-lede p {
		margin: 0;
		font-size: 0.92rem;
		color: var(--color-text);
		line-height: 1.4;
	}

	/* Higher specificity than `.tone-lede p` above, so no !important needed. */
	.tone-lede p.tone-lede-locked {
		margin-top: 0.4rem;
		font-size: 0.85rem;
		font-style: italic;
		color: var(--color-text-secondary);
	}

	.tone-legend {
		position: sticky;
		/* NOT 0. The offset is measured from `.sheet-body`'s content-box top,
		   which sits 0.5rem below the sheet's inner edge — pinning at 0 left a
		   0.5rem band of tiles scrolling past in the open above the bar. Lifting
		   the pin by exactly that padding parks the bar flush against the edge.
		   Coupled to RetinueSheet's `.sheet-body` padding: change it there and
		   this (and the padding-top below, which re-pays the 0.5rem so the text
		   keeps its breathing room) has to change with it. */
		top: -0.5rem;
		/* Above the tiles, which are non-positioned. Belt-and-braces: the
		   sticky element would already paint over them on position alone. */
		z-index: 1;
		/* Column: the lead is a centred caption over the swatch row, not the
		   first item in it. */
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.8rem;
		/* Opaque, and padded on its own account: `.sheet-body`'s top padding
		   scrolls away with the content, so the bar has to own the whole band
		   it occupies or tiles show through above and touch it below. Matches
		   RetinueSheet's `.sheet` ground. */
		background: var(--color-surface-sunken);
		padding: 0.95rem 0 0.5rem;
		margin: 0 0 0.75rem;
		border-bottom: 1px solid var(--color-border-warm);
		color: var(--color-text-secondary);
	}

	/* Gold small-caps, matching the section headings and the lede's own label
	   — gold doing its one job here, labelling. */
	.tone-legend-lead {
		font-size: 0.7rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	/* 0.55rem, not the 0.7rem the flat row ran at — see the markup comment: the
	   four swatches clear a 360 column by ~10px, and this gap is where that
	   margin came from. Wraps rather than overflows if it ever runs out. */
	.tone-legend-items {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: 0.3rem 0.55rem;
	}

	.tone-legend-item {
		display: inline-flex;
		align-items: center;
		/* Also trimmed from 0.4rem for the 360 fit (4 items = 4 of these). */
		gap: 0.32rem;
	}

	.tone-legend-item .swatch {
		width: 0.9rem;
		height: 0.9rem;
		border-radius: 3px;
		border: 1px solid rgba(255,255,255,0.1);
	}

	.tone-legend-item[data-status='default']      .swatch { background: var(--color-neutral); }
	.tone-legend-item[data-status='include']      .swatch { background: var(--color-tone-include); }
	.tone-legend-item[data-status='avoid_detail'] .swatch { background: var(--color-tone-avoid); }
	.tone-legend-item[data-status='never']        .swatch { background: var(--color-danger-muted); }

	.tone-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}

	/* Always 3 across: the tones sheet is a content column capped at 440
	   (docs/STYLE_GUIDE.md "Layout widths"), where 4–5 tiles never fit. */
	.tone-tile {
		min-height: 44px;
		padding: 0.35rem 0.4rem;
		border-radius: 6px;
		border: 1px solid rgba(255,255,255,0.08);
		background: var(--color-neutral);
		color: var(--white);
		font-size: 0.85rem;
		font-weight: 500;
		text-align: center;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		transition: background-color 120ms ease, transform 80ms ease;
		word-break: break-word;
		hyphens: auto;
	}

	.tone-tile:active { transform: scale(0.97); }
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): drop the
	   press-down squish, keep the colour fade — the background *is* the tone's
	   status, so it is feedback, not motion. */
	@media (prefers-reduced-motion: reduce) {
		.tone-tile { transition: background-color 120ms ease; }
		.tone-tile:active { transform: none; }
	}

	.tone-tile[data-status='default']      { background: var(--color-neutral); }
	.tone-tile[data-status='include']      { background: var(--color-tone-include); }
	.tone-tile[data-status='avoid_detail'] { background: var(--color-tone-avoid); }
	.tone-tile[data-status='never']        { background: var(--color-danger-muted); }

	.tone-tile-topic { line-height: 1.2; }

	.tone-add-row {
		display: flex;
		gap: 0.5rem;
		align-items: stretch;
	}

	.tone-add-input {
		flex: 1 1 auto;
		min-width: 0;
		padding: 0.6rem 0.75rem;
		background: var(--color-surface-2);
		border: 1px dashed rgba(255,255,255,0.35);
		border-radius: 6px;
		color: var(--color-text);
		font-family: inherit;
		font-size: 0.9rem;
	}
	.tone-add-input::placeholder { color: color-mix(in srgb, var(--color-text) 50%, transparent); }
	.tone-add-input:focus {
		outline: none;
		border-style: solid;
		border-color: rgba(255,255,255,0.6);
	}

	.tone-add-button {
		flex: 0 0 auto;
		min-width: 5.5rem;
		min-height: 44px;
		padding: 0 1rem;
		/* -ink, not --color-info: this is a filled button with white label text,
		   and --color-info is a *fill against dark grounds*, not a ground for
		   light text — white on it managed only 3.3:1 (AA needs 4.5 at this
		   size). Caught during the 2026-08-01 blue retune, which would have
		   nudged it to 3.2; the dark step fixes it outright at 6.4:1. */
		background: var(--color-highlight-ink);
		color: var(--white);
		border: 1px solid rgba(255,255,255,0.12);
		border-radius: 6px;
		font-size: 0.9rem;
		cursor: pointer;
	}
	.tone-add-button:disabled { opacity: 0.5; cursor: not-allowed; }

</style>
