<!-- MainEventView.svelte
  Main event phase: collapsible retinue bar, public record sidebar, scene feed.
  Owns its local UI state (retinue open, post input, summary form, typing).
  recordRows and scenePosts are bindable so the parent WS handler can also update them.
-->
<script lang="ts">
	import {
		createScenePost, createSceneEntry,
		leverageAsset, refreshAsset, tearMarginalia,
	} from '$lib/api';
	import type { Game, Player, Asset, Marginalium, ScenePost, RecordRow } from '$lib/api';
	import AssetCard from '$lib/components/AssetCard.svelte';
	import PublicRecord from '$lib/components/PublicRecord.svelte';

	interface Props {
		game: Game;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		recordRows: RecordRow[];
		scenePosts: ScenePost[];
		typingLabel: string;
		playerNameMap: Map<number, string>;
	}

	let {
		game,
		players,
		assets,
		currentPlayerID,
		recordRows = $bindable(),
		scenePosts = $bindable(),
		typingLabel,
		playerNameMap,
	}: Props = $props();

	const myAssets = $derived(assets.filter(a => a.owner_id === currentPlayerID));

	const focusPlayerName = $derived(
		game.focus_player_id
			? players.find(p => p.id === game.focus_player_id)?.display_name ?? '?'
			: null
	);

	// ── Retinue panel ─────────────────────────────────────────────────────────
	let retinueOpen = $state(false);

	// ── Scene post input ──────────────────────────────────────────────────────
	let newPostBody = $state('');
	let sending = $state(false);
	let feedEl = $state<HTMLElement | null>(null);
	let error = $state('');

	let lastTypingSent = 0;
	let typingStopTimeout: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		if (scenePosts.length > 0 && feedEl) {
			feedEl.scrollTop = feedEl.scrollHeight;
		}
	});

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
		if (!body || sending) return;
		sending = true;
		try {
			const { post } = await createScenePost(game.id, game.current_row, body);
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

	// ── Scene summary ─────────────────────────────────────────────────────────
	let newSummaryBody = $state('');
	let sendingSummary = $state(false);
	let summaryOpen = $state(false);

	async function submitSummary() {
		const body = newSummaryBody.trim();
		if (!body || sendingSummary) return;
		sendingSummary = true;
		error = '';
		try {
			const { entry } = await createSceneEntry(game.id, game.current_row, body);
			newSummaryBody = '';
			summaryOpen = false;
			// Optimistic update — WS broadcast will also arrive but is deduplicated.
			recordRows = recordRows.map(row =>
				row.row_number === entry.row_number
					? { ...row, entries: row.entries.find(e => e.id === entry.id)
						? row.entries
						: [...row.entries, entry] }
					: row
			);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save summary.';
		} finally {
			sendingSummary = false;
		}
	}

	// ── Asset actions ─────────────────────────────────────────────────────────
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

	async function onTearMarginalia(asset: Asset, m: Marginalium) {
		if (!confirm(`Tear "${m.text}"? This cannot be undone.`)) return;
		try {
			await tearMarginalia(asset.id, m.position);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not tear marginalia.';
		}
	}
</script>

<div class="main-event-view">
	{#if error}
		<p class="local-error">{error}</p>
	{/if}

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
						<AssetCard
							{asset}
							compact
							onTear={onTearMarginalia}
							onToggleLeverage={toggleLeverage}
						/>
					{/each}
				{/if}
			</div>
		{/if}
	</div>

	<!-- Two-column play surface -->
	<div class="play-surface">

		<!-- Left: public record timeline -->
		<aside class="record-panel">
			<PublicRecord
				rows={recordRows}
				currentRow={game.current_row}
				playerNames={playerNameMap}
			/>
		</aside>

		<!-- Right: scene thread + input -->
		<div class="scene-panel">
			<div class="row-header">
				<span>Row <strong>{game.current_row}</strong> of 13</span>
				{#if focusPlayerName}
					<span class="focus-badge">Focus: {focusPlayerName}</span>
				{/if}
			</div>

			<div class="feed" bind:this={feedEl}>
				{#if scenePosts.length === 0}
					<p class="empty">No posts yet. Begin the scene.</p>
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

			<div class="summary-bar">
				{#if summaryOpen}
					<div class="summary-form">
						<textarea
							placeholder="Write a one-line summary for the public record…"
							bind:value={newSummaryBody}
							rows={2}
							disabled={sendingSummary}
							onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submitSummary(); } }}
						></textarea>
						<div class="summary-actions">
							<button
								class="primary"
								onclick={submitSummary}
								disabled={sendingSummary || !newSummaryBody.trim()}
							>
								{sendingSummary ? '…' : 'Add to Record'}
							</button>
							<button
								class="text-btn"
								onclick={() => { summaryOpen = false; newSummaryBody = ''; }}
							>
								Cancel
							</button>
						</div>
					</div>
				{:else}
					<button class="summary-toggle" onclick={() => { summaryOpen = true; }}>
						+ Add to public record
					</button>
				{/if}
			</div>
		</div>

	</div>
</div>

<style>
	.main-event-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-height: 0;
	}

	.local-error {
		color: #e07070;
		font-size: 0.85rem;
		padding: 0.3rem 0;
		flex-shrink: 0;
	}

	/* ── Retinue bar ─────────────────────────────────────────────────────────── */

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
		max-height: 240px;
		overflow-y: auto;
	}

	/* ── Play surface ────────────────────────────────────────────────────────── */

	.play-surface {
		flex: 1;
		display: grid;
		grid-template-columns: 220px 1fr;
		min-height: 0;
		overflow: hidden;
	}

	@media (max-width: 600px) {
		.play-surface {
			grid-template-columns: 1fr;
			grid-template-rows: 180px 1fr;
		}
	}

	/* ── Public record panel ─────────────────────────────────────────────────── */

	.record-panel {
		border-right: 1px solid #2a2a2a;
		padding: 0.75rem 0.6rem 0.75rem 0;
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}

	@media (max-width: 600px) {
		.record-panel {
			border-right: none;
			border-bottom: 1px solid #2a2a2a;
			padding: 0.5rem 0;
		}
	}

	/* ── Scene panel ─────────────────────────────────────────────────────────── */

	.scene-panel {
		display: flex;
		flex-direction: column;
		padding: 0.75rem 0 0 0.75rem;
		overflow: hidden;
		min-height: 0;
	}

	@media (max-width: 600px) {
		.scene-panel { padding: 0.5rem 0 0; }
	}

	.row-header {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		font-size: 0.85rem;
		color: #c8a96e;
		padding-bottom: 0.4rem;
		border-bottom: 1px solid #333;
		flex-shrink: 0;
	}

	.focus-badge {
		background: #3a3020;
		padding: 0.12rem 0.4rem;
		border-radius: 4px;
		font-size: 0.75rem;
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
		font-size: 0.85rem;
	}

	.post {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.4rem;
		align-items: baseline;
	}

	.post-author {
		font-weight: 600;
		color: #c8a96e;
		font-size: 0.85rem;
		white-space: nowrap;
	}

	.post-body {
		font-size: 0.9rem;
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.post-time {
		font-size: 0.72rem;
		color: #555;
		white-space: nowrap;
	}

	.typing-indicator {
		font-size: 0.78rem;
		color: #777;
		height: 1.2em;
		flex-shrink: 0;
	}

	/* ── Post input ──────────────────────────────────────────────────────────── */

	.input-row {
		display: flex;
		gap: 0.5rem;
		padding-top: 0.4rem;
		border-top: 1px solid #2a2a2a;
		align-items: flex-end;
		flex-shrink: 0;
	}

	textarea {
		flex: 1;
		font-size: 0.9rem;
		padding: 0.5rem 0.7rem;
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
		padding: 0.5rem 0.9rem;
		min-width: 56px;
		align-self: flex-end;
		border-radius: 6px;
	}

	.send:disabled { opacity: 0.4; cursor: not-allowed; }

	/* ── Summary bar ─────────────────────────────────────────────────────────── */

	.summary-bar {
		flex-shrink: 0;
		padding-top: 0.4rem;
		border-top: 1px solid #222;
	}

	.summary-toggle {
		background: none;
		color: #8a6a3a;
		font-size: 0.78rem;
		padding: 0.25rem 0;
		cursor: pointer;
		text-decoration: underline dotted;
	}

	.summary-toggle:hover { color: #c8a96e; }

	.summary-form {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.summary-actions {
		display: flex;
		gap: 0.75rem;
		align-items: center;
	}
</style>
