<!-- MainEventView.svelte
  Main event phase: collapsible retinue bar, public record sidebar, scene feed.
  Owns its local UI state (retinue open, post input, summary form, typing).
  recordRows and scenePosts are bindable so the parent WS handler can also update them.
-->
<script lang="ts">
	import {
		createScenePost, createSceneEntry,
		leverageAsset, refreshAsset, tearMarginalia,
		endScene, refreshAssets, passFocus, createRoll,
	} from '$lib/api';
	import type { Game, Player, Asset, Marginalium, ScenePost, RecordRow, DiceRoll, DiceRollDie, DifficultyVote, Plan } from '$lib/api';
	import AssetCard from '$lib/components/AssetCard.svelte';
	import PublicRecord from '$lib/components/PublicRecord.svelte';
	import DiceRollPanel from '$lib/components/DiceRollPanel.svelte';
	import PlanPanel from '$lib/components/PlanPanel.svelte';

	interface Props {
		game: Game;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		recordRows: RecordRow[];
		scenePosts: ScenePost[];
		sceneEnded: boolean;
		typingLabel: string;
		playerNameMap: Map<number, string>;
		isFacilitator: boolean;
		/** Active (unresolved) dice roll, or null if none. */
		activeRoll: DiceRoll | null;
		activeRollDice: DiceRollDie[];
		activeRollVotes: DifficultyVote[];
		voteOpen: boolean;
		/** All plans for this game — owned and fetched by the parent; read-only here. */
		plans: Plan[];
		/**
		 * Called after any plan mutation so the parent can re-fetch and push updated
		 * plans back down. The parent owns plan state; this component never writes it.
		 */
		onPlansChanged: () => void;
	}

	let {
		game,
		players,
		assets,
		currentPlayerID,
		recordRows = $bindable(),
		scenePosts = $bindable(),
		sceneEnded = $bindable(),
		typingLabel,
		playerNameMap,
		isFacilitator,
		activeRoll = $bindable(),
		activeRollDice = $bindable(),
		activeRollVotes = $bindable(),
		voteOpen = $bindable(),
		plans,
		onPlansChanged,
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

	// ── Focus-player action bar ───────────────────────────────────────────────

	const isFocusPlayer = $derived(
		currentPlayerID != null && game.focus_player_id === currentPlayerID
	);

	// Refresh-assets sub-step: which leveraged assets the player has selected.
	let refreshable = $derived(assets.filter(a => a.owner_id === currentPlayerID && a.is_leveraged && !a.is_destroyed));
	let selectedRefreshIDs = $state<Set<number>>(new Set());
	let maxRefresh = $derived(game.current_row);

	// Reset selections when assets or step changes.
	$effect(() => {
		if (!sceneEnded) selectedRefreshIDs = new Set();
	});

	function toggleRefreshSelection(id: number) {
		const next = new Set(selectedRefreshIDs);
		if (next.has(id)) {
			next.delete(id);
		} else if (next.size < maxRefresh) {
			next.add(id);
		}
		selectedRefreshIDs = next;
	}

	let actionBusy = $state(false);

	async function onEndScene() {
		if (actionBusy) return;
		actionBusy = true;
		error = '';
		try {
			await endScene(game.id);
			// The server broadcasts scene.ended; the parent page sets sceneEnded = true.
			// We set it locally too so the UI is instant for the focus player.
			sceneEnded = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not end scene.';
		} finally {
			actionBusy = false;
		}
	}

	async function onRefreshAssets() {
		if (actionBusy) return;
		actionBusy = true;
		error = '';
		try {
			await refreshAssets(game.id, [...selectedRefreshIDs]);
			selectedRefreshIDs = new Set();
			// Assets are updated via the asset.refreshed WS event; no local state needed.
			// Move to the "done" step by marking actionTaken.
			actionTaken = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not refresh assets.';
		} finally {
			actionBusy = false;
		}
	}

	// actionTaken: focus player has chosen their action (refresh or skip).
	// Together with sceneEnded, it drives the action bar step.
	let actionTaken = $state(false);

	// Reset actionTaken when sceneEnded resets (new row or new focus).
	$effect(() => {
		if (!sceneEnded) actionTaken = false;
	});

	async function onSkipRefresh() {
		// Player opts not to refresh any assets.
		actionTaken = true;
	}

	async function onPassFocus() {
		if (actionBusy) return;
		actionBusy = true;
		error = '';
		try {
			await passFocus(game.id);
			// focus.changed WS event will update the parent; sceneEnded resets.
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not pass focus.';
		} finally {
			actionBusy = false;
		}
	}

	// ── Plan state ────────────────────────────────────────────────────────────

	/** True when there is an active resolving plan or a pending plan on the current row. */
	const hasPlansToResolve = $derived(
		plans.some(p => p.status === 'resolving') ||
		plans.some(p => p.status === 'pending' && p.row_number === game.current_row)
	);

	/** True when an in-flight roll hasn't resolved yet. */
	const rollActive = $derived(activeRoll != null && activeRoll.outcome == null);

	/**
	 * The make/mar outcome of a plan-linked roll, once resolved.
	 * Only set when the active roll is tied to a plan — free-scene rolls
	 * don't drive the plan resolution flow.
	 */
	const rollOutcome = $derived(
		(activeRoll?.plan_id != null && activeRoll.outcome != null)
			? (activeRoll.outcome as 'make' | 'mar')
			: null
	);

	/** Called by PlanPanel when it creates a plan-linked dice roll. */
	function onPlanRollCreated(roll: DiceRoll) {
		activeRoll = roll;
		activeRollDice = [];
		activeRollVotes = [];
		voteOpen = false;
	}

	/**
	 * Called by PlanPanel specifically when the focus player prepares a plan —
	 * their chosen step-2 action. Triggers a parent re-fetch and advances the
	 * local action bar state.
	 */
	function onPlanPrepared() {
		onPlansChanged();
		actionTaken = true;
	}

	// ── Dice roll creation ────────────────────────────────────────────────────
	let showRollForm = $state(false);
	let rollDifficulty = $state(3);
	let rollingBusy = $state(false);

	async function onStartRoll() {
		if (rollingBusy) return;
		rollingBusy = true;
		error = '';
		try {
			const { roll } = await createRoll(game.id, rollDifficulty);
			activeRoll = roll;
			activeRollDice = [];
			activeRollVotes = [];
			voteOpen = false;
			showRollForm = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not start roll.';
		} finally {
			rollingBusy = false;
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

			<!-- ── Plan panel ───────────────────────────────────────────────── -->
			<!--
				Shown in two situations:
				1. A plan is currently resolving or pending on this row (visible to all).
				2. The focus player is in their post-scene action step (prep enabled).
			-->
			<PlanPanel
				gameID={game.id}
				currentRow={game.current_row}
				{plans}
				{assets}
				{players}
				{currentPlayerID}
				{isFocusPlayer}
				prepEnabled={isFocusPlayer && sceneEnded && !actionTaken}
				{rollActive}
				{rollOutcome}
				onRollCreated={onPlanRollCreated}
				{onPlansChanged}
				{onPlanPrepared}
			/>

			<!-- ── Dice roll panel ───────────────────────────────────────────── -->
			{#if activeRoll}
				<DiceRollPanel
					bind:roll={activeRoll}
					bind:dice={activeRollDice}
					bind:votes={activeRollVotes}
					bind:voteOpen
					{assets}
					{currentPlayerID}
					{players}
					{playerNameMap}
					{isFacilitator}
				/>
			{:else if game.phase === 'main_event'}
				<!-- Any player can initiate an in-scene roll -->
				{#if showRollForm}
					<div class="roll-start-form">
						<label class="roll-form-label">
							Difficulty (1–6):
							<input
								type="number"
								min="1"
								max="6"
								bind:value={rollDifficulty}
								class="diff-input"
							/>
						</label>
						<div class="roll-form-actions">
							<button class="action-btn primary" onclick={onStartRoll} disabled={rollingBusy}>
								{rollingBusy ? '…' : 'Start Roll'}
							</button>
							<button class="action-btn secondary" onclick={() => { showRollForm = false; }}>
								Cancel
							</button>
						</div>
					</div>
				{:else}
					<button class="roll-init-btn" onclick={() => { showRollForm = true; }}>
						🎲 Start a dice roll
					</button>
				{/if}
			{/if}

			<!-- ── Focus-player action bar ──────────────────────────────────── -->
			{#if isFocusPlayer}
				<div class="action-bar">
					{#if !sceneEnded}
						<!-- Step 1: scene is active — end it when ready -->
						<div class="action-step">
							<span class="action-label">Your turn as focus player</span>
							<button class="action-btn primary" onclick={onEndScene} disabled={actionBusy}>
								{actionBusy ? '…' : 'End Scene'}
							</button>
						</div>

					{:else if !actionTaken}
						<!-- Step 2: post-scene action — prepare a plan (PlanPanel above), refresh, or skip -->
						<div class="action-step">
							{#if hasPlansToResolve}
								<!-- A plan needs to be resolved before the focus player can act. -->
								<span class="action-label">Resolve the active plan above before acting.</span>
							{:else}
								<span class="action-label">
									Prepare a plan (above) or: refresh up to {maxRefresh} asset{maxRefresh === 1 ? '' : 's'}, or skip
								</span>
								{#if refreshable.length > 0}
									<div class="refresh-picker">
										{#each refreshable as asset (asset.id)}
											<label class="refresh-item" class:selected={selectedRefreshIDs.has(asset.id)}>
												<input
													type="checkbox"
													checked={selectedRefreshIDs.has(asset.id)}
													disabled={!selectedRefreshIDs.has(asset.id) && selectedRefreshIDs.size >= maxRefresh}
													onchange={() => toggleRefreshSelection(asset.id)}
												/>
												<span class="refresh-asset-name">{asset.name}</span>
												<span class="refresh-asset-type">{asset.asset_type}</span>
											</label>
										{/each}
									</div>
									<div class="action-buttons">
										<button
											class="action-btn primary"
											onclick={onRefreshAssets}
											disabled={actionBusy || selectedRefreshIDs.size === 0}
										>
											{actionBusy ? '…' : `Refresh ${selectedRefreshIDs.size > 0 ? selectedRefreshIDs.size : ''} Asset${selectedRefreshIDs.size === 1 ? '' : 's'}`}
										</button>
										<button class="action-btn secondary" onclick={onSkipRefresh} disabled={actionBusy}>
											Skip
										</button>
									</div>
								{:else}
									<p class="action-note">No leveraged assets to refresh.</p>
									<button class="action-btn secondary" onclick={onSkipRefresh} disabled={actionBusy}>
										Skip
									</button>
								{/if}
							{/if}
						</div>

					{:else}
						<!-- Step 3: pass focus (server auto-advances row when all plans on this row are resolved) -->
						<div class="action-step">
							<span class="action-label">Ready to move on</span>
							<button class="action-btn primary" onclick={onPassFocus} disabled={actionBusy}>
								{actionBusy ? '…' : 'Pass Focus'}
							</button>
						</div>
					{/if}
				</div>
			{:else if game.focus_player_id != null}
				<!-- Non-focus players see a quiet indicator -->
				<div class="action-bar waiting">
					<span class="action-label">
						Waiting for {players.find(p => p.id === game.focus_player_id)?.display_name ?? 'the focus player'}…
					</span>
				</div>
			{/if}
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

	/* ── Action bar ──────────────────────────────────────────────────────────── */

	.action-bar {
		flex-shrink: 0;
		padding: 0.6rem 0 0;
		border-top: 1px solid #3a3020;
		margin-top: 0.25rem;
	}

	.action-bar.waiting {
		border-color: #222;
	}

	.action-step {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.action-label {
		font-size: 0.78rem;
		color: #c8a96e;
		font-style: italic;
	}

	.action-bar.waiting .action-label {
		color: #666;
	}

	.action-buttons {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.action-btn {
		padding: 0.4rem 0.8rem;
		border-radius: 5px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.action-btn.primary {
		background: #c8a96e;
		color: #1a1a1a;
	}

	.action-btn.secondary {
		background: #333;
		color: #c8a96e;
		border: 1px solid #4a4030;
	}

	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.action-note {
		font-size: 0.82rem;
		color: #666;
		margin: 0;
	}

	/* Roll init */

	.roll-init-btn {
		background: none;
		color: #8a6a3a;
		font-size: 0.78rem;
		padding: 0.25rem 0;
		cursor: pointer;
		text-decoration: underline dotted;
		flex-shrink: 0;
	}

	.roll-init-btn:hover { color: #c8a96e; }

	.roll-start-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.5rem;
		border: 1px solid #4a3a20;
		border-radius: 5px;
		background: #1e1a10;
		flex-shrink: 0;
	}

	.roll-form-label {
		font-size: 0.82rem;
		color: #c8a96e;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.diff-input {
		width: 60px;
		padding: 0.2rem 0.4rem;
		background: #2a2a2a;
		border: 1px solid #555;
		border-radius: 4px;
		color: inherit;
		font-size: 0.9rem;
	}

	.roll-form-actions {
		display: flex;
		gap: 0.5rem;
	}

	/* Refresh asset picker */

	.refresh-picker {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		max-height: 120px;
		overflow-y: auto;
	}

	.refresh-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		background: #252525;
		cursor: pointer;
		font-size: 0.85rem;
		border: 1px solid transparent;
	}

	.refresh-item.selected {
		border-color: #c8a96e;
		background: #2e2510;
	}

	.refresh-item input[type="checkbox"] {
		accent-color: #c8a96e;
		width: 14px;
		height: 14px;
		cursor: pointer;
	}

	.refresh-asset-name {
		flex: 1;
		color: #e8e4d9;
	}

	.refresh-asset-type {
		font-size: 0.72rem;
		color: #777;
		text-transform: capitalize;
	}
</style>
