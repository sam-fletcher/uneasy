<!-- Game shell: loads full game state, routes to phase-specific views. -->
<script lang="ts">
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import {
		getGameState, getIdentity, startToneSetting, startPrologue, startMainEvent,
		listScenePosts, createScenePost
	} from '$lib/api';
	import { createConnection, EventTypes, type WSMessage } from '$lib/ws';
	import type {
		Game, Player, ToneTopic, Ranking, ScenePost, PresenceMember
	} from '$lib/api';

	const gameID = $derived(page.params.id as string);

	// ── Core state ────────────────────────────────────────────────────────────
	let game = $state<Game | null>(null);
	let players = $state<Player[]>([]);
	let toneTopics = $state<ToneTopic[]>([]);
	let rankings = $state<Ranking[]>([]);
	let members = $state<PresenceMember[]>([]);
	let currentPlayerID = $state<number | null>(null);
	let error = $state('');
	let loading = $state(true);

	// Derived: is the current user the facilitator?
	const isFacilitator = $derived(
		currentPlayerID != null && players.some(p => p.id === currentPlayerID && p.is_facilitator)
	);

	// ── Typing indicators ─────────────────────────────────────────────────────
	let typingNames = $state<string[]>([]);
	let typingMap = new Map<number, string>();
	let typingTimeouts = new Map<number, ReturnType<typeof setTimeout>>();

	// ── Scene posts (for main_event phase) ────────────────────────────────────
	let scenePosts = $state<ScenePost[]>([]);
	let newPostBody = $state('');
	let sending = $state(false);
	let feedEl = $state<HTMLElement | null>(null);

	// Scroll the feed when posts change.
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
				if (game) {
					game = { ...game, phase: newPhase };
				}
				// Reload full state to get phase-specific data.
				loadGameState();
				break;
			}

			case EventTypes.PresenceSnapshot: {
				members = msg.payload.members as PresenceMember[];
				break;
			}

			case EventTypes.TypingUpdate: {
				const { player_id, display_name, typing } = msg.payload as {
					player_id: number;
					display_name: string;
					typing: boolean;
				};
				const existingTimeout = typingTimeouts.get(player_id);
				if (existingTimeout) clearTimeout(existingTimeout);

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
					topic_id: number;
					topic: string;
					status: string;
				};
				const idx = toneTopics.findIndex(t => t.id === topic_id);
				if (idx >= 0) {
					toneTopics = toneTopics.map(t =>
						t.id === topic_id ? { ...t, status: status as ToneTopic['status'] } : t
					);
				} else {
					// New topic added.
					toneTopics = [...toneTopics, { id: topic_id, game_id: Number(gameID), topic, status: status as ToneTopic['status'] }];
				}
				break;
			}

			case EventTypes.RankingsUpdated: {
				rankings = msg.payload.rankings as Ranking[];
				break;
			}

			case EventTypes.FocusChanged: {
				if (game) {
					game = { ...game, focus_player_id: msg.payload.player_id as number };
				}
				break;
			}

			case EventTypes.RowAdvanced: {
				if (game) {
					game = { ...game, current_row: msg.payload.row as number };
				}
				break;
			}

			case EventTypes.ScenePostCreated: {
				const post = msg.payload.post as ScenePost;
				if (!scenePosts.find(p => p.id === post.id)) {
					scenePosts = [...scenePosts, post];
				}
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
			// Get our player ID.
			const identity = await getIdentity();
			if (identity.player) {
				currentPlayerID = identity.player.id;
			}

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
				case 'lobby':
					await startToneSetting(gameID);
					break;
				case 'tone_setting':
					await startPrologue(gameID);
					break;
				case 'prologue':
					await startMainEvent(gameID);
					break;
			}
			// State will be updated via the WS phase.changed event.
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

	const typingLabel = $derived(
		typingNames.length === 0 ? '' :
		typingNames.length === 1 ? `${typingNames[0]} is writing…` :
		typingNames.length === 2 ? `${typingNames[0]} and ${typingNames[1]} are writing…` :
		'Several people are writing…'
	);

	// Map a phase to a human label.
	const phaseLabels: Record<string, string> = {
		lobby: 'Lobby',
		tone_setting: 'Tone Setting',
		prologue: 'Prologue',
		main_event: 'Main Event',
		shake_up: 'Shake-Up',
		ended: 'Game Over',
	};

	const focusPlayerName = $derived(
		game?.focus_player_id
			? players.find(p => p.id === game!.focus_player_id)?.display_name ?? '?'
			: null
	);
