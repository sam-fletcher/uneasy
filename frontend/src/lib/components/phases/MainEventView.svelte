<!-- MainEventView.svelte
  Main event phase: public record sidebar + focus-player action bar.
  Owns its local UI state (summary form, refresh-asset selection).
  Chat now lives in the page-level ChatPanel; this view no longer owns posts.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS } from '$lib/ws';
	import { activeDemandAgainst, demandWinnersFromPlan, plansPendingOnRow } from '$lib/components/plans/shared';
	import {
		refreshAssets,
		listWars,
	} from '$lib/api';
	import type { Game, Player, Asset, Ranking, Law, Rumor, RecordRow, DiceRoll, DiceRollDie, DifficultyVote, Plan, Scene, ScenePeerView, WarStateResponse } from '$lib/api';
	import DiceRollPanel from '$lib/components/DiceRollPanel.svelte';
	import PlanPanel from '$lib/components/PlanPanel.svelte';
	import SceneSetupForm from '$lib/components/SceneSetupForm.svelte';
	import SceneDetailsPanel from '$lib/components/SceneDetailsPanel.svelte';
	import { followOnPromptForRow } from '$lib/scenePrompts';
	import type { WaitingOnState, Waitee } from '$lib/components/WaitingOnBar.svelte';

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
		/** Bound by the parent; this view publishes its waiting-on derivation here. */
		waitingOn: WaitingOnState;
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
		waitingOn = $bindable(),
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
	// Waiting-on derivation. Resolves who the game is blocked on, plus a
	// step label and optional subtitle. Priority order:
	//   1. Plan resolving           → focus player, 'Resolving plan'
	//   2. Plan pending on row      → focus player, 'Plan to resolve'
	//   3. War-cost row-advance block → union of cost-payers and claimants
	//   4. Active scene             → focus player, 'Scene'
	//   5. Pre-scene                → focus player, 'Set the scene'
	//   6. Post-scene action        → focus player, 'Prepare a plan' (+ refresh subtitle)
	const mainEventWaitingOn = $derived.by<WaitingOnState>(() => {
		const focusWaitee: Waitee[] = game.focus_player_id != null
			? [{ kind: 'player', playerID: game.focus_player_id }]
			: [];

		if (plans.some(p => p.status === 'resolving')) {
			return { waitees: focusWaitee, stepLabel: 'Resolving plan' };
		}
		if (plansPendingOnRow(plans, game.current_row).length > 0) {
			return { waitees: focusWaitee, stepLabel: 'Plan to resolve' };
		}
		if (blockingCostPayers.length > 0 || blockingClaimants.length > 0) {
			const ids = new Set<number>([...blockingCostPayers, ...blockingClaimants]);
			const parts: string[] = [];
			if (blockingCostPayers.length > 0) parts.push('cost of battle');
			if (blockingClaimants.length > 0) parts.push('surrender-asset claims');
			return {
				waitees: [...ids].map(id => ({ kind: 'player', playerID: id })),
				stepLabel: 'Row advance blocked',
				stepSubtitle: parts.join(' · '),
			};
		}
		if (activeScene) {
			return { waitees: focusWaitee, stepLabel: 'Scene' };
		}
		if (game.focus_player_id != null && !sceneEnded) {
			return { waitees: focusWaitee, stepLabel: 'Set the scene' };
		}
		if (game.focus_player_id != null && sceneEnded) {
			const subtitle = isFocusPlayer
				? `or refresh ${maxRefresh} asset${maxRefresh === 1 ? '' : 's'}`
				: undefined;
			return { waitees: focusWaitee, stepLabel: 'Prepare a plan', stepSubtitle: subtitle };
		}
		return { waitees: [] };
	});
	$effect(() => { waitingOn = mainEventWaitingOn; });


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
			// Assets update via asset.refreshed; the server auto-passes the
			// focus marker (focus.changed) so no local state change is needed.
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not refresh assets.';
		} finally {
			actionBusy = false;
		}
	}

	// ── Plan state ────────────────────────────────────────────────────────────

	/** True when there is an active resolving plan or a pending plan on the current row. */
	const hasPlansToResolve = $derived(
		plans.some(p => p.status === 'resolving') ||
		plansPendingOnRow(plans, game.current_row).length > 0
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
	 * Called by PlanPanel when the focus player prepares a plan. The server
	 * auto-passes the focus marker after preparation succeeds; the resulting
	 * focus.changed event drives the UI transition. We only need to refresh
	 * the parent's plan list here.
	 */
	function onPlanPrepared() {
		onPlansChanged();
	}

</script>

<div class="main-event-view">
	{#if error}
		<p class="local-error">{error}</p>
	{/if}

	<!-- Play surface — single column. PublicRecord lives at the page level
	     now (sibling to ChatPanel) so it can sit in its own grid column on
	     wide desktop layouts. -->
	<div class="play-surface">
		<div class="scene-panel">
			<!-- ── Scene structure ────────────────────────────────────────────
				Two states:
				  1. Active scene  → SceneDetailsPanel (everyone; controls vary)
				  2. No scene, focus player, no pending plans → SceneSetupForm
				While a plan is resolving / pending, neither panel renders —
				PlanPanel takes over. The page-level WaitingOnBar carries the
				"who/what we're waiting on" copy.
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
			{:else if !hasPlansToResolve && !sceneEnded && game.focus_player_id != null}
				<SceneSetupForm
					gameID={game.id}
					{assets}
					{players}
					focusPlayerID={game.focus_player_id}
					prompt={followOnPromptForRow(plans, game.current_row)}
					onSceneStarted={onSceneRefresh}
					readOnly={!isFocusPlayer}
				/>
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
				prepEnabled={isFocusPlayer && sceneEnded}
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
				End Scene lives in SceneDetailsPanel above; preparing a plan
				lives in PlanPanel above. This bar only renders the refresh
				alternative (or a wait/resolve hint). Plan prep and refresh
				both auto-pass the focus marker server-side, so there is no
				explicit "Pass Focus" step here.
			-->
			{#if isFocusPlayer && sceneEnded}
				<div class="action-bar">
					<div class="action-step">
						{#if hasPlansToResolve}
							<!-- A plan needs to be resolved before the focus player can act. -->
							<span class="action-label">Resolve the active plan above before acting.</span>
						{:else}
							<!-- OR divider visually separates the plan grid (above)
							     from the refresh-assets alternative (below). -->
							<div class="or-divider" aria-hidden="true">
								<span class="or-line"></span>
								<span class="or-label">OR</span>
								<span class="or-line"></span>
							</div>
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
	/* Single column. PublicRecord lives at the page level. */

	.play-surface {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-height: 0;
		overflow: hidden;
	}

	/* ── Scene panel ─────────────────────────────────────────────────────────── */

	.scene-panel {
		display: flex;
		flex-direction: column;
		padding: 0.75rem 0.5rem 0;
		/* Scrollable so a tall PlanPanel doesn't push the action-bar
		   (Refresh / Pass Focus buttons) out of reach. The parent .table-body
		   already reserves padding-bottom for the mobile chat strip, so the
		   bottom of this scroll viewport sits above the strip. */
		overflow-y: auto;
		min-height: 0;
		flex: 1;
	}

	@media (max-width: 600px) {
		.scene-panel { padding: 0.5rem 0 0; }
	}

	/* "OR" boundary divider between the plan grid and the refresh action.
	 * Mirrors the chat-panel boundary marker style so both contexts read
	 * the same visual language. */
	.or-divider {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.6rem 0 0.5rem;
	}
	.or-line {
		flex: 1;
		height: 1px;
		background: #3a3020;
	}
	.or-label {
		font-size: 0.78rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	/* ── Action bar ──────────────────────────────────────────────────────────── */

	.action-bar {
		flex-shrink: 0;
		padding: 0.6rem 0 0;
		border-top: 1px solid #3a3020;
		margin-top: 0.25rem;
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

	.action-buttons {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		justify-content: center;
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
