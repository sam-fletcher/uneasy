<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import {
		getGameState, getIdentity,
		startToneSetting, startPrologue,
		updateToneTopic, addToneTopic,
		listAssets, getFullRecord, listScenePosts,
		getRoll, getActiveRollForGame,
		listPlans, getPlan,
		type RankingCategory,
	} from '$lib/api';
	import { createConnection, EventTypes, type WSMessage } from '$lib/ws';
	import type {
		Game, Player, ToneTopic, Ranking, Asset, Marginalium,
		ScenePost, SceneEntry, RecordRow, PresenceMember,
		DiceRoll, DiceRollDie, DifficultyVote,
		Plan,
	} from '$lib/api';
	import MainEventView from '$lib/components/phases/MainEventView.svelte';
	import PrologueView from '$lib/components/phases/PrologueView.svelte';

	const gameID = $derived(page.params.id as string);

	// ── Core state ────────────────────────────────────────────────────────────
	let game = $state<Game | null>(null);
	let players = $state<Player[]>([]);
	let toneTopics = $state<ToneTopic[]>([]);
	let rankings = $state<Ranking[]>([]);
	let assets = $state<Asset[]>([]);
	let members = $state<PresenceMember[]>([]);
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

	// ── Public record + scene posts (main_event) ──────────────────────────────
	let recordRows = $state<RecordRow[]>([]);
	let scenePosts = $state<ScenePost[]>([]);

	// ── Turn step state ───────────────────────────────────────────────────────
	// Tracks whether the focus player has ended the scene for the current row.
	// Resets when focus changes or the row advances.
	let sceneEnded = $state(false);

	// ── Dice roll state ───────────────────────────────────────────────────────
	// activeRoll is the current unresolved dice roll for this game (or null).
	// It's set by roll.created WS events and on page load (via getRoll).
	let activeRoll = $state<DiceRoll | null>(null);
	let activeRollDice = $state<DiceRollDie[]>([]);
	let activeRollVotes = $state<DifficultyVote[]>([]);
	let voteOpen = $state(false);

	// ── Plan state ────────────────────────────────────────────────────────────
	// Loaded on mount for main_event, then kept in sync by plan.* WS events.
	let plans = $state<Plan[]>([]);

	// Player name map passed to MainEventView for attribution.
	const playerNameMap = $derived(new Map(players.map(p => [p.id, p.display_name])));

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
			case EventTypes.RowAdvanced: {
				const newRow = msg.payload.row_number as number;
				if (game) game = { ...game, current_row: newRow };
				sceneEnded = false; // reset turn step for new row
				// Reload posts for the new row's scene thread.
				listScenePosts(gameID, newRow).then(data => { scenePosts = data.posts; }).catch(() => {});
				break;
			}
			case EventTypes.SceneEnded: {
				sceneEnded = true;
				break;
			}
			case EventTypes.ScenePostCreated: {
				const post = msg.payload.post as ScenePost;
				if (!scenePosts.find(p => p.id === post.id)) {
					scenePosts = [...scenePosts, post];
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
			case EventTypes.FestivityDuelTriggered:
			case EventTypes.WarDeclared:
			case EventTypes.DemandDraftPick:
			case EventTypes.DemandCounterPlaced: {
				const { plan_id } = msg.payload as { plan_id: number };
				refreshPlan(plan_id);
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.WarPlayerJoined:
			case EventTypes.WarBattleCostDue:
			case EventTypes.WarBattleCostPaid:
			case EventTypes.WarPeaceProposed:
			case EventTypes.WarEnded: {
				// War events expose war_id only; refresh all plans so any
				// war-bearing plans pick up updated participants / costs / peace state.
				refreshAllPlans();
				window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case EventTypes.RevealSubmitted:
			case EventTypes.RevealComplete: {
				// Reveal widgets subscribe to these directly; no plan ID in payload.
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
			members = data.players.map(p => ({
				id: p.id,
				display_name: p.display_name,
				online: false
			}));

			// Load assets during prologue and main_event.
			if (data.game.phase === 'prologue' || data.game.phase === 'main_event') {
				const assetData = await listAssets(gameID);
				assets = assetData.assets;
			}

			// Load public record, scene posts, plans, and active roll if in main_event.
			if (data.game.phase === 'main_event' && data.game.current_row > 0) {
				const [postsData, recordData, rollData, plansData] = await Promise.all([
					listScenePosts(gameID, data.game.current_row),
					getFullRecord(gameID),
					getActiveRollForGame(gameID),
					listPlans(gameID),
				]);
				scenePosts = postsData.posts;
				recordRows = recordData.rows;
				plans = plansData.plans;
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
			const identity = await getIdentity();
			if (identity.player) currentPlayerID = identity.player.id;
			await loadGameState();
			disconnect = createConnection(gameID, handleWSMessage);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load table.';
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		disconnect?.();
		typingTimeouts.forEach(clearTimeout);
	});

	// ── Plan helpers ─────────────────────────────────────────────────────────
	/** Re-fetches the plan list. Passed to MainEventView as onPlansChanged. */
	async function refreshPlans() {
		try {
			const data = await listPlans(gameID);
			plans = data.plans;
		} catch { /* ignore — WS events will keep us in sync */ }
	}

	// ── Phase advancement (lobby + tone_setting only) ─────────────────────────
	let advancing = $state(false);

	async function advancePhase() {
		if (!game || advancing) return;
		advancing = true;
		error = '';
		try {
			switch (game.phase) {
				case 'lobby':        await startToneSetting(gameID); break;
				case 'tone_setting': await startPrologue(gameID); break;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally {
			advancing = false;
		}
	}

	const nextPhaseLabel: Record<string, string> = {
		lobby: 'Start Tone Setting',
		tone_setting: 'Start Prologue',
	};

	// ── Tone-setting ──────────────────────────────────────────────────────────
	let newTopicText = $state('');
	let addingTopic = $state(false);

	async function onTopicStatusChange(topicID: number, status: string) {
		try {
			await updateToneTopic(gameID, topicID, status as ToneTopic['status']);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update topic.';
		}
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
		<div class="game-info">
			<span class="game-title">Uneasy Lies the Head</span>
			{#if game}
				<span class="phase-badge">{phaseLabels[game.phase] ?? game.phase}</span>
				<button class="code-badge" onclick={() => navigator.clipboard.writeText(game!.join_code)}>
					{game.join_code}
					<span class="copy-hint">copy</span>
				</button>
			{/if}
		</div>
		<div class="members">
			{#each members as member}
				<span class="member" class:online={member.online}>
					<span class="dot"></span>
					{member.display_name}
				</span>
			{/each}
		</div>
	</header>

	{#if error}
		<p class="error">{error}</p>
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

	<!-- ── Tone Setting ──────────────────────────────────────────────────── -->
	{:else if game?.phase === 'tone_setting'}
		<div class="phase-view tone-setting">
			<h2>Tone Setting</h2>
			<p class="muted">
				Discuss what themes and topics your group wants to include or avoid.
				Anyone can change a topic's status.
			</p>

			<div class="tone-list">
				{#each toneTopics as topic (topic.id)}
					<div class="tone-row" data-status={topic.status}>
						<span class="tone-topic">{topic.topic}</span>
						<select
							class="tone-select"
							value={topic.status}
							onchange={(e) => onTopicStatusChange(topic.id, (e.target as HTMLSelectElement).value)}
						>
							<option value="default">Default</option>
							<option value="include">Include</option>
							<option value="avoid_detail">Avoid detail</option>
							<option value="never">Never</option>
						</select>
					</div>
				{/each}
			</div>

			<div class="add-topic">
				<input
					type="text"
					placeholder="Add a custom topic…"
					bind:value={newTopicText}
					onkeydown={(e) => { if (e.key === 'Enter') submitNewTopic(); }}
					maxlength={120}
				/>
				<button onclick={submitNewTopic} disabled={!newTopicText.trim() || addingTopic}>
					{addingTopic ? '…' : 'Add'}
				</button>
			</div>

			{#if isFacilitator}
				<button class="primary" onclick={advancePhase} disabled={advancing}>
					{advancing ? '…' : nextPhaseLabel['tone_setting']}
				</button>
			{/if}
		</div>

	<!-- ── Prologue ───────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'prologue'}
		<PrologueView
			{gameID}
			bind:players
			bind:rankings
			{assets}
			{currentPlayerID}
			{isFacilitator}
		/>

	<!-- ── Main Event ─────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'main_event'}
		<MainEventView
			{game}
			{players}
			{assets}
			{currentPlayerID}
			bind:recordRows
			bind:scenePosts
			bind:sceneEnded
			{typingLabel}
			{playerNameMap}
			{isFacilitator}
			bind:activeRoll
			bind:activeRollDice
			bind:activeRollVotes
			bind:voteOpen
			{plans}
			onPlansChanged={refreshPlans}
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
</div>

<style>
	/* ── Layout ─────────────────────────────────────────────────────────────── */

	.table-page {
		display: flex;
		flex-direction: column;
		height: 100dvh;
		max-width: 100%;
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

	.game-title {
		font-weight: 700;
		font-size: 1.1rem;
		color: #c8a96e;
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

	.copy-hint {
		font-size: 0.7rem;
		color: #888;
	}

	.members {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.member {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.85rem;
		color: #888;
	}

	.member.online { color: #e8e4d9; }

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
	}

	.member.online .dot { background: #6dbf7a; }

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

	.tone-list { display: flex; flex-direction: column; gap: 0.3rem; }

	.tone-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.4rem 0.6rem;
		border-radius: 4px;
		background: #2a2a2a;
		gap: 0.5rem;
	}

	.tone-row[data-status='include']      { border-left: 3px solid #6dbf7a; }
	.tone-row[data-status='avoid_detail'] { border-left: 3px solid #e0c070; }
	.tone-row[data-status='never']        { border-left: 3px solid #e07070; }
	.tone-row[data-status='default']      { border-left: 3px solid #555; }

	.tone-topic { font-size: 0.9rem; flex: 1; }

	.tone-select {
		font-size: 0.8rem;
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.2rem 0.4rem;
	}

	.add-topic {
		display: flex;
		gap: 0.5rem;
	}

	.add-topic input {
		flex: 1;
		padding: 0.4rem 0.6rem;
		background: #2a2a2a;
		border: 1px solid #444;
		border-radius: 4px;
		color: inherit;
		font-size: 0.9rem;
	}

	.add-topic button {
		background: #444;
		color: #e8e4d9;
		padding: 0.4rem 0.8rem;
		border-radius: 4px;
		font-size: 0.85rem;
	}

	.add-topic button:disabled { opacity: 0.4; cursor: not-allowed; }

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
</style>
