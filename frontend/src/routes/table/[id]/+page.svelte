<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, onDestroy } from 'svelte';
	import {
		getGameState, getMe,
		startPrologue,
		updateToneTopic, addToneTopic,
		listAssets, getFullRecord, listGamePosts,
		getRoll, getActiveRollForGame,
		listPlans, getPlan,
		setEndgameMode,
		getVisibleSecrets,
		getActiveScene,
		type RankingCategory,
		type EndgameMode,
		type Scene,
		type ScenePeerView,
	} from '$lib/api';
	import { createConnection, EventTypes, type WSMessage } from '$lib/ws';
	import type {
		Game, Player, ToneTopic, Ranking, Asset, Marginalium,
		Law, Rumor,
		ChatPost, SceneEntry, RecordRow, PresenceMember,
		DiceRoll, DiceRollDie, DifficultyVote,
		Plan, Secret,
	} from '$lib/api';
	import MainEventView from '$lib/components/phases/MainEventView.svelte';
	import PublicRecord from '$lib/components/PublicRecord.svelte';
	import PrologueView from '$lib/components/phases/PrologueView.svelte';
	import ShakeUpView from '$lib/components/phases/ShakeUpView.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import LawsRumors from '$lib/components/LawsRumors.svelte';
	import RetinueView from '$lib/components/RetinueView.svelte';
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import { playerColorByID } from '$lib/playerColor';

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

	const typingLabel = $derived(
		typingNames.length === 0 ? '' :
		typingNames.length === 1 ? `${typingNames[0]} is writing…` :
		typingNames.length === 2 ? `${typingNames[0]} and ${typingNames[1]} are writing…` :
		'Several people are writing…'
	);

	// ── Public record + unified chat feed ─────────────────────────────────────
	let recordRows = $state<RecordRow[]>([]);
	let chatPosts = $state<ChatPost[]>([]);

	// ── Turn step state ───────────────────────────────────────────────────────
	// Tracks whether the focus player has ended the scene for the current row.
	// Resets when focus changes or the row advances.
	let sceneEnded = $state(false);

	// ── Scene state (SCENES_PLAN.md) ──────────────────────────────────────────
	// activeScene is the currently-running scene (location/time/peers), or
	// null between scenes. Loaded on mount and kept in sync via WS events.
	let activeScene = $state<Scene | null>(null);
	let activeScenePeers = $state<ScenePeerView[]>([]);

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
	let activeRollVotes = $state<DifficultyVote[]>([]);
	let voteOpen = $state(false);

	// ── Retinue sheet ─────────────────────────────────────────────────────────
	let retinueOpenForPlayer = $state<number | null>(null);
	let tonesOpen = $state(false);
	let lawsOpen = $state(false);
	let rumorsOpen = $state(false);
	let prologueActivePlayerID = $state<number | null>(null);

	const blockingPlayerID = $derived.by(() => {
		if (!game) return null;
		if (game.phase === 'prologue') return prologueActivePlayerID;
		if (game.phase === 'main_event') return game.focus_player_id;
		return null;
	});
	const tonesLocked = $derived(
		game != null && (game.phase === 'main_event' || game.phase === 'shake_up' || game.phase === 'ended')
	);

	// ── Plan state ────────────────────────────────────────────────────────────
	// Loaded on mount for main_event, then kept in sync by plan.* WS events.
	let plans = $state<Plan[]>([]);

	// Player name map passed to MainEventView for attribution.
	const playerNameMap = $derived(new Map(players.map(p => [p.id, p.display_name])));

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

	function handleWSMessage(msg: WSMessage) {
		switch (msg.type) {
			case EventTypes.PhaseChanged: {
				const newPhase = msg.payload.phase as Game['phase'];
				if (game) game = { ...game, phase: newPhase };
				loadGameState();
				break;
			}
			case EventTypes.PlayerJoined: {
				const player = msg.payload.player as Player;
				if (!players.find(p => p.id === player.id)) {
					players = [...players, player];
					members = [...members, { id: player.id, display_name: player.display_name, online: false }];
				}
				break;
			}
			case EventTypes.PresenceSnapshot: {
				const snap = msg.payload.members as PresenceMember[];
				members = members.map(m => ({
					...m,
					online: snap.some(s => s.id === m.id)
				}));
				break;
			}
			case EventTypes.TypingUpdate: {
				const { player_id, display_name, typing } = msg.payload as {
					player_id: number; display_name: string; typing: boolean;
				};
				const t = typingTimeouts.get(player_id);
				if (t) clearTimeout(t);
				if (typing) {
					typingMap.set(player_id, display_name);
					typingTimeouts.set(player_id, setTimeout(() => {
						typingMap.delete(player_id);
						typingNames = [...typingMap.values()];
					}, 4000));
				} else {
					typingMap.delete(player_id);
					typingTimeouts.delete(player_id);
				}
				typingNames = [...typingMap.values()];
				break;
			}
			case EventTypes.ToneUpdated: {
				const { topic_id, topic, status } = msg.payload as {
					topic_id: number; topic: string; status: string;
				};
				const idx = toneTopics.findIndex(t => t.id === topic_id);
				if (idx >= 0) {
					toneTopics = toneTopics.map(t =>
						t.id === topic_id ? { ...t, status: status as ToneTopic['status'] } : t
					);
				} else {
					toneTopics = [...toneTopics, {
						id: topic_id, game_id: Number(gameID), topic,
						status: status as ToneTopic['status']
					}];
				}
				break;
			}
			case EventTypes.RankingsUpdated: {
				rankings = msg.payload.rankings as Ranking[];
				break;
			}
			case EventTypes.FocusChanged: {
				if (game) game = { ...game, focus_player_id: msg.payload.player_id as number };
				sceneEnded = false; // reset turn step when focus changes
				break;
			}
			case EventTypes.PrologueTurnAdvanced: {
				prologueActivePlayerID = (msg.payload.current_player_id as number | null) ?? null;
				break;
			}
			case EventTypes.RowAdvanced: {
				const newRow = msg.payload.row_number as number;
				if (game) game = { ...game, current_row: newRow };
				sceneEnded = false; // reset turn step for new row
				// Chat is now a single continuous game-wide feed; no reset needed.
				break;
			}
			case EventTypes.SceneEnded: {
				sceneEnded = true;
				activeScene = null;
				activeScenePeers = [];
				break;
			}
			case EventTypes.SceneStarted: {
				const scene = msg.payload.scene as Scene;
				const peers = msg.payload.peers as ScenePeerView[];
				activeScene = scene;
				activeScenePeers = peers;
				sceneEnded = false;
				break;
			}
			case EventTypes.ScenePeerClaimed: {
				const { scene_id, peer_asset_id, controller_id } = msg.payload as {
					scene_id: number; peer_asset_id: number; controller_id: number;
				};
				if (activeScene && activeScene.id === scene_id) {
					activeScenePeers = activeScenePeers.map(p =>
						p.peer_asset_id === peer_asset_id
							? { ...p, controller_player_id: controller_id }
							: p
					);
				}
				break;
			}
			case EventTypes.ScenePostCreated: {
				const post = msg.payload.post as ChatPost;
				if (!chatPosts.find(p => p.id === post.id)) {
					chatPosts = [...chatPosts, post];
				}
				break;
			}
			case EventTypes.SceneEntryCreated: {
				const entry = msg.payload.entry as SceneEntry;
				recordRows = recordRows.map(row =>
					row.row_number === entry.row_number
						? { ...row, entries: row.entries.find(e => e.id === entry.id)
							? row.entries
							: [...row.entries, entry] }
						: row
				);
				break;
			}
			// Asset events
			case EventTypes.AssetCreated: {
				const asset = msg.payload.asset as Asset;
				if (!assets.find(a => a.id === asset.id)) {
					assets = [...assets, asset];
				}
				break;
			}
			case EventTypes.AssetUpdated: {
				const asset = msg.payload.asset as Asset;
				assets = assets.map(a => a.id === asset.id ? asset : a);
				break;
			}
			case EventTypes.AssetTaken: {
				const asset = msg.payload.asset as Asset;
				assets = assets.map(a => a.id === asset.id ? asset : a);
				break;
			}
			case EventTypes.AssetLeveraged: {
				const { asset_id } = msg.payload as { asset_id: number };
				assets = assets.map(a =>
					a.id === asset_id ? { ...a, is_leveraged: true } : a
				);
				break;
			}
			case EventTypes.AssetRefreshed: {
				const { asset_id } = msg.payload as { asset_id: number };
				assets = assets.map(a =>
					a.id === asset_id ? { ...a, is_leveraged: false } : a
				);
				break;
			}
			case EventTypes.AssetDestroyed: {
				const { asset_id } = msg.payload as { asset_id: number };
				assets = assets.filter(a => a.id !== asset_id);
				break;
			}
			case EventTypes.MarginaliaAdded: {
				const { asset_id, marginalia } = msg.payload as { asset_id: number; marginalia: Marginalium };
				assets = assets.map(a => {
					if (a.id !== asset_id) return a;
					const already = a.marginalia.find(m => m.id === marginalia.id);
					return already ? a : { ...a, marginalia: [...a.marginalia, marginalia] };
				});
				break;
			}
			case EventTypes.MarginaliaUpdated: {
				const { asset_id, marginalia } = msg.payload as { asset_id: number; marginalia: Marginalium };
				assets = assets.map(a => {
					if (a.id !== asset_id) return a;
					return { ...a, marginalia: a.marginalia.map(m => m.id === marginalia.id ? marginalia : m) };
				});
				break;
			}
			case EventTypes.MarginaliaTorn: {
				const { asset_id, position } = msg.payload as { asset_id: number; position: number };
				assets = assets.map(a => {
					if (a.id !== asset_id) return a;
					return {
						...a,
						marginalia: a.marginalia.map(m =>
							m.position === position ? { ...m, is_torn: true } : m
						)
					};
				});
				break;
			}
			// Dice roll events
			case EventTypes.RollCreated: {
				const roll = msg.payload.roll as DiceRoll;
				// Fetch full details (includes dice) so we're in sync.
				getRoll(roll.id).then(data => {
					activeRoll = data.roll;
					activeRollDice = data.dice;
					activeRollVotes = data.votes;
					voteOpen = false;
				}).catch(() => {
					activeRoll = roll;
					activeRollDice = [];
					activeRollVotes = [];
					voteOpen = false;
				});
				break;
			}
			case EventTypes.RollLeverageAdded: {
				const { roll_id } = msg.payload as { roll_id: number };
				// Re-fetch dice so we have the actual DB row with its ID.
				if (activeRoll && activeRoll.id === roll_id) {
					getRoll(roll_id).then(data => {
						activeRollDice = data.dice;
					}).catch(() => {});
				}
				break;
			}
			case EventTypes.RollVoteCalled: {
				voteOpen = true;
				break;
			}
			case EventTypes.RollVoteCast: {
				const { roll_id, player_id, vote } = msg.payload as {
					roll_id: number; player_id: number; vote: 'yea' | 'nay';
				};
				if (activeRoll && activeRoll.id === roll_id) {
					activeRollVotes = [
						...activeRollVotes.filter(v => v.player_id !== player_id),
						{ roll_id, player_id, vote, voted_at: new Date().toISOString() }
					];
				}
				break;
			}
			case EventTypes.RollVoteResolved: {
				const { roll_id, adjusted_difficulty } = msg.payload as {
					roll_id: number; adjusted_difficulty: number;
				};
				if (activeRoll && activeRoll.id === roll_id) {
					activeRoll = { ...activeRoll, adjusted_difficulty };
				}
				break;
			}
			case EventTypes.RollResolved: {
				const resolvedRoll = msg.payload.roll as DiceRoll;
				const resolvedDice = msg.payload.dice as DiceRollDie[];
				if (activeRoll && activeRoll.id === resolvedRoll.id) {
					activeRoll = resolvedRoll;
					activeRollDice = resolvedDice;
				}
				break;
			}
			// Plan events
			case EventTypes.PlanPrepared: {
				const prepared = msg.payload.plan as Plan;
				if (!plans.find(p => p.id === prepared.id)) {
					plans = [...plans, prepared];
				}
				break;
			}
			case EventTypes.PlanResolving: {
				const resolving = msg.payload.plan as Plan;
				plans = plans.map(p => p.id === resolving.id ? resolving : p);
				break;
			}
			case EventTypes.PlanResolved: {
				const { plan_id, result } = msg.payload as { plan_id: number; result: string };
				plans = plans.map(p =>
					p.id === plan_id
						? { ...p, status: 'completed' as Plan['status'], result: result as Plan['result'] }
						: p
				);
				break;
			}
			case EventTypes.PlanDelayedArrival:
			case EventTypes.LiaisePhaseChanged:
			case EventTypes.LiaiseChoicesRevealed:
			case EventTypes.DuelChampionElected:
			case EventTypes.DuelStakesRevealed:
			case EventTypes.DuelBoutResolved:
			case EventTypes.DuelBoutsComplete:
			case EventTypes.FestivityGuestJoined:
			case EventTypes.FestivityGuestRolled:
			case EventTypes.FestivityGuestChose:
			case EventTypes.FestivityHostChose:
			case EventTypes.FestivityInsistHostMar:
			case EventTypes.FestivityPhaseChanged:
			case EventTypes.FestivityChallengeIssued:
			case EventTypes.FestivityChallengeDeclined:
			case EventTypes.FestivityDuelTriggered:
			case EventTypes.WarDeclared:
			case EventTypes.DemandPrepared:
			case EventTypes.DemandResolved:
			case EventTypes.DemandDraftPick:
			case EventTypes.DemandCounterPending:
			case EventTypes.DemandCounterPlaced:
			case EventTypes.DemandRetargeted: {
				const { plan_id } = msg.payload as { plan_id: number };
				refreshPlan(plan_id);
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.DemandLeverageSet: {
				// Adds dice to the target plan's open roll without going through
				// RollLeverageAdded; refresh dice if it's the active roll.
				const { plan_id, roll_id } = msg.payload as { plan_id: number; roll_id: number };
				if (activeRoll && activeRoll.id === roll_id) {
					getRoll(roll_id).then(data => { activeRollDice = data.dice; }).catch(() => {});
				}
				refreshPlan(plan_id);
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.WarPlayerJoined:
			case EventTypes.WarBattleCostDue:
			case EventTypes.WarBattleCostPaid:
			case EventTypes.WarPlayerSurrendered:
			case EventTypes.WarAssetSeized:
			case EventTypes.WarEntryCompleted:
			case EventTypes.WarPeaceProposed:
			case EventTypes.WarPeaceVote:
			case EventTypes.WarEnded: {
				// War events expose war_id only; refresh all plans so any
				// war-bearing plans pick up updated participants / costs / peace state.
				refreshAllPlans();
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.LawEnacted: {
				const { law } = msg.payload as { law: Law };
				if (law) {
					const idx = laws.findIndex(l => l.id === law.id);
					laws = idx >= 0 ? laws.map(l => l.id === law.id ? law : l) : [...laws, law];
				}
				break;
			}
			case EventTypes.LawUpdated: {
				const { law } = msg.payload as { law: Law };
				if (law) laws = laws.map(l => l.id === law.id ? law : l);
				break;
			}
			case EventTypes.RumorCreated: {
				const { rumor } = msg.payload as { rumor: Rumor };
				if (rumor) {
					const idx = rumors.findIndex(r => r.id === rumor.id);
					rumors = idx >= 0 ? rumors.map(r => r.id === rumor.id ? rumor : r) : [...rumors, rumor];
				}
				break;
			}
			case EventTypes.RumorUpdated: {
				const { rumor } = msg.payload as { rumor: Rumor };
				if (rumor) rumors = rumors.map(r => r.id === rumor.id ? rumor : r);
				break;
			}
			case EventTypes.SecretCreated:
			case EventTypes.SecretVisibilityGrant: {
				// SecretCreated payload omits text. SecretVisibilityGrant grows
				// the viewer's set when relevant. Either way, refetch.
				getVisibleSecrets(gameID).then(d => { secrets = d.secrets; }).catch(() => {});
				break;
			}
			case EventTypes.RevealSubmitted:
			case EventTypes.RevealComplete: {
				// Reveal widgets subscribe to these directly; no plan ID in payload.
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.EndgameModeSet: {
				const mode = (msg.payload as { mode: string }).mode;
				if (game) game = { ...game, ending_mode: mode };
				break;
			}
			case EventTypes.ShakeUpStepChanged:
			case EventTypes.ShakeUpRolled:
			case EventTypes.ShakeUpSpendOpened:
			case EventTypes.ShakeUpAdjusted:
			case EventTypes.ShakeUpSpendCommitted:
			case EventTypes.ShakeUpEnded: {
				if (msg.type === EventTypes.ShakeUpStepChanged && game) {
					const p = msg.payload as { category: string; step: number };
					game = { ...game, shake_up_category: p.category, shake_up_step: p.step };
				}
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.PrologueChoiceClaimed:
			case EventTypes.PrologueRankingStepChanged:
			case EventTypes.PrologueHeartsDeclared:
			case EventTypes.PrologueTrackRanked:
			case EventTypes.PrologueSetAsidesPlaced:
			case EventTypes.PrologueCommittedHeartsChanged:
			case EventTypes.PrologueDoneChanged:
			case EventTypes.PrologueExtraPeerCreated: {
				// Step changes update the game's ranking_step locally so the
				// view re-renders the right sub-flow without a full reload.
				if (msg.type === EventTypes.PrologueRankingStepChanged && game) {
					const step = (msg.payload as { step: string }).step;
					game = { ...game, prologue_ranking_step: step ? (step as Game['prologue_ranking_step']) : null };
				}
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
		}
	}

	function refreshPlan(planID: number) {
		getPlan(planID).then(detail => {
			const next = detail.plan;
			const idx = plans.findIndex(p => p.id === planID);
			plans = idx >= 0
				? plans.map(p => p.id === planID ? next : p)
				: [...plans, next];
		}).catch(() => {});
	}

	function refreshAllPlans() {
		listPlans(gameID).then(data => { plans = data.plans; }).catch(() => {});
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

			// Load assets in lobby (for main-character editing) and during
			// prologue/main_event (full retinue). Secrets only exist once the
			// prologue has begun.
			if (data.game.phase === 'lobby' || data.game.phase === 'prologue' || data.game.phase === 'main_event') {
				const assetData = await listAssets(gameID);
				assets = assetData.assets;
			}
			if (data.game.phase === 'prologue' || data.game.phase === 'main_event') {
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
				const [recordData, rollData, plansData, sceneData] = await Promise.all([
					getFullRecord(gameID),
					getActiveRollForGame(gameID),
					listPlans(gameID),
					getActiveScene(gameID).catch(() => ({ scene: null, peers: [] as ScenePeerView[] })),
				]);
				recordRows = recordData.rows;
				plans = plansData.plans;
				activeScene = sceneData.scene;
				activeScenePeers = sceneData.peers;
				if (rollData.roll) {
					activeRoll = rollData.roll;
					activeRollDice = rollData.dice;
					activeRollVotes = rollData.votes;
					// We don't know vote-open state from DB alone; check if votes exist.
					voteOpen = rollData.votes.length > 0;
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
				goto(`/login?next=/table/${gameID}`);
				return;
			}
			await loadGameState();
			const seat = players.find((p) => p.account_id === me.id);
			if (!seat) {
				goto('/profile');
				return;
			}
			currentPlayerID = seat.id;
			disconnect = createConnection(gameID, handleWSMessage);
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
	/** Re-fetches the plan list. Passed to MainEventView as onPlansChanged. */
	async function refreshPlans() {
		try {
			const data = await listPlans(gameID);
			plans = data.plans;
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
	<header>
		<div class="top-strip">
			<a class="home" href="/profile" aria-label="Home">
				<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
					<path d="M3 11l9-8 9 8" />
					<path d="M5 10v10h14V10" />
				</svg>
			</a>
			<div class="members">
				{#each members as member}
					<button type="button" class="member" class:online={member.online} class:active={member.id === blockingPlayerID} onclick={() => retinueOpenForPlayer = member.id} aria-label={`View ${member.display_name}'s retinue${member.id === blockingPlayerID ? ' (their turn)' : ''}`} style:--member-color={playerColorByID(member.id, players)}>
						<span class="dot"></span>
						<span class="member-name">{member.display_name}</span>
					</button>
				{/each}
			</div>
		</div>
		{#if game}
			<div class="game-info">
				<span class="phase-badge">{phaseLabels[game.phase] ?? game.phase}</span>
				<button class="tones-button" onclick={() => tonesOpen = true} aria-label="Open tones">
					Tones
				</button>
				{#if game.phase === 'main_event'}
					<button class="tones-button" onclick={() => lawsOpen = true} aria-label="Open laws">
						Laws{laws.length > 0 ? ` (${laws.length})` : ''}
					</button>
					<button class="tones-button" onclick={() => rumorsOpen = true} aria-label="Open rumors">
						Rumors{rumors.length > 0 ? ` (${rumors.length})` : ''}
					</button>
				{/if}
				{#if game.phase === 'lobby'}
					<button class="code-badge" onclick={() => navigator.clipboard.writeText(game!.join_code)}>
						{game.join_code}
						<span class="copy-hint">copy</span>
					</button>
				{/if}
			</div>
		{/if}
	</header>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	<!--
		Body: on desktop ≥1024px this becomes a 2-column grid (game | chat).
		On mobile/tablet it's a single column with the chat panel positioned
		absolutely (strip pinned to bottom, expanded sheet covering the body).
	-->
	<div class="table-body" class:has-record={game?.phase === 'main_event'}>

	<!-- Public Record sidebar — only in main event. Page-level so it can sit
	     in its own grid column on wide desktop layouts (mirrors ChatPanel). -->
	{#if !loading && game?.phase === 'main_event'}
		<PublicRecord
			rows={recordRows}
			currentRow={game.current_row}
			playerNames={playerNameMap}
			onRowJump={jumpToRow}
			onPlanJump={jumpToPlan}
			onSceneJump={jumpToScene}
		/>
	{/if}

	{#if loading}
		<div class="center-message">Loading…</div>

	<!-- ── Lobby ──────────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'lobby'}
		<div class="phase-view lobby">
			<h2>Waiting for players</h2>
			<p class="muted">
				Share the join code <strong>{game.join_code}</strong> with your friends. The game needs 2–5 players.
			</p>
			<div class="player-list">
				{#each players as p}
					<div class="player-row">
						{p.display_name}
						{#if p.is_facilitator}<span class="tag">facilitator</span>{/if}
					</div>
				{/each}
			</div>
			{#if isFacilitator && players.length >= 2}
				<button class="primary" onclick={advancePhase} disabled={advancing}>
					{advancing ? '…' : nextPhaseLabel['lobby']}
				</button>
			{:else if isFacilitator}
				<p class="muted">Need at least 2 players to start.</p>
			{/if}
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
			bind:sceneEnded
			{playerNameMap}
			{isFacilitator}
			bind:activeRoll
			bind:activeRollDice
			bind:activeRollVotes
			bind:voteOpen
			{plans}
			onPlansChanged={refreshPlans}
			{activeScene}
			{activeScenePeers}
			onSceneRefresh={refreshActiveScene}
		/>

	<!-- ── Shake-Up ───────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'shake_up'}
		<ShakeUpView
			{gameID}
			{game}
			{players}
			{assets}
			{currentPlayerID}
		/>

	<!-- ── Ended ──────────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'ended'}
		<div class="phase-view ended">
			<h2>Game Over</h2>
			<p class="muted">The public record is sealed. Thank you for playing.</p>
			{#if rankings.length > 0}
				<h3>Final Rankings</h3>
				<div class="rankings-preview">
					{#each ['power', 'knowledge', 'esteem'] as cat}
						<div class="rank-col">
							<h4>{cat}</h4>
							{#each rankings.filter(r => r.category === (cat as RankingCategory)).sort((a, b) => a.rank - b.rank) as r}
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
			/>
		{/if}
	</div>

	<RetinueSheet open={tonesOpen} onClose={() => tonesOpen = false}>
		<div class="tones-sheet">
			<h3>Tones</h3>
			<p class="muted small">
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
			<h3>Laws</h3>
			<LawsRumors
				kind="laws"
				{laws}
				{rumors}
				{plans}
				playerNames={playerNameMap}
				{currentPlayerID}
			/>
		</div>
	</RetinueSheet>

	<RetinueSheet open={rumorsOpen} onClose={() => rumorsOpen = false}>
		<div class="laws-rumors-sheet">
			<h3>Rumors</h3>
			<LawsRumors
				kind="rumors"
				{laws}
				{rumors}
				{plans}
				playerNames={playerNameMap}
				{currentPlayerID}
			/>
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
				leverageActive={activeRoll != null && activeRoll.outcome == null}
				onSecretsChanged={() => getVisibleSecrets(gameID).then(d => { secrets = d.secrets; }).catch(() => {})}
			/>
		{/if}
	</RetinueSheet>

	{#if endgamePromptModes}
		<div class="endgame-overlay">
			<div class="endgame-modal">
				<h3>Choose an endgame mode</h3>
				<p class="muted small">
					A plan would land past row 13. Pick how the game should wind down — this can't be undone.
				</p>
				{#if endgamePromptModes.includes('smooth_landing')}
					<button class="primary" disabled={endgameSubmitting} onclick={() => chooseEndgameMode('smooth_landing')}>
						Smooth Landing
					</button>
					<p class="muted small">
						Disallow plans past row 13. Let in-flight plans complete on their existing rows, then Shake-Up.
					</p>
				{/if}
				{#if endgamePromptModes.includes('explosive_finale')}
					<button class="primary" disabled={endgameSubmitting} onclick={() => chooseEndgameMode('explosive_finale')}>
						Explosive Finale
					</button>
					<p class="muted small">
						Collapse all remaining plans onto row 13. Resolve them in sequence with no scenes between, then Shake-Up.
					</p>
				{/if}
				<button class="secondary" disabled={endgameSubmitting} onclick={() => endgamePromptModes = null}>
					Cancel
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	/* ── Layout ─────────────────────────────────────────────────────────────── */

	.table-page {
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
		   including the iOS home-indicator safe area. Must stay in sync with
		   the strip's min-height + padding in ChatPanel.svelte. */
		padding-bottom: calc(56px + env(safe-area-inset-bottom));
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
	}

	header {
		padding: 0.75rem 0;
		border-bottom: 1px solid #333;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		flex-shrink: 0;
	}

	.game-info {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.phase-badge {
		font-size: 0.8rem;
		background: #3a3020;
		color: #c8a96e;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.code-badge {
		font-family: monospace;
		font-size: 0.85rem;
		background: #333;
		color: #e8e4d9;
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		letter-spacing: 0.1em;
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}

	.tones-button {
		font-size: 0.8rem;
		background: #2a2a2a;
		color: #e8e4d9;
		padding: 0.3rem 0.7rem;
		border-radius: 4px;
		border: 1px solid #3a3a3a;
		min-height: 32px;
	}
	.tones-button:hover { background: #333; }
	.tones-button:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

	.tones-sheet h3 { margin: 0 0 0.5rem; }
	.tones-sheet .small { font-size: 0.85rem; }
	.laws-rumors-sheet h3 { margin: 0 0 0.5rem; }

	.copy-hint {
		font-size: 0.7rem;
		color: #888;
	}

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
		color: #c8a96e;
		border-radius: 6px;
		text-decoration: none;
	}
	.home:hover { color: #d9bb80; background: #2a2a2a; }
	.home:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

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
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		flex-shrink: 0;
		min-height: 44px;
		padding: 0.4rem 0.7rem;
		font-size: 0.85rem;
		color: #888;
		background: #262626;
		border: 1px solid #333;
		border-radius: 999px;
		cursor: pointer;
	}
	.member:hover { background: #2e2e2e; }
	.member:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }
	.member.online { color: #e8e4d9; }
	.member.active {
		border-color: #c8a96e;
		box-shadow: 0 0 0 1px #c8a96e, 0 0 8px rgba(200, 169, 110, 0.45);
		color: #e8e4d9;
	}

	.member-name {
		white-space: nowrap;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
		flex-shrink: 0;
	}

	.member.online .dot { background: var(--member-color, #6dbf7a); }

	.error {
		color: #e07070;
		font-size: 0.85rem;
		padding: 0.5rem 0;
		flex-shrink: 0;
	}

	.center-message {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: #888;
	}

	/* ── Phase views ────────────────────────────────────────────────────────── */

	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	.phase-view h2 {
		color: #c8a96e;
		font-size: 1.3rem;
		margin: 0;
	}

	.phase-view h3 {
		color: #c8a96e;
		font-size: 1rem;
		margin: 0;
	}

	.muted {
		color: #999;
		font-size: 0.9rem;
		line-height: 1.5;
		margin: 0;
	}

	.primary {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.6rem 1.2rem;
		border-radius: 6px;
		align-self: flex-start;
	}

	.primary:disabled {
		opacity: 0.4;
		cursor: not-allowed;
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
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
	}

	/* ── Tone Setting ─────────────────────────────────────────────────────── */

	.tone-legend {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		font-size: 0.8rem;
		margin: 0.5rem 0 0.75rem;
		color: #cfcabd;
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

	.tone-legend-item[data-status='default']      .swatch { background: #555; }
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
		aspect-ratio: 3 / 2;
		padding: 0.5rem;
		border-radius: 6px;
		border: 1px solid rgba(255,255,255,0.08);
		background: #555;
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

	.tone-tile[data-status='default']      { background: #555; }
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
		color: #e8e4d9;
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
		font-weight: 600;
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
		color: #c8a96e;
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot-display {
		font-size: 0.85rem;
		color: #ccc;
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
		background: #1e1e1e;
		border: 1px solid #444;
		border-radius: 8px;
		padding: 1.25rem;
		max-width: 28rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.endgame-modal h3 { color: #c8a96e; margin: 0; font-size: 1.1rem; }
	.endgame-modal .primary {
		background: #c8a96e; color: #1a1a1a; font-weight: 600;
		padding: 0.5rem 1rem; border-radius: 6px;
	}
	.endgame-modal .secondary {
		background: #333; color: #e8e4d9; font-weight: 600;
		padding: 0.4rem 0.8rem; border-radius: 6px; border: 1px solid #555;
		align-self: flex-end;
	}
	.endgame-modal .muted { color: #999; font-size: 0.9rem; margin: 0; }
	.endgame-modal .muted.small { font-size: 0.8rem; }
</style>
