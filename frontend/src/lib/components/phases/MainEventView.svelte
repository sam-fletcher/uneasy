<!-- MainEventView.svelte
  Main event phase: public record sidebar + focus-player action bar.
  Owns its local UI state (summary form, refresh-asset selection).
  Chat now lives in the page-level ChatPanel; this view no longer owns posts.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS } from '$lib/ws';
	import { activeDemandAgainst, demandWinnersFromPlan } from '$lib/components/plans/shared';
	import {
		refreshAssets, passFocus,
		listWars,
	} from '$lib/api';
	import type { Game, Player, Asset, Ranking, Law, Rumor, RecordRow, DiceRoll, DiceRollDie, DifficultyVote, Plan, Scene, ScenePeerView, WarStateResponse } from '$lib/api';
	import PublicRecord from '$lib/components/PublicRecord.svelte';
	import DiceRollPanel from '$lib/components/DiceRollPanel.svelte';
	import PlanPanel from '$lib/components/PlanPanel.svelte';
	import SceneSetupForm from '$lib/components/SceneSetupForm.svelte';
	import SceneDetailsPanel from '$lib/components/SceneDetailsPanel.svelte';
	import { followOnPromptForRow } from '$lib/scenePrompts';

	interface Props {
		game: Game;
		players: Player[];
		rankings: Ranking[];
		laws: Law[];
		rumors: Rumor[];
		assets: Asset[];
		currentPlayerID: number | null;
		recordRows: RecordRow[];
		sceneEnded: boolean;
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
		/**
		 * Active scene + present peers, owned and refreshed by the parent.
		 * Null between scenes; null while not in main_event.
		 */
		activeScene?: Scene | null;
		activeScenePeers?: ScenePeerView[];
		/** Called after a scene mutation so the parent can re-fetch. */
		onSceneRefresh?: () => void;
	}

	let {
		game,
		players,
		rankings,
		laws,
		rumors,
		assets,
		currentPlayerID,
		recordRows = $bindable(),
		sceneEnded = $bindable(),
		playerNameMap,
		isFacilitator,
		activeRoll = $bindable(),
		activeRollDice = $bindable(),
		activeRollVotes = $bindable(),
		voteOpen = $bindable(),
		plans,
		onPlansChanged,
		activeScene = null,
		activeScenePeers = [],
		onSceneRefresh = () => {},
	}: Props = $props();

	// ── War cost-of-battle gate ───────────────────────────────────────────────
	// Track active wars game-wide so the row header can warn when row advance
	// is blocked on unpaid battle costs or open surrender claims (the server
	// returns a 409/`advance_blocked` field; we surface the same up-front).
	let wars = $state<WarStateResponse[]>([]);
	async function refreshWars() {
		try {
			const data = await listWars(game.id);
			wars = data.wars;
		} catch { wars = []; }
	}
	function onWarEvent() { refreshWars(); }
	useWindowEvents(WAR_EVENTS, onWarEvent);
	onMount(() => { if (game.phase === 'main_event') refreshWars(); });
	// Refresh when the row changes too — outstanding-cost computation is per-row.
	$effect(() => {
		if (game.phase === 'main_event') {
			void game.current_row;
			refreshWars();
		}
	});

	const blockingCostPayers = $derived.by<number[]>(() => {
		const ids = new Set<number>();
		for (const w of wars) for (const c of w.outstanding_costs) ids.add(c.payer_id);
		return [...ids];
	});
	const blockingClaimants = $derived.by<number[]>(() => {
		const ids = new Set<number>();
		for (const w of wars) for (const c of w.open_claims) ids.add(c.claimant_id);
		return [...ids];
	});
	const playerName = (id: number) =>
		players.find(p => p.id === id)?.display_name ?? `Player ${id}`;

	const focusPlayerName = $derived(
		game.focus_player_id
			? players.find(p => p.id === game.focus_player_id)?.display_name ?? '?'
			: null
	);

	// Local error string for non-chat actions (refresh, pass focus).
	// Chat errors live inside ChatPanel now.
	let error = $state('');

	// ── Focus-player action bar ───────────────────────────────────────────────

	const isFocusPlayer = $derived(
		currentPlayerID != null && game.focus_player_id === currentPlayerID
	);

	// Refresh-assets sub-step: which leveraged assets the player has selected.
	let refreshable = $derived(assets.filter(a => a.owner_id === currentPlayerID && a.is_leveraged && !a.is_destroyed));
	let selectedRefreshIDs = $state<Set<number>>(new Set());
	// Refresh cap: smaller of the current row number (per rules) and how many
	// leveraged assets the focus player actually has.
	let maxRefresh = $derived(Math.min(game.current_row, refreshable.length));
	// Button label: how many assets the click would refresh right now.
	let refreshButtonCount = $derived(selectedRefreshIDs.size > 0 ? selectedRefreshIDs.size : maxRefresh);

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

	// Block the actor's own leverage if a Make Demands `control_leverage`
	// winner has authority over this roll's plan. Backend would 403 anyway;
	// hiding the button avoids confusing UI.
	const actorLeverageBlocked = $derived.by(() => {
		const planID = activeRoll?.plan_id;
		if (planID == null) return false;
		const targetPlan = plans.find(p => p.id === planID);
		if (!targetPlan) return false;
		const demand = activeDemandAgainst(targetPlan, plans);
		if (!demand) return false;
		return demandWinnersFromPlan(demand).control_leverage != null;
	});

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

