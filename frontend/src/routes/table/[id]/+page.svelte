<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import {
		getGameState, getIdentity,
		startToneSetting, startPrologue, startMainEvent,
		updateToneTopic, addToneTopic,
		listAssets, createAsset, updateAsset,
		addMarginalia, updateMarginalia, tearMarginalia,
		leverageAsset, refreshAsset,
		setRankings, setSeats,
		listScenePosts, createScenePost,
		type RankingCategory,
	} from '$lib/api';
	import { createConnection, EventTypes, type WSMessage } from '$lib/ws';
	import type {
		Game, Player, ToneTopic, Ranking, Asset, Marginalium, ScenePost, PresenceMember
	} from '$lib/api';

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
	const myAssets = $derived(assets.filter(a => a.owner_id === currentPlayerID));

	// ── Typing indicators ─────────────────────────────────────────────────────
	let typingNames = $state<string[]>([]);
	let typingMap = new Map<number, string>();
	let typingTimeouts = new Map<number, ReturnType<typeof setTimeout>>();

	// ── Scene posts (for main_event phase) ────────────────────────────────────
	let scenePosts = $state<ScenePost[]>([]);
	let newPostBody = $state('');
	let sending = $state(false);
	let feedEl = $state<HTMLElement | null>(null);

	$effect(() => {
		if (scenePosts.length > 0 && feedEl) {
			feedEl.scrollTop = feedEl.scrollHeight;
		}
	});

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
				break;
			}
			case EventTypes.RowAdvanced: {
				if (game) game = { ...game, current_row: msg.payload.row as number };
				break;
			}
			case EventTypes.ScenePostCreated: {
				const post = msg.payload.post as ScenePost;
				if (!scenePosts.find(p => p.id === post.id)) {
					scenePosts = [...scenePosts, post];
				}
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
		}
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

			// Load scene posts if in main_event.
			if (data.game.phase === 'main_event' && data.game.current_row > 0) {
				const postsData = await listScenePosts(gameID, data.game.current_row);
				scenePosts = postsData.posts;
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

	// ── Phase advancement ─────────────────────────────────────────────────────
	let advancing = $state(false);

	async function advancePhase() {
		if (!game || advancing) return;
		advancing = true;
		error = '';
		try {
			switch (game.phase) {
				case 'lobby':        await startToneSetting(gameID); break;
				case 'tone_setting': await startPrologue(gameID); break;
				case 'prologue':     await startMainEvent(gameID); break;
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
		prologue: 'Start Main Event',
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

	// ── Asset creation ────────────────────────────────────────────────────────
	let newAssetType = $state<Asset['asset_type']>('peer');
	let newAssetName = $state('');
	let newAssetIsMain = $state(false);
	let newAssetMarginalia = $state(['', '']);
	let creatingAsset = $state(false);

	async function submitAsset() {
		const name = newAssetName.trim();
		if (!name || creatingAsset) return;
		creatingAsset = true;
		error = '';
		try {
			const marginalia = newAssetMarginalia.map(m => m.trim()).filter(Boolean);
			await createAsset(gameID, {
				asset_type: newAssetType,
				name,
				is_main_character: newAssetIsMain && newAssetType === 'peer',
				marginalia,
			});
			newAssetName = '';
			newAssetIsMain = false;
			newAssetMarginalia = ['', ''];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create asset.';
		} finally {
			creatingAsset = false;
		}
	}

	// Leverage / refresh toggle on an asset card.
	async function toggleLeverage(asset: Asset) {
		try {
			if (asset.is_leveraged) {
				await refreshAsset(asset.id);
			} else {
				await leverageAsset(asset.id);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not toggle leverage.';
		}
	}

	// Tear a marginalia.
	async function onTearMarginalia(asset: Asset, m: Marginalium) {
		if (!confirm(`Tear "${m.text}"? This cannot be undone.`)) return;
		try {
			await tearMarginalia(asset.id, m.position);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not tear marginalia.';
		}
	}

	// ── Rankings (facilitator) ────────────────────────────────────────────────
	// draftRankings[category][rank-1] = player_id | null (null = dummy)
	type RankSlot = number | null | 'unset';
	let draftRankings = $state<Record<RankingCategory, RankSlot[]>>({
		power:     ['unset','unset','unset','unset','unset'],
		knowledge: ['unset','unset','unset','unset','unset'],
		esteem:    ['unset','unset','unset','unset','unset'],
	});
	let savingRankings = $state(false);

	// Initialise draft when rankings load from server.
	$effect(() => {
		if (rankings.length > 0) {
			const draft: Record<RankingCategory, RankSlot[]> = {
				power:     ['unset','unset','unset','unset','unset'],
				knowledge: ['unset','unset','unset','unset','unset'],
				esteem:    ['unset','unset','unset','unset','unset'],
			};
			for (const r of rankings) {
				draft[r.category][r.rank - 1] = r.player_id ?? null;
			}
			draftRankings = draft;
		}
	});

	async function saveRankings() {
		// Validate all 15 slots are set.
		const entries: Array<{ player_id: number | null; category: RankingCategory; rank: number }> = [];
		for (const cat of ['power', 'knowledge', 'esteem'] as RankingCategory[]) {
			for (let i = 0; i < 5; i++) {
				const val = draftRankings[cat][i];
				if (val === 'unset') {
					error = `Please fill all ranking slots (missing: ${cat} rank ${i + 1})`;
					return;
				}
				entries.push({ player_id: val as number | null, category: cat, rank: i + 1 });
			}
		}
		savingRankings = true;
		error = '';
		try {
			const result = await setRankings(gameID, entries);
			rankings = result.rankings;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save rankings.';
		} finally {
			savingRankings = false;
		}
	}

	// ── Seat order (facilitator) ──────────────────────────────────────────────
	// draftSeats[player_id] = seat number
	let draftSeats = $state<Record<number, string>>({});
	let savingSeats = $state(false);

	$effect(() => {
		const seats: Record<number, string> = {};
		for (const p of players) {
			seats[p.id] = p.seat_order != null ? String(p.seat_order) : '';
		}
		draftSeats = seats;
	});

	async function saveSeats() {
		const seats: Array<{ player_id: number; seat_order: number }> = [];
		for (const p of players) {
			const raw = draftSeats[p.id];
			const n = parseInt(raw, 10);
			if (!raw || isNaN(n) || n < 1) {
				error = `Enter a valid seat number for ${p.display_name}`;
				return;
			}
			seats.push({ player_id: p.id, seat_order: n });
		}
		savingSeats = true;
		error = '';
		try {
			await setSeats(gameID, seats);
			// Optimistically update local player seat_order.
			players = players.map(p => {
				const s = seats.find(s => s.player_id === p.id);
				return s ? { ...p, seat_order: s.seat_order } : p;
			});
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save seats.';
		} finally {
			savingSeats = false;
		}
	}

	// ── Scene post input ──────────────────────────────────────────────────────
	let lastTypingSent = 0;
	let typingStopTimeout: ReturnType<typeof setTimeout> | null = null;

	function onInput() {
		const now = Date.now();
		if (now - lastTypingSent > 2500) {
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: true } }));
			lastTypingSent = now;
		}
		if (typingStopTimeout) clearTimeout(typingStopTimeout);
		typingStopTimeout = setTimeout(() => {
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: false } }));
		}, 2000);
	}

	async function sendPost() {
		const body = newPostBody.trim();
		if (!body || sending || !game) return;
		sending = true;
		try {
			const { post } = await createScenePost(gameID, game.current_row, body);
			newPostBody = '';
			if (!scenePosts.find(p => p.id === post.id)) {
				scenePosts = [...scenePosts, post];
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send.';
		} finally {
			sending = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendPost();
		}
	}

	// ── Shared display helpers ────────────────────────────────────────────────
	const typingLabel = $derived(
		typingNames.length === 0 ? '' :
		typingNames.length === 1 ? `${typingNames[0]} is writing…` :
		typingNames.length === 2 ? `${typingNames[0]} and ${typingNames[1]} are writing…` :
		'Several people are writing…'
	);

	const phaseLabels: Record<string, string> = {
		lobby: 'Lobby',
		tone_setting: 'Tone Setting',
		prologue: 'Prologue',
		main_event: 'Main Event',
		shake_up: 'Shake-Up',
		ended: 'Game Over',
	};

	const assetTypeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	const focusPlayerName = $derived(
		game?.focus_player_id
			? players.find(p => p.id === game!.focus_player_id)?.display_name ?? '?'
			: null
	);

	// Player name lookup used in ranking display.
	function rankingLabel(playerID: number | null): string {
		if (playerID === null) return 'Dummy';
		return players.find(p => p.id === playerID)?.display_name ?? '?';
	}

	// Build the option list for a ranking slot select:
	// real players + one dummy entry per track beyond the player count.
	const rankingOptions = $derived([
		...players.map(p => ({ value: String(p.id), label: p.display_name })),
		{ value: 'null', label: 'Dummy token' },
	]);

	// Whether all 15 ranking slots are set (used to gate the start button).
	const rankingsComplete = $derived(
		(['power', 'knowledge', 'esteem'] as RankingCategory[])
			.every(cat => draftRankings[cat].every(v => v !== 'unset'))
	);

	// Whether all players have a seat order set.
	const seatsComplete = $derived(
		players.length > 0 && players.every(p => p.seat_order != null)
	);

	// Retinue panel open state (main_event).
	let retinueOpen = $state(false);
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
		<div class="phase-view prologue">
			<h2>Prologue</h2>
			<p class="muted">
				Each player creates their main character (a peer) plus any additional assets.
				{#if isFacilitator}
					As facilitator, you also set the initial rankings and seat order before starting.
				{/if}
			</p>

			<div class="prologue-columns">
				<!-- Left column: asset creation + retinue -->
				<section class="prologue-section">
					<h3>Create an Asset</h3>
					<div class="asset-form">
						<div class="form-row">
							<label>
								Type
								<select bind:value={newAssetType}>
									<option value="peer">Peer</option>
									<option value="holding">Holding</option>
									<option value="artifact">Artifact</option>
									<option value="resource">Resource</option>
								</select>
							</label>
							<label class="name-label">
								Name
								<input
									type="text"
									bind:value={newAssetName}
									placeholder="Asset name…"
									maxlength={80}
								/>
							</label>
						</div>

						{#if newAssetType === 'peer'}
							<label class="checkbox-label">
								<input type="checkbox" bind:checked={newAssetIsMain} />
								Main character (sets this peer as your character)
							</label>
						{/if}

						<div class="marginalia-inputs">
							<span class="field-label">Marginalia (optional, up to 4)</span>
							{#each newAssetMarginalia as _, i}
								<input
									type="text"
									placeholder="Marginalia {i + 1}…"
									bind:value={newAssetMarginalia[i]}
									maxlength={200}
								/>
							{/each}
							{#if newAssetMarginalia.length < 4}
								<button class="text-btn" onclick={() => { newAssetMarginalia = [...newAssetMarginalia, '']; }}>
									+ Add marginalia
								</button>
							{/if}
						</div>

						<button
							class="primary"
							onclick={submitAsset}
							disabled={!newAssetName.trim() || creatingAsset}
						>
							{creatingAsset ? '…' : 'Create Asset'}
						</button>
					</div>

					{#if myAssets.length > 0}
						<h3 style="margin-top: 1.5rem;">Your Retinue</h3>
						<div class="asset-list">
							{#each myAssets as asset (asset.id)}
								<div class="asset-card" class:main-char={asset.is_main_character}>
									<div class="asset-header">
										<span class="asset-name">
											{asset.name}
											{#if asset.is_main_character}
												<span class="tag main-tag">★ main</span>
											{/if}
										</span>
										<span class="asset-type-badge">{assetTypeLabels[asset.asset_type]}</span>
									</div>
									{#if asset.marginalia.length > 0}
										<ul class="marginalia-list">
											{#each asset.marginalia as m (m.id)}
												<li class:torn={m.is_torn}>
													<span class="m-text">{m.text}</span>
													{#if !m.is_torn}
														<button
															class="tear-btn"
															title="Tear this marginalia"
															onclick={() => onTearMarginalia(asset, m)}
														>✂</button>
													{:else}
														<span class="torn-label">torn</span>
													{/if}
												</li>
											{/each}
										</ul>
									{:else}
										<p class="no-marginalia">No marginalia yet.</p>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<!-- Right column: rankings + seats (facilitator only) -->
				{#if isFacilitator}
					<section class="prologue-section facilitator-panel">
						<h3>Initial Rankings</h3>
						<p class="muted small">
							Assign each rank (1 = highest) to a player or a dummy token.
							You need to fill all 15 slots before starting.
						</p>

						<div class="rankings-grid">
							{#each ['power', 'knowledge', 'esteem'] as cat}
								<div class="rank-col">
									<h4>{cat}</h4>
									{#each [1,2,3,4,5] as rank}
										<div class="rank-slot">
											<span class="rank-num">{rank}</span>
											<select
												value={
													draftRankings[cat as RankingCategory][rank - 1] === 'unset'
														? ''
														: draftRankings[cat as RankingCategory][rank - 1] === null
															? 'null'
															: String(draftRankings[cat as RankingCategory][rank - 1])
												}
												onchange={(e) => {
													const v = (e.target as HTMLSelectElement).value;
													draftRankings[cat as RankingCategory][rank - 1] =
														v === '' ? 'unset' : v === 'null' ? null : Number(v);
												}}
											>
												<option value="">— pick —</option>
												{#each rankingOptions as opt}
													<option value={opt.value}>{opt.label}</option>
												{/each}
											</select>
										</div>
									{/each}
								</div>
							{/each}
						</div>

						<button
							class="secondary"
							onclick={saveRankings}
							disabled={savingRankings}
						>
							{savingRankings ? '…' : 'Save Rankings'}
						</button>

						<h3 style="margin-top: 1.5rem;">Seat Order</h3>
						<p class="muted small">
							Assign a clockwise seat number to each player (1, 2, 3…).
						</p>

						<div class="seat-grid">
							{#each players as p}
								<div class="seat-row">
									<span class="seat-name">{p.display_name}</span>
									<input
										type="number"
										min="1"
										max={players.length}
										placeholder="#"
										class="seat-input"
										value={draftSeats[p.id] ?? ''}
										oninput={(e) => { draftSeats[p.id] = (e.target as HTMLInputElement).value; }}
									/>
								</div>
							{/each}
						</div>

						<button
							class="secondary"
							onclick={saveSeats}
							disabled={savingSeats}
						>
							{savingSeats ? '…' : 'Save Seat Order'}
						</button>

						<div class="start-section">
							<button
								class="primary"
								onclick={advancePhase}
								disabled={advancing || !rankingsComplete || !seatsComplete}
								title={
									!rankingsComplete ? 'Rankings are not fully set' :
									!seatsComplete    ? 'Seat order is not fully set' : undefined
								}
							>
								{advancing ? '…' : 'Start Main Event'}
							</button>
							{#if !rankingsComplete}
								<p class="hint">Fill all 15 ranking slots first.</p>
							{:else if !seatsComplete}
								<p class="hint">Assign a seat to every player first.</p>
							{/if}
						</div>
					</section>
				{/if}
			</div>

			<!-- Show current rankings to non-facilitators if set -->
			{#if !isFacilitator && rankings.length > 0}
				<div class="rankings-preview">
					{#each ['power', 'knowledge', 'esteem'] as cat}
						<div class="rank-col">
							<h3>{cat}</h3>
							{#each rankings.filter(r => r.category === cat).sort((a, b) => a.rank - b.rank) as r}
								<div class="rank-slot-display">
									{r.rank}. {rankingLabel(r.player_id)}
								</div>
							{/each}
						</div>
					{/each}
				</div>
			{/if}
		</div>

	<!-- ── Main Event ─────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'main_event'}
		<div class="phase-view main-event">
			<!-- Retinue panel (collapsible) -->
			<div class="retinue-bar">
				<button class="retinue-toggle" onclick={() => { retinueOpen = !retinueOpen; }}>
					Your Retinue ({myAssets.length})
					<span class="chevron">{retinueOpen ? '▲' : '▼'}</span>
				</button>
				{#if retinueOpen}
					<div class="retinue-panel">
						{#if myAssets.length === 0}
							<p class="muted">You have no assets.</p>
						{:else}
							{#each myAssets as asset (asset.id)}
								<div class="asset-card-compact" class:leveraged={asset.is_leveraged}>
									<div class="asset-header">
										<span class="asset-name">
											{asset.name}
											{#if asset.is_main_character}<span class="tag main-tag">★</span>{/if}
										</span>
										<div class="asset-actions">
											<span class="asset-type-badge">{assetTypeLabels[asset.asset_type]}</span>
											<button
												class="lev-btn"
												class:active={asset.is_leveraged}
												onclick={() => toggleLeverage(asset)}
												title={asset.is_leveraged ? 'Refresh (un-leverage)' : 'Leverage'}
											>
												{asset.is_leveraged ? '⊙ leveraged' : '○ leverage'}
											</button>
										</div>
									</div>
									{#if asset.marginalia.length > 0}
										<ul class="marginalia-list compact">
											{#each asset.marginalia as m (m.id)}
												<li class:torn={m.is_torn}>
													<span class="m-text">{m.text}</span>
													{#if !m.is_torn}
														<button class="tear-btn" onclick={() => onTearMarginalia(asset, m)}>✂</button>
													{:else}
														<span class="torn-label">torn</span>
													{/if}
												</li>
											{/each}
										</ul>
									{/if}
								</div>
							{/each}
						{/if}
					</div>
				{/if}
			</div>

			<div class="row-header">
				<span>Row {game.current_row} of 13</span>
				{#if focusPlayerName}
					<span class="focus-badge">Focus: {focusPlayerName}</span>
				{/if}
			</div>

			<!-- Scene post feed -->
			<div class="feed" bind:this={feedEl}>
				{#if scenePosts.length === 0}
					<p class="empty">The public record is empty. Begin the scene.</p>
				{:else}
					{#each scenePosts as post (post.id)}
						<div class="post">
							<span class="post-author">
								{players.find(p => p.id === post.author_id)?.display_name ?? 'Unknown'}
							</span>
							<span class="post-body">{post.body}</span>
							<span class="post-time">{new Date(post.created_at).toLocaleTimeString()}</span>
						</div>
					{/each}
				{/if}
			</div>

			<div class="typing-indicator" aria-live="polite">{typingLabel}</div>

			<div class="input-row">
				<textarea
					placeholder="Write something… (Enter to send, Shift+Enter for newline)"
					bind:value={newPostBody}
					oninput={onInput}
					onkeydown={onKeydown}
					rows={2}
					disabled={sending}
				></textarea>
				<button class="send" onclick={sendPost} disabled={sending || !newPostBody.trim()}>
					{sending ? '…' : 'Send'}
				</button>
			</div>
		</div>

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
							{#each rankings.filter(r => r.category === cat).sort((a, b) => a.rank - b.rank) as r}
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

	.muted.small { font-size: 0.8rem; }

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

	.secondary {
		background: #333;
		color: #e8e4d9;
		font-weight: 600;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		align-self: flex-start;
		border: 1px solid #555;
	}

	.secondary:disabled { opacity: 0.4; cursor: not-allowed; }

	.text-btn {
		background: none;
		color: #c8a96e;
		padding: 0;
		font-size: 0.85rem;
		text-decoration: underline;
		cursor: pointer;
	}

	.hint {
		font-size: 0.8rem;
		color: #e0a060;
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
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
	}

	.main-tag {
		background: #4a3010;
		color: #e8c080;
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

	/* ── Prologue ───────────────────────────────────────────────────────────── */

	.prologue-columns {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 2rem;
		align-items: start;
	}

	@media (max-width: 700px) {
		.prologue-columns { grid-template-columns: 1fr; }
	}

	.prologue-section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.asset-form {
		background: #222;
		border-radius: 8px;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.form-row {
		display: flex;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.form-row label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.8rem;
		color: #aaa;
	}

	.name-label { flex: 1; }

	.form-row select, .form-row input {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.9rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.85rem;
		color: #ccc;
	}

	.field-label {
		font-size: 0.8rem;
		color: #aaa;
	}

	.marginalia-inputs {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.marginalia-inputs input {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.85rem;
	}

	/* ── Asset cards ──────────────────────────────────────────────────────── */

	.asset-list { display: flex; flex-direction: column; gap: 0.6rem; }

	.asset-card {
		background: #242420;
		border: 1px solid #444;
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.asset-card.main-char { border-color: #c8a96e; }

	.asset-card-compact {
		background: #242420;
		border: 1px solid #444;
		border-radius: 6px;
		padding: 0.4rem 0.6rem;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.asset-card-compact.leveraged { border-color: #6090c8; opacity: 0.75; }

	.asset-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.asset-name {
		font-weight: 600;
		font-size: 0.9rem;
		color: #e8e4d9;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.asset-type-badge {
		font-size: 0.7rem;
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.asset-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.lev-btn {
		font-size: 0.75rem;
		color: #888;
		padding: 0.15rem 0.4rem;
		border: 1px solid #555;
		border-radius: 3px;
	}

	.lev-btn.active {
		color: #6090c8;
		border-color: #6090c8;
	}

	.marginalia-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.marginalia-list li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 0.82rem;
		color: #bbb;
		gap: 0.4rem;
	}

	.marginalia-list li.torn { opacity: 0.45; }

	.marginalia-list.compact li { font-size: 0.78rem; }

	.m-text { flex: 1; }

	.tear-btn {
		color: #c07070;
		font-size: 0.8rem;
		padding: 0;
		background: none;
		flex-shrink: 0;
		opacity: 0.6;
	}

	.tear-btn:hover { opacity: 1; }

	.torn-label {
		font-size: 0.7rem;
		color: #666;
		flex-shrink: 0;
	}

	.no-marginalia {
		font-size: 0.8rem;
		color: #666;
		margin: 0;
	}

	/* ── Facilitator panel ─────────────────────────────────────────────────── */

	.facilitator-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 1rem;
	}

	.rankings-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.rank-col h4 {
		font-size: 0.8rem;
		color: #c8a96e;
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.3rem;
	}

	.rank-num {
		font-size: 0.75rem;
		color: #666;
		width: 1rem;
		flex-shrink: 0;
	}

	.rank-slot select {
		flex: 1;
		font-size: 0.8rem;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 3px;
		padding: 0.2rem 0.3rem;
	}

	.seat-grid {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		margin-bottom: 0.75rem;
	}

	.seat-row {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.seat-name {
		flex: 1;
		font-size: 0.9rem;
	}

	.seat-input {
		width: 3.5rem;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.25rem 0.4rem;
		font-size: 0.9rem;
		text-align: center;
	}

	.start-section {
		margin-top: 1.25rem;
		padding-top: 1rem;
		border-top: 1px solid #333;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	/* ── Rankings preview (non-facilitator) ───────────────────────────────── */

	.rankings-preview {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.rank-slot-display {
		font-size: 0.85rem;
		color: #ccc;
		padding: 0.15rem 0;
	}

	/* ── Main Event ─────────────────────────────────────────────────────────── */

	.main-event {
		overflow: hidden;
	}

	.retinue-bar {
		flex-shrink: 0;
		border-bottom: 1px solid #333;
	}

	.retinue-toggle {
		width: 100%;
		text-align: left;
		padding: 0.4rem 0;
		font-size: 0.85rem;
		color: #c8a96e;
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: none;
	}

	.chevron { font-size: 0.7rem; }

	.retinue-panel {
		padding: 0.5rem 0 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		max-height: 280px;
		overflow-y: auto;
	}

	.row-header {
		display: flex;
		gap: 1rem;
		align-items: center;
		font-size: 0.9rem;
		color: #c8a96e;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid #333;
		flex-shrink: 0;
	}

	.focus-badge {
		background: #3a3020;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		font-size: 0.8rem;
	}

	.feed {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		min-height: 0;
	}

	.empty {
		color: #666;
		text-align: center;
		margin-top: 2rem;
	}

	.post {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.5rem;
		align-items: baseline;
	}

	.post-author {
		font-weight: 600;
		color: #c8a96e;
		font-size: 0.9rem;
		white-space: nowrap;
	}

	.post-body {
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.post-time {
		font-size: 0.75rem;
		color: #666;
		white-space: nowrap;
	}

	.typing-indicator {
		font-size: 0.8rem;
		color: #888;
		height: 1.2em;
		flex-shrink: 0;
	}

	.input-row {
		display: flex;
		gap: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid #333;
		align-items: flex-end;
		flex-shrink: 0;
	}

	textarea {
		flex: 1;
		font-size: 1rem;
		padding: 0.6rem 0.8rem;
		border-radius: 6px;
		border: 1px solid #444;
		background: #2a2a2a;
		color: inherit;
		font-family: inherit;
		resize: none;
		line-height: 1.4;
	}

	textarea:focus {
		outline: 2px solid #c8a96e;
		outline-offset: 1px;
	}

	.send {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.6rem 1rem;
		min-width: 60px;
		align-self: flex-end;
	}

	.send:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
