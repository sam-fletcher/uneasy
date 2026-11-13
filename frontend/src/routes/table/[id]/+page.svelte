<!--
  Game shell: loads full game state, routes to phase-specific views, and frames
  them. The frame is a title bar (Home, the table's title, Help) and the player
  row, both the full width, above three panes: the Public Record, Play and Chat.
  A wide window shows the panes as columns and a narrow one swaps them with
  tabs (see lib/breakpoints.ts and docs/STYLE_GUIDE.md "Layout widths"). The data and the
  live updates live here; the pieces of the frame are in lib/components/table/.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/modalShell.css';
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
		markLawsSeen, markRumorsSeen, listLaws, listRumors,
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
	import { readTableSnapshot, writeTableSnapshot, type TableSnapshot } from '$lib/pageCache';
	import { logBoundaryError } from '$lib/boundaryError';
	import { handleWSMessage as runWSMessage, syncActiveRoll, type WSContext } from './ws-handlers';
	import {
		reconnectResync, resolveAnchor, enterHistoryMode, countUnread, type ChatFeedContext,
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
	import TableTitleBar from '$lib/components/table/TableTitleBar.svelte';
	import PlayerRow from '$lib/components/table/PlayerRow.svelte';
	import ShellColumn from '$lib/components/table/ShellColumn.svelte';
	import PaneName from '$lib/components/table/PaneName.svelte';
	import PaneTabs from '$lib/components/table/PaneTabs.svelte';
	import RecordTabs from '$lib/components/table/RecordTabs.svelte';
	import TonesSheet from '$lib/components/table/TonesSheet.svelte';
	import WarEntry from '$lib/components/table/WarEntry.svelte';
	import {
		leftPaneOf, panesShown, tabBarFor, paneMarksFor,
		type PaneId, type PaneMarkState, type RecordTab,
	} from '$lib/components/table/panes';
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
	import { tableTitle } from '$lib/tableTitle';
	import { tableLayout, watchTableLayout } from '$lib/tableLayout.svelte';
	import { provideSheetHost } from '$lib/sheetHost';
	import { hasNewEntries, seenThrough } from '$lib/lawsRumorsSeen';
	import { provideSecretCounts } from '$lib/secretCountsContext';
	import { provideSuccession } from '$lib/successionContext';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import { atRiskCountByPlayer, typingIndicatorLabel } from '$lib/tableHeader';

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
	// Separate from HelpButton's own Feedback sheet — this one's trigger lives
	// in the lobby phase's inline (unsheeted) HelpContent, not behind the "?".
	let lobbyFeedbackOpen = $state(false);
	let prologueActivePlayerID = $state<number | null>(null);

	// ── Lobby push soft-ask ──────────────────────────────────────────────────
	// vapidPublicKey is fetched once here (from getMe, alongside the rest of
	// onMount's auth check) and handed to LobbyView, which owns the rest of
	// the push-enable flow (its own pushState/dismissal/enable logic).
	let vapidPublicKey = $state('');

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

	// Every player the game is currently waiting on — drives the clock on the
	// player row's chips, and whether Play's mark is lit. Derived from
	// `waitingOn`, NOT blockingPlayerID: the latter is phase-specific and
	// single-player, while a row can be waiting on several people at once (see
	// waitingOn.ts / ResolvingWaitees). Sharing the bar's source is the point —
	// the clock and the "Waiting On:" list can never disagree.
	//   'everyone' marks everyone: going dark there would read as "not you".
	//   'label' waitees (e.g. "facilitator to start") name no player → mark none.
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
	// ── Panes ─────────────────────────────────────────────────────────────────
	// The table's three panes show as one, two or three columns, by viewport
	// width (lib/tableLayout). Below two columns a pane is a tab and `activePane`
	// is the one on screen. At two, Chat is docked and the left column shows
	// Public Record or Play. At three they all show. The table opens on Play, the
	// pane people return for, and the pane you pick is remembered only for this
	// visit: none of it is persisted and none of it is history, so Back leaves the
	// table instead of stepping through tabs.
	let activePane = $state<PaneId>('play');
	// What the Public Record pane shows. Opening Laws or Rumors is what marks
	// that list as seen.
	let recordTab = $state<RecordTab>('events');

	const layoutMode = $derived(tableLayout.mode);
	// Which panes are on screen, and the tab bar that goes with it. Both are
	// pure functions of the layout and the chosen pane (lib/components/table/panes).
	const leftPane = $derived(leftPaneOf(activePane));
	const paneShown = $derived(panesShown(layoutMode, activePane));
	const tabBar = $derived(tabBarFor(layoutMode, activePane));
	const tabPanes = $derived(tabBar.panes);
	const tabCurrent = $derived(tabBar.current);
	// From three columns up each column starts with its name, since no tab names
	// it. At two, only Chat's header does; on a phone the tab bar names them all.
	const nameColumns = $derived(layoutMode === 'three');

	async function selectPane(pane: PaneId) {
		activePane = pane;
		// The tab bar is drawn in the foot of whichever pane is showing, so the
		// button that was pressed is gone once the pane changes. Put focus on the
		// same tab in the new bar, or a keyboard or screen-reader user is dropped
		// back to the top of the page on every switch.
		await tick();
		document
			.querySelector<HTMLElement>(`.panes nav[aria-label="Panes"] [data-pane="${pane}"]`)
			?.focus();
	}
	function showRecordTab(tab: RecordTab) {
		activePane = 'record';
		recordTab = tab;
	}

	// The columns, for sheets to cover (lib/sheetHost). Header sheets (Help, a
	// player's retinue) cover Play, or the left column when there are two; on a
	// phone there is one column, so the answer is "the screen".
	let recordColumn = $state<HTMLElement | null>(null);
	let playColumn = $state<HTMLElement | null>(null);
	let chatColumn = $state<HTMLElement | null>(null);
	provideSheetHost(() =>
		layoutMode === 'three' ? playColumn
		: layoutMode === 'two' ? (leftPane === 'record' ? recordColumn : playColumn)
		: null
	);

	// ── Marks on the tabs and column names ────────────────────────────────────
	// Two marks with one meaning each. The clock on Play means it is YOUR move
	// (the player row already says whose move it is for everyone else); the pale
	// dot means something new since you last had that pane on screen. Neither
	// carries a count.
	const isMyMove = $derived(currentPlayerID != null && waitingPlayerIDs.has(currentPlayerID));
	const hasUnreadChat = $derived(countUnread(chatFeedPosts, chatLastReadPostID, currentPlayerID) > 0);

	// ── Laws and Rumors: what is new to me ────────────────────────────────────
	// The server keeps the highest `rev` of each list this player has seen
	// (lib/lawsRumorsSeen). null until the first load says, so nothing lights up
	// for a marker nobody has read yet. Not snapshotted: a table painted from
	// the cache shows no dots until the load below has landed.
	let lawsSeenRev = $state<number | null>(null);
	let rumorsSeenRev = $state<number | null>(null);
	const lawsNew = $derived(lawsSeenRev != null && hasNewEntries(laws, lawsSeenRev));
	const rumorsNew = $derived(rumorsSeenRev != null && hasNewEntries(rumors, rumorsSeenRev));
	const paneMarks = $derived<PaneMarkState>(
		paneMarksFor({ lawsNew, rumorsNew, yourMove: isMyMove, unreadChat: hasUnreadChat })
	);

	// Opening a list (with the Public Record on screen) is what marks it seen.
	// The highest rev already reported per list is kept so a reply that lands
	// lower than asked (the server clamps) can't set off a loop of requests.
	const reportedSeen = { laws: 0, rumors: 0 };
	$effect(() => {
		if (!paneShown.record || recordTab === 'events') return;
		const kind = recordTab;
		const seen = kind === 'laws' ? lawsSeenRev : rumorsSeenRev;
		if (seen == null) return;
		const top = seenThrough(kind === 'laws' ? laws : rumors, seen);
		if (top == null || top <= reportedSeen[kind]) return;
		reportedSeen[kind] = top;
		const report = kind === 'laws' ? markLawsSeen : markRumorsSeen;
		report(gameID, top)
			.then((r) => {
				if (kind === 'laws') lawsSeenRev = Math.max(lawsSeenRev ?? 0, r.seen_rev);
				else rumorsSeenRev = Math.max(rumorsSeenRev ?? 0, r.seen_rev);
			})
			// Best effort: a miss only leaves a dot up. Forget the attempt so the
			// next change to the list tries again.
			.catch(() => { reportedSeen[kind] = 0; });
	});

	// Your own writing isn't new to you: the server moves the writer's marker
	// along with the write, but the client only hears about the entry. So when a
	// list gains something above my marker, ask the server for the marker again
	// (after a beat, in case the event overtook the commit). Once per newest rev,
	// so someone else's entry, which leaves the marker where it was, doesn't
	// start a loop.
	const checkedTop = { laws: 0, rumors: 0 };
	// One pending re-read per list. A newer entry replaces the one waiting, and
	// leaving the page cancels both.
	const markerRereads: { laws?: ReturnType<typeof setTimeout>; rumors?: ReturnType<typeof setTimeout> } = {};
	$effect(() => {
		for (const kind of ['laws', 'rumors'] as const) {
			const seen = kind === 'laws' ? lawsSeenRev : rumorsSeenRev;
			if (seen == null) continue;
			const top = (kind === 'laws' ? laws : rumors).reduce((m, e) => Math.max(m, e.rev), 0);
			if (top <= seen || top <= checkedTop[kind]) continue;
			checkedTop[kind] = top;
			clearTimeout(markerRereads[kind]);
			markerRereads[kind] = setTimeout(() => {
				const read = kind === 'laws' ? listLaws(Number(gameID)) : listRumors(Number(gameID));
				void read
					.then((r) => {
						if (kind === 'laws') lawsSeenRev = Math.max(lawsSeenRev ?? 0, r.seen_rev);
						else rumorsSeenRev = Math.max(rumorsSeenRev ?? 0, r.seen_rev);
					})
					.catch(() => { /* best effort */ });
			}, 500);
		}
	});

	// The table's one title ("Row 6 · Seek Answers"). The plan comes from the
	// row state, which the server decides once, so this and the profile page's
	// row for this table cannot disagree.
	const title = $derived(
		game
			? tableTitle({
					phase: game.phase,
					current_row: game.current_row,
					plan_type: rowState?.title_plan_type ?? null,
					prologue_ranking_step: game.prologue_ranking_step,
					ending_vote_open: game.ending_vote_open,
					shake_up_category: game.shake_up_category,
				})
			: ''
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

	// Per-player count of "needlessly at-risk" assets — the warning mark on each
	// player's chip (the avoidable case; see isNeedlesslyAtRisk). Only whether
	// it is above zero is drawn; the count itself is in the retinue.
	// Suppressed in the lobby: every joiner starts with a bare [Main Character]
	// peer, so the mark would fire on all chips at once and greet a new player
	// with three alarms on the screen that means "relax". Main-character
	// authoring stays available there (the retinue's marginalia "+" buttons
	// work) — it just isn't advertised until the prologue, where the player has
	// the context for it (ADR LOBBY_AND_CHECKLIST R3/D7).
	const atRiskByPlayer = $derived(
		game?.phase === 'lobby' ? new Map<number, number>() : atRiskCountByPlayer(assets)
	);

	// ── Public Record → Chat jump bridge ──────────────────────────────────────
	// Tapping a row/plan/scene in the Public Record resolves the anchoring
	// system post (Chat Overhaul Phase 2e: the loaded window first, cheap;
	// then the Phase 1d anchor endpoint) and pushes a request to ChatPanel,
	// which scrolls there (on a phone, jumpTo switches to the Chat tab first). If the
	// anchor wasn't already loaded, this also switches the feed into history
	// mode via an around-fetch — ChatPanel's "Return to now" button gets the
	// player back to the live tail afterward.
	let chatJumpRequest = $state<{ postID: number; key: number } | null>(null);
	let jumpKey = 0;
	function jumpTo(postID: number) {
		// On a phone the chat is another tab, so go there; in columns it is
		// already beside the record and just scrolls.
		if (layoutMode === 'phone') activePane = 'chat';
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

	// ── Snapshot (stale-while-revalidate) ────────────────────────────────────
	// captureSnapshot/applySnapshot are exact mirrors; every field one writes
	// the other reads. Keeping them adjacent and total (no partial snapshots)
	// is what stops a seeded page from rendering a half-populated table — the
	// derived-throws-and-freezes-reactivity failure that the asset.taken bug
	// produced. If you add server-owned state above, add it to both.
	function captureSnapshot(): TableSnapshot {
		return {
			game, players, toneTopics, rankings, assets, laws, rumors, members,
			playerActivity, secrets, currentPlayerID, prologueActivePlayerID,
			recordRows, plans, planTokens, activeScene, activeScenePeers, rowState,
			bankedDice, activeRoll, activeRollDice, activeRollVotes,
			activeRollParticipants,
			// A history window is not restorable (see the note on TableSnapshot),
			// so snapshot an empty feed instead and let the next mount load a
			// fresh one.
			chatPosts: chatFeedMode === 'live' ? chatFeedPosts : [],
			chatHasMoreBefore: chatFeedMode === 'live' && chatHasMoreBefore,
			chatLastReadPostID, chatInitialReadMarker,
		};
	}

	function applySnapshot(s: TableSnapshot) {
		game = s.game; players = s.players; toneTopics = s.toneTopics;
		rankings = s.rankings; assets = s.assets; laws = s.laws; rumors = s.rumors;
		members = s.members; playerActivity = s.playerActivity; secrets = s.secrets;
		currentPlayerID = s.currentPlayerID;
		prologueActivePlayerID = s.prologueActivePlayerID;
		recordRows = s.recordRows; plans = s.plans; planTokens = s.planTokens;
		activeScene = s.activeScene; activeScenePeers = s.activeScenePeers;
		rowState = s.rowState; bankedDice = s.bankedDice;
		activeRoll = s.activeRoll; activeRollDice = s.activeRollDice;
		activeRollVotes = s.activeRollVotes;
		activeRollParticipants = s.activeRollParticipants;
		chatFeedPosts = s.chatPosts;
		chatHasMoreBefore = s.chatHasMoreBefore;
		chatLastReadPostID = s.chatLastReadPostID;
		chatInitialReadMarker = s.chatInitialReadMarker;
	}

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
			// Assign-only, for the same reason as player_activity below: the
			// payload is best-effort, and a resync that lost that one query
			// shouldn't blank the dots.
			if (data.laws_rumors_seen) {
				lawsSeenRev = data.laws_rumors_seen.laws;
				rumorsSeenRev = data.laws_rumors_seen.rumors;
			}
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
				const [recordData, , plansData, tokensData, sceneData, bankedData] = await Promise.all([
					getFullRecord(gameID),
					// Installs the roll itself, guarded against the live roll events
					// and fetches it races — see syncActiveRoll.
					syncActiveRoll(wsCtx, getActiveRollForGame(gameID)),
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
				const [, endRecord] = await Promise.all([
					syncActiveRoll(wsCtx, getActiveRollForGame(gameID).catch(() => null), { votes: false }), /* tolerate; RollCreated/RollResolved WS events keep this in sync */
					getFullRecord(gameID).catch(() => null), /* tolerate; the sidebar just shows what's already loaded */
				]);
				if (endRecord) recordRows = endRecord.rows;
			}

			gameStateLoaded = true;
			// Written on success only, so a failed refresh never overwrites a
			// good snapshot with a partial one.
			writeTableSnapshot(gameID, captureSnapshot());
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

	onMount(() => watchTableLayout());

	onMount(() => {
		const onVisible = () => {
			if (document.visibilityState === 'visible') pingActivity();
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => document.removeEventListener('visibilitychange', onVisible);
	});

	onMount(async () => {
		try {
			// Paint last-known state before touching the network. The load below
			// runs regardless and overwrites all of it — this only decides
			// whether the player watches a spinner for a second or looks at
			// their table while it refreshes.
			//
			// `loading` is what gates the whole render, so clearing it here is
			// the entire point. `gameStateLoaded` is deliberately NOT set: it
			// governs error reporting, and a snapshot is not evidence that the
			// server can still be reached, so a failed first load must still
			// raise its banner — over the stale content, which is honest.
			const cached = readTableSnapshot(gameID);
			if (cached) {
				applySnapshot(cached);
				loading = false;
			}

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
		clearTimeout(markerRereads.laws);
		clearTimeout(markerRereads.rumors);
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
	// Rendered inside the Tones sheet, not in the page banner. The sheet is a
	// scrim and a raised panel over its column, so a message in the page banner
	// is painted underneath the dialog the player is looking at — they'd see the
	// topic silently fail to change with the dialog still open. Cleared when the
	// sheet opens.
	let toneError = $state('');

	/** Both entry points to the Tones sheet (the chat header's chip and the
	 *  lobby's and prologue's own links) go through here so a message left over
	 *  from a previous visit never greets the player on open. */
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

	/** Adds a custom topic. Resolves true if it was added, so the sheet can
	 *  clear its box. */
	async function addTopic(text: string): Promise<boolean> {
		toneError = '';
		try {
			await addToneTopic(gameID, text);
			return true;
		} catch (e) {
			toneError = e instanceof Error ? e.message : 'Could not add topic.';
			return false;
		}
	}

</script>

<div class="table-page" data-layout={layoutMode}>
	<TableTitleBar {title} gameId={gameID} route={page.url.pathname} phase={game?.phase} />

	<PlayerRow
		{players}
		{currentPlayerID}
		{waitingPlayerIDs}
		{atRiskByPlayer}
		onSelect={(playerID) => (retinueOpenForPlayer = playerID)}
	/>

	{#if loadError}
		<ErrorText message={loadError} extra="error" />
	{/if}

	<!--
		The three panes, left to right: Public Record, Play, Chat. Wide windows show
		them as columns and narrow ones swap them with tabs, but all three stay
		mounted either way (a pane that isn't showing is only hidden), so the chat
		keeps its place in the feed while you are on Play. `data-layout` is how the
		stylesheet below knows which; it comes from lib/tableLayout, the one place
		the breakpoints are read.
	-->
	<div class="panes" data-layout={layoutMode}>

		<!-- Public Record ───────────────────────────────────────────────────────
		     Events, Laws or Rumors, chosen by the chips at its foot. Shown in
		     every phase: in the Prologue Events is the thirteen blank rows while
		     Laws and Rumors fill up, and the lobby gets no special treatment. -->
		<ShellColumn
			pane="record"
			label="Public Record"
			shown={paneShown.record}
			bind:el={recordColumn}
			head={nameColumns ? recordHead : undefined}
			foot={recordFoot}
		>
			{#if !loading && game}
				{#if recordTab === 'events'}
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
				{:else}
					<div class="record-list">
						<LawsRumors
							kind={recordTab}
							{laws}
							{rumors}
							{plans}
							{players}
							playerNames={playerNameMap}
							{currentPlayerID}
						/>
					</div>
				{/if}
			{/if}
		</ShellColumn>

		<!-- Play ────────────────────────────────────────────────────────────────
		     The Waiting On bar and the way into wars sit at the top, unchanged,
		     until the Play and war pieces replace them. -->
		<ShellColumn
			pane="play"
			label="Play"
			shown={paneShown.play}
			bind:el={playColumn}
			head={nameColumns ? playHead : undefined}
			foot={playFoot}
		>
			<div class="phase-column">
				{#if !loading && game}
					<WaitingOnBar state={waitingOn} {currentPlayerID} {players} />
					<WarEntry />
				{/if}

				<!--
					A throw during render used to blank the page with no message: there was no
					boundary anywhere in the app, and the two incidents we've had (the
					asset.taken payload that had to be merged rather than replaced, and the
					optimistic-append/WS duplicate key) both presented as "the UI froze"
					rather than as an error. Scoped to the phase views only — the title bar,
					player row, Waiting On bar, Public Record and chat stay alive, so the
					player can still read the table and reach the reload. `reset` re-renders:
					worth offering first, since a bad payload is usually repaired by the next
					resync.
				-->
				<svelte:boundary onerror={(error) => logBoundaryError(`table phase view (${game?.phase ?? 'no game'})`, error)}>
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
							onOpenLaws={() => showRecordTab('laws')}
							onOpenRumors={() => showRecordTab('rumors')}
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
		</ShellColumn>

		<!-- Chat ────────────────────────────────────────────────────────────────
		     Tones open from the chat header and rise over this column. -->
		<ShellColumn
			pane="chat"
			label="Chat"
			shown={paneShown.chat}
			bind:el={chatColumn}
			foot={chatFoot}
		>
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
					{plans}
					jumpRequest={chatJumpRequest}
					visible={paneShown.chat}
					showName={layoutMode !== 'phone'}
					{tonesSetCount}
					onOpenTones={openTones}
				/>
			{/if}

			<TonesSheet
				open={tonesOpen}
				onClose={() => tonesOpen = false}
				topics={toneTopics}
				locked={tonesLocked}
				error={toneError}
				onCycle={cycleTopicStatus}
				onAdd={addTopic}
			/>
		</ShellColumn>
	</div>

	<RetinueSheet open={lobbyFeedbackOpen} onClose={() => lobbyFeedbackOpen = false}>
		<div class="feedback-sheet">
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

<!-- What a column's name bar and foot hold. A name bar only from three columns up,
     where no tab names the pane. The foot is the tab bar the layout calls for, and
     for the Public Record the Events / Laws / Rumors chips above it. Only the pane
     that is showing draws its foot, so the page never holds a second, hidden copy
     of a tab bar. -->
{#snippet recordHead()}
	<PaneName pane="record" marked={paneMarks.record} />
{/snippet}

{#snippet playHead()}
	<PaneName pane="play" marked={paneMarks.play} />
{/snippet}

{#snippet recordFoot()}
	{#if paneShown.record}
		<RecordTabs current={recordTab} {lawsNew} {rumorsNew} onSelect={(tab) => (recordTab = tab)} />
		{#if tabPanes.length > 0}
			<PaneTabs panes={tabPanes} current={tabCurrent} marks={paneMarks} onSelect={selectPane} />
		{/if}
	{/if}
{/snippet}

{#snippet playFoot()}
	{#if paneShown.play && tabPanes.length > 0}
		<PaneTabs panes={tabPanes} current={tabCurrent} marks={paneMarks} onSelect={selectPane} />
	{/if}
{/snippet}

<!-- Chat has a tab bar only on a phone: from two columns up it is docked. -->
{#snippet chatFoot()}
	{#if paneShown.chat && layoutMode === 'phone'}
		<PaneTabs panes={tabPanes} current={tabCurrent} marks={paneMarks} onSelect={selectPane} />
	{/if}
{/snippet}

<style>
	/* ── Layout ─────────────────────────────────────────────────────────────── */

	.table-page {
		display: flex;
		flex-direction: column;
		/* Fills main.full-bleed, which the layout sizes to the viewport minus
		   whatever the update banner is taking (see body:has in +layout.svelte). */
		height: 100%;
		max-width: 100%;
	}

	/* The panes. A column is drawn at phone width, between 360 and 440: columns
	   share spare room equally up to the top of that band, and past it the group
	   is centred
	   (docs/STYLE_GUIDE.md "Layout widths"). Their gaps, and the padding beside
	   the outer two, are one --space-2; the breakpoints in lib/breakpoints.ts are
	   where that many columns fit.

	   One row, always: the columns are as tall as the space under the player row
	   and scroll inside. Panes that share the screen (all three on a phone; the
	   left column's two at two columns) sit in the same grid cell and are told
	   apart by ShellColumn hiding the ones that aren't shown. */
	.panes {
		flex: 1;
		min-height: 0;
		display: grid;
		grid-template-rows: minmax(0, 1fr);
		justify-content: center;
	}

	/* A phone: one column, capped at the top of the band and centred. */
	.panes[data-layout='phone'] {
		grid-template-columns: minmax(0, var(--layout-column-max));
	}
	.panes[data-layout='phone'] > :global(.shell-column) {
		grid-area: 1 / 1;
	}

	/* Two columns: Chat docked on the right; the left column is Public Record or
	   Play. */
	.panes[data-layout='two'] {
		grid-template-columns: repeat(2, minmax(var(--layout-column-min), var(--layout-column-max)));
		column-gap: var(--space-2);
		padding-inline: var(--space-2);
	}
	.panes[data-layout='two'] > :global(.shell-column[data-pane='record']),
	.panes[data-layout='two'] > :global(.shell-column[data-pane='play']) {
		grid-area: 1 / 1;
	}
	.panes[data-layout='two'] > :global(.shell-column[data-pane='chat']) {
		grid-area: 1 / 2;
	}

	/* Three columns, side by side. */
	.panes[data-layout='three'] {
		grid-template-columns: repeat(3, minmax(var(--layout-column-min), var(--layout-column-max)));
		column-gap: var(--space-2);
		padding-inline: var(--space-2);
	}

	/* Columns have no boxes: only a thin line in the gutter between them. */
	.panes[data-layout='two'] > :global(.shell-column[data-pane='chat'])::before,
	.panes[data-layout='three'] > :global(.shell-column[data-pane='play'])::before,
	.panes[data-layout='three'] > :global(.shell-column[data-pane='chat'])::before {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: calc(-1 * var(--space-1) - 0.5px);
		width: 1px;
		background: var(--color-border);
		pointer-events: none;
	}

	/* The play column's content. WaitingOnBar and WarEntry sit above the phase
	   view in it, so they share the column's width and not the other panes'.
	   A named query container: components inside adapt to the COLUMN's width via
	   @container column (…), never to the viewport (docs/STYLE_GUIDE.md "Layout
	   widths"). */
	.phase-column {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		container: column / inline-size;
	}

	/* Laws and Rumors: plain lists that scroll inside the column. */
	.record-list {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: var(--space-3) var(--space-2);
	}

	/* :global — see ChatPanel; the element belongs to ErrorText now. */
	.table-page :global(.error) {
		padding: var(--space-2) var(--space-4);
		flex-shrink: 0;
	}

	/* Error-boundary fallback. Inherits .center-message's fill-and-centre. */
	.boundary-failed {
		flex-direction: column;
		gap: var(--space-3);
		text-align: center;
		padding: var(--space-4);
	}
	.boundary-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		justify-content: center;
	}

	.center-message {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-text-muted);
	}
</style>