</script>

<div class="main-event-view">
	{#if error}
		<p class="local-error">{error}</p>
	{/if}

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

			{#if blockingCostPayers.length > 0 || blockingClaimants.length > 0}
				<div class="war-block-banner">
					<strong>Row advance blocked by war costs.</strong>
					{#if blockingCostPayers.length > 0}
						Waiting on cost of battle from:
						{blockingCostPayers.map(playerName).join(', ')}.
					{/if}
					{#if blockingClaimants.length > 0}
						Waiting on surrender-asset claims from:
						{blockingClaimants.map(playerName).join(', ')}.
					{/if}
				</div>
			{/if}

			<!-- ── Scene structure ────────────────────────────────────────────
				Three states:
				  1. Active scene  → SceneDetailsPanel (everyone; controls vary)
				  2. No scene, focus player, no pending plans → SceneSetupForm
				  3. No scene, anyone else                    → quiet waiting hint
				While a plan is resolving / pending, neither panel renders —
				PlanPanel takes over.
			-->
			{#if activeScene}
				<SceneDetailsPanel
					gameID={game.id}
					scene={activeScene}
					peers={activeScenePeers}
					{assets}
					{players}
					{currentPlayerID}
					{isFocusPlayer}
					onSceneEnded={onSceneRefresh}
					{rollActive}
					onRollCreated={onPlanRollCreated}
				/>
			{:else if !hasPlansToResolve && isFocusPlayer && !sceneEnded}
				<SceneSetupForm
					gameID={game.id}
					{assets}
					{players}
					focusPlayerID={currentPlayerID!}
					prompt={followOnPromptForRow(plans, game.current_row)}
					onSceneStarted={onSceneRefresh}
				/>
			{:else if !hasPlansToResolve && !sceneEnded}
				<p class="action-note">
					Waiting for {focusPlayerName ?? 'the focus player'} to set the scene…
				</p>
			{/if}

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
				{rankings}
				{currentPlayerID}
				{isFocusPlayer}
				prepEnabled={isFocusPlayer && sceneEnded && !actionTaken}
				{rollActive}
				{rollOutcome}
				{activeRoll}
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
					{actorLeverageBlocked}
				/>
			{/if}

			<!-- ── Focus-player action bar ──────────────────────────────────── -->
			<!--
				Step 1 (End Scene) used to live here; it now lives in
				SceneDetailsPanel above. The action bar only handles
				post-scene actions (refresh / skip / prepare plan) and
				passing focus.
			-->
			{#if isFocusPlayer && sceneEnded}
				<div class="action-bar">
					{#if !actionTaken}
						<!-- Step 2: post-scene action — prepare a plan (PlanPanel above), refresh, or skip -->
						<div class="action-step">
							{#if hasPlansToResolve}
								<!-- A plan needs to be resolved before the focus player can act. -->
								<span class="action-label">Resolve the active plan above before acting.</span>
							{:else}
								<span class="action-label">
									Prepare a plan (above) or refresh up to {maxRefresh} asset{maxRefresh === 1 ? '' : 's'}
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
								{/if}
								<div class="action-buttons">
									<button
										class="action-btn primary"
										onclick={onRefreshAssets}
										disabled={actionBusy || selectedRefreshIDs.size === 0}
									>
										{actionBusy ? '…' : `Refresh ${refreshButtonCount} Asset${refreshButtonCount === 1 ? '' : 's'}`}
									</button>
								</div>
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
			{:else if !isFocusPlayer && sceneEnded && game.focus_player_id != null}
				<!-- Non-focus players see a quiet indicator post-scene only.
					 Pre-scene waiting copy lives near SceneSetupForm above. -->
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

	.war-block-banner {
		background: #3a1f1f;
		border: 1px solid #6a3030;
		color: #e7c5c5;
		padding: 0.4rem 0.6rem;
		border-radius: 4px;
		font-size: 0.85rem;
		margin: 0.3rem 0;
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

	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.action-note {
		font-size: 0.82rem;
		color: #666;
		margin: 0;
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