</script>

<div class="table-page">
	<!-- Header ────────────────────────────────────────────────────────────── -->
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
				Discuss what themes and topics your group wants to include or avoid. Anyone can change a topic's status.
			</p>
			<div class="tone-list">
				{#each toneTopics as topic (topic.id)}
					<div class="tone-row" data-status={topic.status}>
						<span class="tone-topic">{topic.topic}</span>
						<span class="tone-status">{topic.status.replace('_', ' ')}</span>
					</div>
				{/each}
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
				The facilitator sets initial rankings across Power, Knowledge, and Esteem, assigns seat order, and prepares the public record.
			</p>
			{#if rankings.length > 0}
				<div class="rankings-preview">
					{#each ['power', 'knowledge', 'esteem'] as cat}
						<div class="rank-col">
							<h3>{cat}</h3>
							{#each rankings.filter(r => r.category === cat).sort((a, b) => a.rank - b.rank) as r}
								<div class="rank-slot">
									{r.rank}. {r.player_id ? (players.find(p => p.id === r.player_id)?.display_name ?? '?') : 'dummy'}
								</div>
							{/each}
						</div>
					{/each}
				</div>
			{/if}
			{#if isFacilitator}
				<button class="primary" onclick={advancePhase} disabled={advancing}>
					{advancing ? '…' : nextPhaseLabel['prologue']}
				</button>
			{/if}
		</div>

	<!-- ── Main Event ─────────────────────────────────────────────────────── -->
	{:else if game?.phase === 'main_event'}
		<div class="phase-view main-event">
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

			<!-- Typing indicator -->
			<div class="typing-indicator" aria-live="polite">
				{typingLabel}
			</div>

			<!-- Input -->
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
		</div>

	<!-- ── Fallback ───────────────────────────────────────────────────────── -->
	{:else}
		<div class="center-message">Unknown phase.</div>
	{/if}
</div>

<style>
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

	.member.online {
		color: #e8e4d9;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
	}

	.member.online .dot {
		background: #6dbf7a;
	}

	.error {
		color: #e07070;
		font-size: 0.85rem;
		padding: 0.5rem 0;
	}

	.center-message {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: #888;
	}

	/* ── Phase views ──────────────────────────────────────────────────────── */

	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0;
		gap: 1rem;
		overflow-y: auto;
	}

	.phase-view h2 {
		color: #c8a96e;
		font-size: 1.3rem;
		margin: 0;
	}

	.muted {
		color: #999;
		font-size: 0.9rem;
		line-height: 1.5;
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

	/* ── Lobby ────────────────────────────────────────────────────────────── */

	.player-list {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

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

	/* ── Tone Setting ────────────────────────────────────────────────────── */

	.tone-list {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.tone-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.4rem 0.6rem;
		border-radius: 4px;
		background: #2a2a2a;
	}

	.tone-row[data-status='include'] {
		border-left: 3px solid #6dbf7a;
	}

	.tone-row[data-status='avoid_detail'] {
		border-left: 3px solid #e0c070;
	}

	.tone-row[data-status='never'] {
		border-left: 3px solid #e07070;
	}

	.tone-topic {
		font-size: 0.9rem;
	}

	.tone-status {
		font-size: 0.8rem;
		color: #888;
		text-transform: capitalize;
	}

	/* ── Prologue ─────────────────────────────────────────────────────────── */

	.rankings-preview {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.rank-col h3 {
		font-size: 0.85rem;
		color: #c8a96e;
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot {
		font-size: 0.85rem;
		color: #ccc;
		padding: 0.15rem 0;
	}

	/* ── Main Event ───────────────────────────────────────────────────────── */

	.main-event {
		/* Let feed grow, input stay at bottom */
		overflow: hidden;
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

	.send:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
</style>
