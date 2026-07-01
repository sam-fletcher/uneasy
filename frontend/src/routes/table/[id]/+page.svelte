<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/rankChip.css';
	import '$lib/components/shared/statusText.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, onDestroy } from 'svelte';
	import {
		getGameState, getMe,
		startPrologue,
		updateToneTopic, addToneTopic,
		listAssets, getFullRecord, listGamePosts,
		getActiveRollForGame, listBankedDice,
		listPlans, listPlanTokens,
		setEndgameMode,
		getVisibleSecrets,
		getActiveScene,
		type RankingCategory,
		type EndgameMode,
		type Scene,
		type ScenePeerView,
		type SceneSetupDraft,
		type PreparePlanDraft,
		type RowState,
	} from '$lib/api';
	import { createConnection, type WSMessage } from '$lib/ws';
	import { handleWSMessage as runWSMessage, type WSContext } from './ws-handlers';
	import type {
		Game, Player, ToneTopic, Ranking, Asset, Marginalium,
		Law, Rumor,
		ChatPost, SceneEntry, RecordRow, PresenceMember,
		DiceRoll, DiceRollDie, VoteView, RollParticipant, BankedDie,
		Plan, PlanToken, Secret,
	} from '$lib/api';
	import MainEventView from '$lib/components/phases/MainEventView.svelte';
	import PublicRecord from '$lib/components/PublicRecord.svelte';
	import PrologueView from '$lib/components/phases/PrologueView.svelte';
	import ShakeUpView from '$lib/components/phases/ShakeUpView.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import LawsRumors from '$lib/components/LawsRumors.svelte';
	import RetinueView from '$lib/components/RetinueView.svelte';
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import HelpContent from '$lib/components/HelpContent.svelte';
	import WaitingOnBar, { type WaitingOnState } from '$lib/components/WaitingOnBar.svelte';
	import { playerColorByID } from '$lib/playerColor';
	import { warDrawerOpen, activeWarCount, pendingWarCount } from '$lib/warDrawer';
	import { provideSecretCounts } from '$lib/secretCountsContext';
	import { provideSuccession } from '$lib/successionContext';
	import {
		rankTriplesByPlayer, topRanks, atRiskCountByPlayer, typingIndicatorLabel,
	} from '$lib/tableHeader';

	const gameID = $derived(page.params.id as string);

	// ── Core state ────────────────────────────────────────────────────────────
	let game = $state<Game | null>(null);
	let endgamePromptModes = $state<EndgameMode[] | null>(null);
	let endgameSubmitting = $state(false);
	let players = $state<Player[]>([]);
	let toneTopics = $state<ToneTopic[]>([]);
	let rankings = $state<Ranking[]>([]);
	let assets = $state<Asset[]>([]);
	let laws = $state<Law[]>([]);
	let rumors = $state<Rumor[]>([]);
	let members = $state<PresenceMember[]>([]);
	let secrets = $state<Secret[]>([]);
	let currentPlayerID = $state<number | null>(null);
	let error = $state('');
	let loading = $state(true);

	// Derived helpers
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
	let chatPosts = $state<ChatPost[]>([]);

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
	let helpOpen = $state(false);
	let prologueActivePlayerID = $state<number | null>(null);

	// ── Join-code copy feedback ───────────────────────────────────────────────
	// Briefly flips the badge label to "Copied!" after a successful copy.
	let joinCodeCopied = $state(false);
	let joinCodeCopyTimer: ReturnType<typeof setTimeout> | null = null;
	async function copyJoinCode() {
		if (!game) return;
		try {
			await navigator.clipboard.writeText(game.join_code);
			joinCodeCopied = true;
			if (joinCodeCopyTimer) clearTimeout(joinCodeCopyTimer);
			joinCodeCopyTimer = setTimeout(() => (joinCodeCopied = false), 1500);
		} catch {
			// Clipboard can reject (permissions / insecure context); leave the
			// label unchanged so the user can still read & copy manually.
		}
	}

	// ── Mobile chat sheet ─────────────────────────────────────────────────────
	// Bound to ChatPanel's `expanded`. Kept here so the page can enforce one
	// full-screen surface at a time on mobile: opening any header panel closes
	// the chat (so the panel doesn't render behind the higher-z chat sheet),
	// and tapping the header bar dismisses the chat.
	let chatExpanded = $state(false);
	function closeChatSheet() {
		if (chatExpanded) chatExpanded = false;
	}

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
			stepLabel: 'Start the game',
		};
	});
	$effect(() => {
		if (game?.phase === 'lobby') waitingOn = lobbyWaitingOn;
		else if (game?.phase === 'ended') waitingOn = { waitees: [] };
	});
	const tonesLocked = $derived(
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
	const atRiskByPlayer = $derived(atRiskCountByPlayer(assets));

	// ── Public Record → Chat jump bridge ──────────────────────────────────────
	// Tapping a row/plan/scene in the expanded sidebar finds the anchoring
	// system post in chatPosts and pushes a request to ChatPanel, which
	// scrolls there (and on mobile, expands itself first).
	let chatJumpRequest = $state<{ postID: number; key: number } | null>(null);
	let jumpKey = 0;
	function jumpTo(postID: number) {
		chatJumpRequest = { postID, key: ++jumpKey };
	}
	function jumpToRow(rowNumber: number) {
		const anchor = chatPosts.find(p =>
			p.system_code === 'row.advanced' && p.row_number === rowNumber
		);
		if (anchor) { jumpTo(anchor.id); return; }
		// Row 1 has no row.advanced post — fall back to the first post.
		if (rowNumber === 1 && chatPosts.length > 0) jumpTo(chatPosts[0].id);
	}
	function jumpToPlan(planID: number) {
		const anchor = chatPosts.find(p =>
			p.system_code === 'plan.prepared' && p.plan_id === planID
		);
		if (anchor) jumpTo(anchor.id);
	}
	function jumpToScene(rowNumber: number) {
		// SceneEntry doesn't carry scene_id — anchor by row's first scene.started.
		const anchor = chatPosts.find(p =>
			p.system_code === 'scene.started' && p.row_number === rowNumber
		);
		if (anchor) jumpTo(anchor.id);
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
		get rankings() { return rankings; }, set rankings(v) { rankings = v; },
		get assets() { return assets; }, set assets(v) { assets = v; },
		get laws() { return laws; }, set laws(v) { laws = v; },
		get rumors() { return rumors; }, set rumors(v) { rumors = v; },
		get secrets() { return secrets; }, set secrets(v) { secrets = v; },
		get chatPosts() { return chatPosts; }, set chatPosts(v) { chatPosts = v; },
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
	async function loadGameState() {
		try {
			const data = await getGameState(gameID);
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

			// Load assets in lobby (for main-character editing) and during every
			// phase that shows the retinue or targets assets: prologue, main_event,
			// the shake_up endgame (take/break/claim-title pickers + crown display),
			// and the ended summary. Secrets only exist once the prologue has begun.
			if (data.game.phase === 'lobby' || data.game.phase === 'prologue' ||
				data.game.phase === 'main_event' || data.game.phase === 'shake_up' ||
				data.game.phase === 'ended') {
				const assetData = await listAssets(gameID);
				assets = assetData.assets;
			}
			if (data.game.phase === 'prologue' || data.game.phase === 'main_event' ||
				data.game.phase === 'shake_up' || data.game.phase === 'ended') {
				try {
					const sd = await getVisibleSecrets(gameID);
					secrets = sd.secrets;
				} catch { /* tolerate; secrets feature is non-critical */ }
			}

			// Chat is available in every phase, so load it unconditionally.
			try {
				const postsData = await listGamePosts(gameID);
				chatPosts = postsData.posts;
			} catch { /* tolerate empty chat on failure */ }

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
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load game state.';
		}
	}

	onMount(async () => {
		try {
			const me = await getMe();
			if (!me) {
				goto(`/?next=/table/${gameID}`);
				return;
			}
			// Open the WS first, with loadGameState as the resync callback.
			// createConnection will run loadGameState on every (re)connect —
			// including this initial one — and buffer any events that
			// arrive during the fetch so we never miss a transition. Await
			// `ready` so we can read `players` below to find our seat.
			const conn = createConnection(gameID, handleWSMessage, loadGameState);
			disconnect = conn.disconnect;
			await conn.ready;

			const seat = players.find((p) => p.account_id === me.id);
			if (!seat) {
				goto('/profile');
				return;
			}
			currentPlayerID = seat.id;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load table.';
		} finally {
			loading = false;
		}
	});

	function onEndgameRequired(e: Event) {
		const detail = (e as CustomEvent<{ modes: EndgameMode[] }>).detail;
		// Only the facilitator can resolve this; others see the toast via the
		// thrown error from the original preparePlan call.
		if (isFacilitator && detail?.modes?.length) {
			endgamePromptModes = detail.modes;
		}
	}
	onMount(() => window.addEventListener('uneasy:endgame_choice_required', onEndgameRequired));

	onDestroy(() => {
		disconnect?.();
		typingTimeouts.forEach(clearTimeout);
		window.removeEventListener('uneasy:endgame_choice_required', onEndgameRequired);
	});

	async function chooseEndgameMode(mode: EndgameMode) {
		if (endgameSubmitting) return;
		endgameSubmitting = true;
		try {
			await setEndgameMode(gameID, mode);
			endgamePromptModes = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not set endgame mode.';
		} finally {
			endgameSubmitting = false;
		}
	}

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

	// ── Phase advancement (lobby only) ────────────────────────────────────────
	let advancing = $state(false);

	async function advancePhase() {
		if (!game || advancing) return;
		advancing = true;
		error = '';
		try {
			if (game.phase === 'lobby') {
				await startPrologue(gameID);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally {
			advancing = false;
		}
	}

	const nextPhaseLabel: Record<string, string> = {
		lobby: 'Start Prologue',
	};

	// ── Tone-setting ──────────────────────────────────────────────────────────
	let newTopicText = $state('');
	let addingTopic = $state(false);

	const toneCycle: ToneTopic['status'][] = ['default', 'include', 'avoid_detail', 'never'];

	async function onTopicStatusChange(topicID: number, status: string) {
		try {
			await updateToneTopic(gameID, topicID, status as ToneTopic['status']);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update topic.';
		}
	}

	async function cycleTopicStatus(topic: ToneTopic) {
		const idx = toneCycle.indexOf(topic.status);
		const next = toneCycle[(idx + 1) % toneCycle.length];
		await onTopicStatusChange(topic.id, next);
	}

	async function submitNewTopic() {
		const text = newTopicText.trim();
		if (!text || addingTopic) return;
		addingTopic = true;
		try {
			await addToneTopic(gameID, text);
			newTopicText = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not add topic.';
		} finally {
			addingTopic = false;
		}
	}

	// ── Shared display helpers ────────────────────────────────────────────────
	const phaseLabels: Record<string, string> = {
		lobby: 'Lobby',
		tone_setting: 'Tone Setting',
		prologue: 'Prologue',
		main_event: 'Main Event',
		shake_up: 'Shake-Up',
		ended: 'Game Over',
	};

	// Player name lookup used in the ended-phase rankings display.
	function rankingLabel(playerID: number | null): string {
		if (playerID === null) return 'Dummy';
		return players.find(p => p.id === playerID)?.display_name ?? '?';
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
			<div class="members">
				{#each members as member}
					{@const mr = ranksByPlayer.get(member.id)}
					{@const atRisk = atRiskByPlayer.get(member.id) ?? 0}
					{@const isMe = member.id === currentPlayerID}
					<button type="button" class="member" class:active={isMe} onclick={() => retinueOpenForPlayer = member.id} aria-label={`View ${member.display_name}'s retinue${isMe ? ' (you)' : ''}${member.id === blockingPlayerID ? ' (their turn)' : ''}${atRisk > 0 ? ` — ${atRisk} ${atRisk === 1 ? 'asset needs' : 'assets need'} marginalia` : ''}`} style:--member-color={playerColorByID(member.id, players)}>
						{#if atRisk > 0}
							<span class="risk-badge" class:mine={isMe} title={`${atRisk} ${atRisk === 1 ? 'asset has' : 'assets have'} too few marginalia — fill an empty slot to avoid losing ${atRisk === 1 ? 'it' : 'them'}`} aria-hidden="true">{atRisk}</span>
						{/if}
						<span class="member-body">
							<span class="member-name-row">
								<span class="dot"></span>
								<span class="member-name">{member.display_name}</span>
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
			<button type="button" class="help-button" onclick={() => helpOpen = true} aria-label="How to play & feedback">
				<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
					<circle cx="12" cy="12" r="10" />
					<path d="M9.5 9a2.5 2.5 0 0 1 4.5 1.5c0 1.5-2 2-2 3" />
					<line x1="12" y1="17" x2="12" y2="17" />
				</svg>
			</button>
		</div>
		{#if game}
			<div class="game-info" class:has-war={$activeWarCount + $pendingWarCount > 0}>
				<span class="phase-badge">{#each (phaseLabels[game.phase] ?? game.phase).split(' ') as word, i}{#if i}{' '}{/if}<span>{word}</span>{/each}</span>
				<button class="tones-button" onclick={() => tonesOpen = true} aria-label="Open tones">
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
						class:war-mixed={$activeWarCount > 0 && $pendingWarCount > 0}
						onclick={() => warDrawerOpen.set(true)}
						aria-label="Open wars"
					>
						<span class="lbl">War</span>{#if $activeWarCount + $pendingWarCount > 1}<span class="count">{$activeWarCount + $pendingWarCount}</span>{/if}
					</button>
				{/if}
			</div>
		{/if}
	</header>

	{#if error}
		<p class="error-text error">{error}</p>
	{/if}

	<!--
		Body: on desktop ≥1024px this becomes a 2-column grid (game | chat),
		or 3-column when the Public Record rail is present.
		On mobile/tablet it's a single column with the chat panel positioned
		absolutely (strip pinned to bottom, expanded sheet covering the body).
		WaitingOnBar lives inside .phase-column so it only spans the phase
		content's column — not the PublicRecord rail or the Chat column.
	-->
	<div class="table-body" class:has-record={game?.phase === 'main_event'}>

	<!-- Public Record sidebar — only in main event. Page-level so it can sit
	     in its own grid column on wide desktop layouts (mirrors ChatPanel). -->
	{#if !loading && game?.phase === 'main_event'}
		<PublicRecord
			rows={recordRows}
			currentRow={game.current_row}
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

	{#if loading}
		<div class="center-message">Loading…</div>

	<!-- ── Lobby ──────────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'lobby'}
		<div class="phase-view lobby">
			<section class="lobby-join">
				<h2>Join Code</h2>
				<button class="code-badge" class:copied={joinCodeCopied} onclick={copyJoinCode} aria-label="Copy join code">
					{game.join_code}
					<span class="copy-hint" aria-live="polite">{joinCodeCopied ? 'Copied!' : 'copy'}</span>
				</button>
				<p class="muted-text">
					Share this code with your friends to invite them. The game needs 2–5 players.
				</p>
			</section>
			<div class="player-list">
				{#each players as p}
					<div class="player-row">
						{p.display_name}
						{#if p.is_facilitator}<span class="tag">facilitator</span>{/if}
					</div>
				{/each}
			</div>
			{#if isFacilitator && players.length >= 2}
				<button class="action-btn primary" onclick={advancePhase} disabled={advancing}>
					{advancing ? '…' : nextPhaseLabel['lobby']}
				</button>
			{:else if isFacilitator}
				<p class="muted-text">Need at least 2 players to start.</p>
			{/if}

			<section class="lobby-help">
				<h2>New to the game? Start here.</h2>
				<p class="muted-text">
					A two-minute primer while you wait for everyone to arrive. You can reopen this
					any time from the ? in the top-right corner.
				</p>
				<HelpContent />
			</section>
		</div>

	<!-- ── Prologue ───────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'prologue'}
		<PrologueView
			{gameID}
			{game}
			bind:players
			bind:rankings
			bind:assets
			{currentPlayerID}
			{isFacilitator}
			bind:waitingOn
			onResync={loadGameState}
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
			{isFacilitator}
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
			{currentPlayerID}
			bind:waitingOn
		/>

	<!-- ── Ended ──────────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'ended'}
		<div class="phase-view ended">
			<h2>Game Over</h2>
			<p class="muted-text">The public record is sealed. Thank you for playing.</p>
			{#if rankings.length > 0}
				<h3>Final Rankings</h3>
				<div class="rankings-preview">
					{#each ['power', 'knowledge', 'esteem'] as cat}
						<div class="rank-col">
							<h4>{cat}</h4>
							{#each rankings.filter(r => r.category === (cat as RankingCategory) && r.player_id !== null).sort((a, b) => a.rank - b.rank) as r}
								<div class="rank-slot-display">{r.rank}. {rankingLabel(r.player_id)}</div>
							{/each}
						</div>
					{/each}
				</div>
			{/if}
		</div>

	{:else}
		<div class="center-message">Unknown phase.</div>
	{/if}
	</div><!-- /.phase-column -->

		{#if !loading && currentPlayerID != null && game}
			<ChatPanel
				{gameID}
				posts={chatPosts}
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
			<h3>Tones</h3>
			<p class="muted-text small">
				Themes and topics your group wants to include or avoid. Tap a tile to cycle its status.
				{#if tonesLocked}<br /><em>Locked — the main event has begun.</em>{/if}
			</p>

			<div class="tone-legend" aria-label="Legend">
				<span class="tone-legend-item" data-status="default"><span class="swatch"></span>No Opinion</span>
				<span class="tone-legend-item" data-status="include"><span class="swatch"></span>Include</span>
				<span class="tone-legend-item" data-status="avoid_detail"><span class="swatch"></span>Avoid detail</span>
				<span class="tone-legend-item" data-status="never"><span class="swatch"></span>Never</span>
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
						maxlength={120}
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
			<h3>Laws</h3> <!--  ({laws.length}) -->
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
			<h3>Rumors</h3> <!--  ({rumors.length}) -->
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

	<RetinueSheet open={helpOpen} onClose={() => helpOpen = false}>
		<div class="help-sheet">
			<h3>How to play</h3>
			<HelpContent panel />
		</div>
	</RetinueSheet>

	<RetinueSheet open={retinueOpenForPlayer !== null} onClose={() => retinueOpenForPlayer = null}>
		{#if retinueOpenForPlayer !== null}
			<RetinueView
				playerId={retinueOpenForPlayer}
				{players}
				{members}
				{assets}
				{secrets}
				{rankings}
				viewerPlayerId={currentPlayerID}
				focusPlayerId={blockingPlayerID}
				onSecretsChanged={() => getVisibleSecrets(gameID).then(d => { secrets = d.secrets; }).catch(() => {})}
			/>
		{/if}
	</RetinueSheet>

	{#if endgamePromptModes}
		<div class="endgame-overlay">
			<div class="endgame-modal">
				<h3>Choose an endgame mode</h3>
				<p class="muted-text small">
					A plan would land past row 13. Pick how the game should wind down — this can't be undone.
				</p>
				{#if endgamePromptModes.includes('smooth_landing')}
					<button class="action-btn primary" disabled={endgameSubmitting} onclick={() => chooseEndgameMode('smooth_landing')}>
						Smooth Landing
					</button>
					<p class="muted-text small">
						Disallow plans past row 13. Let in-flight plans complete on their existing rows, then Shake-Up.
					</p>
				{/if}
				{#if endgamePromptModes.includes('explosive_finale')}
					<button class="action-btn primary" disabled={endgameSubmitting} onclick={() => chooseEndgameMode('explosive_finale')}>
						Explosive Finale
					</button>
					<p class="muted-text small">
						Collapse all remaining plans onto row 13. Resolve them in sequence with no scenes between, then Shake-Up.
					</p>
				{/if}
				<button class="action-btn secondary" disabled={endgameSubmitting} onclick={() => endgamePromptModes = null}>
					Cancel
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	/* ── Layout ─────────────────────────────────────────────────────────────── */

	.table-page {
		/* Single source of truth for the mobile chat strip's height. Read by
		   ChatPanel.svelte (.strip min-height) and by .table-body's reserved
		   padding-bottom below, so the two stay in sync. */
		--chat-strip-height: 46px;

		display: flex;
		flex-direction: column;
		height: 100dvh;
		max-width: 100%;
	}

	/*
	 * Body fills the space below the header. ChatPanel is a sibling of the
	 * phase content. On mobile it positions itself absolutely (strip pinned
	 * to bottom; expanded sheet covers the body), so the phase content reads
	 * the body's full size. On desktop ≥1024px the body becomes a 2-col
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

	/* In main_event on mobile, the public-record rail sits to the left of the
	   phase view rather than stacking above it (the rail is full-height, so
	   stacking pushes the phase content off-screen). The chat panel is
	   position:absolute on mobile so it stays unaffected. */
	.table-body.has-record {
		flex-direction: row;
		gap: 0.75rem;
		padding-right: 0.75rem;
	}
	.table-body.has-record > .phase-column {
		flex: 1;
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
	}

	@media (min-width: 1024px) {
		.table-body {
			display: grid;
			grid-template-columns: 1fr 360px;
			padding-bottom: 0;
		}
		/* When the public record is present (main_event), it takes the first
		   column. Below 1280 it's a thin rail with overlay-on-tap; at ≥1280
		   it becomes a permanent 320px panel. The phase view and chat shift
		   to columns 2 and 3 by source order. */
		.table-body.has-record {
			grid-template-columns: auto 1fr 360px;
		}
		/* The phase content children are direct children of .table-body; in
		   grid mode they automatically land in column 1 (or 2 with PR), and
		   ChatPanel in the last column. The min-width: 0 prevents long
		   content from blowing out the column. */
		.table-body > :global(*) { min-width: 0; min-height: 0; }
	}

	@media (min-width: 1280px) {
		/* All three columns equal width — each one mirrors a mobile viewport,
		   so layout/typography that works on phones works in any column. */
		.table-body.has-record {
			grid-template-columns: 360px 440px 1fr;
		}
		/* Phases without the record rail (prologue, lobby, ended) reserve the
		   same 800px (360 record + 440 phase) on the left and give chat the
		   remaining 1fr, so the chat column is the same width in every phase
		   instead of being pinned to 360px while the main event's grows. */
		.table-body:not(.has-record) {
			grid-template-columns: 800px 1fr;
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

	/* Two-line, square-ish badge so it stays as narrow as a button and fits the
	   status row on phones. The phase label's spaces are rendered as line breaks. */
	.phase-badge {
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.1em;
		min-height: 32px;
		/* Bottom padding is trimmed because capitals have no descenders — this
		   keeps the space above, between, and below the two words even. */
		padding: 0.3em 0.55em 0.2em;
		background: var(--color-border-warm);
		color: var(--color-accent);
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-family: var(--font-serif);
		font-size: 0.76rem;
		line-height: 1;
		white-space: nowrap;
	}
	/* Wars are rare; when one is active on a phone, drop the badge so the War
	   button takes its slot — the row never exceeds four items. */
	@media (max-width: 599px) {
		.game-info.has-war .phase-badge { display: none; }
	}

	.code-badge {
		font-family: monospace;
		font-size: 0.85rem;
		background: var(--color-border);
		color: var(--color-text);
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		letter-spacing: 0.1em;
		display: flex;
		gap: 0.4rem;
		align-items: center;
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
		border-left: 1px solid #3a3326; /* subtle warm hairline */
		color: #a89464;                 /* dimmed gilt */
		font-size: 0.75rem;
		font-variant-numeric: tabular-nums;
	}
	.war-button .count { border-left-color: #5a2020; color: #c89088; }

	.war-button {
		background: #3a1414;
		border-color: #6a2424;
		color: #f0c8c0;
	}
	.war-button:hover { background: #4a1a1a; }
	.war-button:focus-visible { outline-color: #d07060; }

	/* Yellow: only pending wars (planned, not yet started). */
	.war-button.war-pending {
		background: #3a3014;
		border-color: #6a5824;
		color: #f0e0a8;
	}
	.war-button.war-pending:hover { background: #4a3e1a; }
	.war-button.war-pending:focus-visible { outline-color: #d0b060; }

	/* Orange: mix of pending and active. */
	.war-button.war-mixed {
		background: #3a2214;
		border-color: #6a4024;
		color: #f0c898;
	}
	.war-button.war-mixed:hover { background: #4a2a1a; }
	.war-button.war-mixed:focus-visible { outline-color: #d08850; }

	.tones-sheet h3 { margin: 0 0 0.5rem; }
	.tones-sheet .small { font-size: 0.85rem; }
	.laws-rumors-sheet h3 { margin: 0 0 0.5rem; }
	.help-sheet h3 { margin: 0 0 0.75rem; }

	.copy-hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}
	.code-badge.copied .copy-hint { color: var(--color-accent); }

	.top-strip {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
		margin: 0 -1rem;
		padding: 0 1rem;
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

	/* Help "?" sits at the far right of the top strip, opposite the home icon.
	   .members is flex:1, so this naturally pins to the right edge. */
	.help-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		flex-shrink: 0;
		color: var(--color-accent);
		background: none;
		border: none;
		border-radius: 6px;
		cursor: pointer;
	}
	.help-button:hover { color: var(--color-accent-hover); background: var(--color-surface-2); }
	.help-button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.help-button svg { width: 24px; height: 24px; flex-shrink: 0; }

	.members {
		display: flex;
		gap: 0.4rem;
		flex: 1;
		min-width: 0;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		scrollbar-width: none;
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

	/* Warning badge: assets that are one tear from destruction but still have
	   empty marginalia slots to fill. Muted amber on other players' chips for
	   awareness; bright red on your own, where it's actionable. */
	.risk-badge {
		position: absolute;
		top: -4px;
		right: -4px;
		z-index: 1;
		min-width: 18px;
		height: 18px;
		padding: 0 4px;
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 999px;
		font-size: 0.7rem;
		font-weight: 600;
		line-height: 1;
		font-variant-numeric: tabular-nums;
		background: #3a3326;
		color: #a89464;
		border: 1px solid #4a4030;
	}
	.risk-badge.mine {
		background: #7a1f1f;
		color: #ffd9d4;
		border-color: #b03a3a;
		box-shadow: 0 0 6px rgba(176, 58, 58, 0.55);
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
		gap: 0.4rem;
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
	.member:hover { background: var(--color-border-subtle); }
	.member:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.member.active {
		border-color: var(--color-accent);
		box-shadow: 0 0 0 1px var(--color-accent), 0 0 8px rgba(200, 169, 110, 0.45);
		color: var(--color-text);
	}

	.member-name {
		white-space: nowrap;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--member-color, var(--color-text-muted));
		flex-shrink: 0;
	}

	.error {
		padding: 0.5rem 0;
		flex-shrink: 0;
	}

	.center-message {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-text-muted);
	}

	/* ── Phase views ────────────────────────────────────────────────────────── */

	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0.75rem;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	.phase-view h2 {
		color: var(--color-accent);
		font-size: 1.3rem;
		margin: 0;
	}

	.phase-view h3 {
		color: var(--color-accent);
		font-size: 1rem;
		margin: 0;
	}



	/* ── Lobby ──────────────────────────────────────────────────────────────── */

	.player-list { display: flex; flex-direction: column; gap: 0.4rem; }

	.player-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.95rem;
	}

	.tag {
		font-size: 0.7rem;
		background: var(--color-border-warm);
		color: var(--color-accent);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
	}

	.lobby-help {
		margin-top: 0.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--color-border);
	}
	.lobby h2 {
		color: var(--color-accent);
		font-size: 1.15rem;
		margin: 0 0 0.35rem;
	}
	.lobby-help .muted-text { margin-bottom: 0.9rem; }

	.lobby-join { margin-bottom: 0.5rem; }
	.lobby-join .muted-text {
		margin-top: 0.5rem;
		margin-bottom: 0.2rem;
	}
	.lobby-join .code-badge {
		display: inline-flex;
		font-size: 1rem;
		padding: 0.35rem 0.8rem;
	}

	/* ── Tone Setting ─────────────────────────────────────────────────────── */

	.tone-legend {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		font-size: 0.8rem;
		margin: 0.5rem 0 0.75rem;
		color: var(--color-text-secondary);
	}

	.tone-legend-item {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}

	.tone-legend-item .swatch {
		width: 0.9rem;
		height: 0.9rem;
		border-radius: 3px;
		border: 1px solid rgba(255,255,255,0.1);
	}

	.tone-legend-item[data-status='default']      .swatch { background: var(--color-neutral); }
	.tone-legend-item[data-status='include']      .swatch { background: #4f8a5c; }
	.tone-legend-item[data-status='avoid_detail'] .swatch { background: #b89446; }
	.tone-legend-item[data-status='never']        .swatch { background: #b35454; }

	.tone-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}

	@media (min-width: 600px) {
		.tone-grid { grid-template-columns: repeat(4, 1fr); }
	}
	@media (min-width: 900px) {
		.tone-grid { grid-template-columns: repeat(5, 1fr); }
	}

	.tone-tile {
		min-height: 44px;
		padding: 0.35rem 0.4rem;
		border-radius: 6px;
		border: 1px solid rgba(255,255,255,0.08);
		background: var(--color-neutral);
		color: #fff;
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

	.tone-tile[data-status='default']      { background: var(--color-neutral); }
	.tone-tile[data-status='include']      { background: #4f8a5c; }
	.tone-tile[data-status='avoid_detail'] { background: #b89446; }
	.tone-tile[data-status='never']        { background: #b35454; }

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
		background: #2a2a28;
		border: 1px dashed rgba(255,255,255,0.35);
		border-radius: 6px;
		color: var(--color-text);
		font-family: inherit;
		font-size: 0.9rem;
	}
	.tone-add-input::placeholder { color: rgba(232,228,217,0.5); }
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
		background: #6a8fb3;
		color: #fff;
		border: 1px solid rgba(255,255,255,0.12);
		border-radius: 6px;
		font-size: 0.9rem;
		cursor: pointer;
	}
	.tone-add-button:disabled { opacity: 0.5; cursor: not-allowed; }

	/* ── Ended ──────────────────────────────────────────────────────────────── */

	.rankings-preview {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.rank-col { display: flex; flex-direction: column; gap: 0.2rem; }

	.rank-col h4 {
		font-size: 0.8rem;
		color: var(--color-accent);
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot-display {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		padding: 0.15rem 0;
	}

	.endgame-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.6);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}
	.endgame-modal {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border-strong);
		border-radius: 8px;
		padding: 1.25rem;
		max-width: 28rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.endgame-modal h3 { color: var(--color-accent); margin: 0; font-size: 1.1rem; }
	.endgame-modal .secondary { align-self: flex-end; }
</style>
