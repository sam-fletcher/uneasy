<!-- MainEventView.svelte
  Main event phase: public record sidebar + focus-player action bar.
  Owns its local UI state (summary form, refresh-asset selection).
  Chat now lives in the page-level ChatPanel; this view no longer owns posts.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS, REVEAL_EVENTS } from '$lib/ws';
	import { warDrawerOpen, activeWarCount, pendingWarCount } from '$lib/warDrawer';
	import MakeWarPanel from '$lib/components/plans/MakeWarPanel.svelte';
	import ClandestinelyLiaisePanel from '$lib/components/plans/ClandestinelyLiaisePanel.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import type { PlanContext } from '$lib/components/plans/types';
	import { activeDemandAgainst, demandWinnersFromPlan, parseResolutionData } from '$lib/components/plans/shared';
	import { parseLiaiseData } from '$lib/plans/resolutionData/liaise';
	import {
		refreshAssets,
		listWars,
		getReveal,
	} from '$lib/api';
	import type { Game, Player, Asset, Ranking, Law, Rumor, RecordRow, DiceRoll, DiceRollDie, VoteView, RollParticipant, BankedDie, Plan, PlanToken, Scene, ScenePeerView, SceneSetupDraft, PreparePlanDraft, WarStateResponse, SimultaneousReveal, RowState } from '$lib/api';
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
		/** Authoritative row-state from the server. Null briefly during the
		 * first snapshot fetch; treated as a "still loading" state by the
		 * render chain below. See model/row_state.go for the type. */
		rowState: RowState | null;
		playerNameMap: Map<number, string>;
		isFacilitator: boolean;
		/** Active (unresolved) dice roll, or null if none. */
		activeRoll: DiceRoll | null;
		activeRollDice: DiceRollDie[];
		activeRollVotes: VoteView[];
		activeRollParticipants: RollParticipant[];
		bankedDice: BankedDie[];
		/** All plans for this game — owned and fetched by the parent; read-only here. */
		plans: Plan[];
		/** Plan tokens (one per plan_type/player), owned by the parent. Forwarded
		 *  to PlanPanel to render the prep-grid pips for every viewer. */
		planTokens: PlanToken[];
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
		/**
		 * Ephemeral mirror of the focus player's in-flight scene-setup
		 * selections. Non-focus players render the setup form from this
		 * so they can see what's being filled in. Null until the focus
		 * player makes their first change (late joiners see a blank form).
		 */
		sceneSetupDraft?: SceneSetupDraft | null;
		/**
		 * Ephemeral mirror of the focus player's currently-highlighted plan
		 * card during the post-scene prep step. Forwarded to PlanPanel so
		 * non-focus viewers can see which card is being considered.
		 */
		preparePlanDraft?: PreparePlanDraft | null;
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
		rowState,
		playerNameMap,
		isFacilitator,
		activeRoll = $bindable(),
		activeRollDice = $bindable(),
		activeRollVotes = $bindable(),
		activeRollParticipants = $bindable(),
		bankedDice = $bindable(),
		plans,
		planTokens,
		onPlansChanged,
		activeScene = null,
		activeScenePeers = [],
		sceneSetupDraft = null,
		preparePlanDraft = null,
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
	// Waiting-on derivation. Renders the WaitingOnBar at the top of the
	// page based on the server-authoritative RowState kind. The "waitees"
	// (who the game is blocked on) are usually the focus player, with two
	// exceptions: Make War's delay reveal blocks on the pending submitters,
	// and the row-advance gates block on the cost-payers / claimants.
	// An unresolved dice roll overrides the row-state waitees: the table is
	// blocked on the roll's own stage (whoever still owes a vote or a Ready),
	// not on the plan's focus player.
	function rollWaitingOn(roll: DiceRoll): WaitingOnState {
		switch (roll.stage) {
			case 'decide_vote':
				return {
					waitees: [{ kind: 'player', playerID: roll.actor_id }],
					stepLabel: 'Dice roll — call a vote?',
				};
			case 'voting': {
				const voted = new Set(activeRollVotes.map(v => v.player_id));
				return {
					waitees: players
						.filter(p => !voted.has(p.id))
						.map(p => ({ kind: 'player', playerID: p.id })),
					stepLabel: 'Dice roll — difficulty vote',
				};
			}
			case 'leverage':
				return {
					waitees: activeRollParticipants
						.filter(p => !p.is_ready)
						.map(p => ({ kind: 'player', playerID: p.player_id })),
					stepLabel: 'Dice roll — leverage & ready',
				};
			default:
				return { waitees: [] };
		}
	}

	const mainEventWaitingOn = $derived.by<WaitingOnState>(() => {
		if (activeRoll != null && activeRoll.outcome == null) {
			return rollWaitingOn(activeRoll);
		}

		const focusWaitee: Waitee[] = game.focus_player_id != null
			? [{ kind: 'player', playerID: game.focus_player_id }]
			: [];

		switch (rowState?.kind) {
			case 'plan_resolving':
				return { waitees: focusWaitee, stepLabel: 'Resolving plan' };
			case 'plan_pending':
				// Auto-kicked off by the server; this kind only surfaces
				// briefly mid-transition, or in the recovery path if
				// OnResolve failed. Same copy as plan_resolving — observers
				// don't need to distinguish.
				return { waitees: focusWaitee, stepLabel: 'Resolving plan' };
			case 'await_demand_counter': {
				// Sub-phase of plan_resolving: a marred Make Demands roll
				// is waiting on the demand target's decision to counter
				// or waive. Acting player is typically not the focus player.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Make Demands — awaiting counter' };
			}
			case 'await_demand_draft_pick': {
				// Made Make Demands: demander and target-plan preparer
				// alternate the four-pick draft. Half the picks block on
				// the non-focus player.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Make Demands — draft pick' };
			}
			case 'await_festivity_guest_turn': {
				// Host Festivity socializing phase: next guest (by ascending
				// esteem; host last) owes a roll/opt-out/choice.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Host Festivity — guest turn' };
			}
			case 'await_festivity_challenge_response': {
				// Open duel challenge inside a festivity; all other actions
				// pause until the target accepts or declines.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Host Festivity — challenge response' };
			}
			case 'await_duel_staking': {
				// Propose Duel setup/staking — both duellists simultaneously
				// submit stake counts then specific assets. List both as
				// waitees; one may have already submitted but the bar can't
				// tell that without fetching duel stakes.
				const duelPlan = plans.find(p => p.id === rowState.plan_id);
				const ids: number[] = [];
				if (duelPlan?.preparer_id != null) ids.push(duelPlan.preparer_id);
				if (duelPlan?.target_player_id != null) ids.push(duelPlan.target_player_id);
				return {
					waitees: ids.map(id => ({ kind: 'player', playerID: id })),
					stepLabel: 'Propose Duel — staking',
				};
			}
			case 'await_duel_bout': {
				// Propose Duel bouts phase — either the declarer (initiative
				// holder) or the responder owes the next action.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Propose Duel — bout' };
			}
			case 'await_take_consent': {
				// Spread Rumors take-asset — the victim must agree or disagree
				// before anything else happens. ActingPlayerID names them.
				const actor = rowState.acting_player_id;
				const waitees: Waitee[] = actor != null
					? [{ kind: 'player', playerID: actor }]
					: [];
				return { waitees, stepLabel: 'Spread Rumors — consent to take asset' };
			}
			case 'await_delay_reveal': {
				const planType = delayRevealPlan?.plan_type;
				const label =
					planType === 'make_war' ? 'Make War — delay reveal'
					: planType === 'clandestinely_liaise' ? 'Clandestinely Liaise — delay reveal'
					: 'Delay reveal';
				return {
					waitees: delayRevealPendingSubmitterIDs.map(id => ({ kind: 'player', playerID: id })),
					stepLabel: label,
				};
			}
			case 'await_battle_cost':
			case 'await_surrender_claim': {
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
			case 'scene_active':
				return { waitees: focusWaitee, stepLabel: 'Scene' };
			case 'scene_setting':
				return { waitees: focusWaitee, stepLabel: 'Set the scene' };
			case 'post_scene_action': {
				const subtitle = `or refresh ${maxRefresh} asset${maxRefresh === 1 ? '' : 's'}`;
				return { waitees: focusWaitee, stepLabel: 'Prepare a plan', stepSubtitle: subtitle };
			}
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

	// Refresh-assets sub-step: which leveraged assets the focus player has
	// selected. Keyed on focus_player_id (not currentPlayerID) so the
	// derived `maxRefresh` count is meaningful for every observer — the
	// picker itself is still gated behind `isFocusPlayer` below.
	let refreshable = $derived(assets.filter(a => a.owner_id === game.focus_player_id && a.is_leveraged && !a.is_destroyed));
	let selectedRefreshIDs = $state<Set<number>>(new Set());
	// Refresh cap: smaller of the current row number (per rules) and how many
	// leveraged assets the focus player actually has.
	let maxRefresh = $derived(Math.min(game.current_row, refreshable.length));
	// Button label: how many assets the click would refresh right now.
	let refreshButtonCount = $derived(selectedRefreshIDs.size > 0 ? selectedRefreshIDs.size : maxRefresh);

	// Reset selections when assets or step changes.
	$effect(() => {
		if (rowState?.kind !== 'post_scene_action') selectedRefreshIDs = new Set();
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

	/** True when an in-flight roll hasn't resolved yet. */
	const rollActive = $derived(activeRoll != null && activeRoll.outcome == null);

	// ── Delay-reveal play area takeover ──────────────────────────────────────
	// While kind='await_delay_reveal', a single plan (Make War or
	// Clandestinely Liaise) is blocking the row until all participants
	// submit a hidden die. The play area shows the appropriate panel for
	// every player; inputs are gated per-panel by participant identity.
	const delayRevealPlan = $derived(
		rowState?.kind === 'await_delay_reveal' && rowState.plan_id != null
			? plans.find(p => p.id === rowState.plan_id) ?? null
			: null,
	);
	const delayRevealActive = $derived(delayRevealPlan != null);

	// Extract the delay reveal ID from the right slot in resolution_data
	// depending on the plan type — the two plans store it under different
	// keys but otherwise share the simultaneous-reveal contract.
	const delayRevealID = $derived.by<number | null>(() => {
		if (!delayRevealPlan) return null;
		if (delayRevealPlan.plan_type === 'make_war') {
			return parseResolutionData(delayRevealPlan).make_war?.delay_reveal_id ?? null;
		}
		if (delayRevealPlan.plan_type === 'clandestinely_liaise') {
			return parseLiaiseData(delayRevealPlan).delay_reveal_id ?? null;
		}
		return null;
	});
	let delayReveal = $state<SimultaneousReveal | null>(null);
	async function refreshDelayReveal(revealID: number) {
		try { delayReveal = await getReveal(revealID); }
		catch { delayReveal = null; }
	}
	$effect(() => {
		const id = delayRevealID;
		if (id == null) { delayReveal = null; return; }
		void refreshDelayReveal(id);
	});
	useWindowEvents(REVEAL_EVENTS, (e) => {
		const id = delayRevealID;
		const detail = (e as CustomEvent<{ reveal_id: number }>).detail;
		if (id != null && detail?.reveal_id === id) void refreshDelayReveal(id);
	});
	// Participants whose reveal entry is still face=null.
	const delayRevealPendingSubmitterIDs = $derived.by<number[]>(() => {
		if (!delayReveal || delayReveal.is_complete) return [];
		// revealed_at (not face) marks who has submitted — faces stay hidden
		// until the reveal completes, so face is null for everyone before then.
		return delayReveal.entries
			.filter(e => e.revealed_at == null)
			.map(e => e.player_id);
	});


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
		activeRollParticipants = [];
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

	// ── War drawer (header button) ───────────────────────────────────────────
	// Once a Make War plan is placed on the public record, its panel hides
	// from the play area; the player accesses war state via the header
	// "War" button. Plans still in delay-reveal stay inline as the takeover
	// above. The button colours itself off two counts:
	//   • pending — plan still 'pending' on a future row (war planned but
	//     not started). Drives the yellow tint.
	//   • active — plan resolved, war row still 'active' (war ongoing).
	//     Drives the red tint. Both non-zero → orange.
	const drawerWarPlans = $derived(
		plans.filter(p => {
			if (p.plan_type !== 'make_war') return false;
			if (p.status === 'cancelled') return false;
			if (p.row_number == null) return false; // still in delay reveal
			const w = wars.find(x => x.origin_plan_id === p.id);
			if (w && w.status === 'ended' && p.status === 'resolved') return false;
			return true;
		}),
	);
	const drawerPendingCount = $derived(
		drawerWarPlans.filter(p => p.status === 'pending').length,
	);
	const drawerActiveCount = $derived(
		drawerWarPlans.length - drawerPendingCount,
	);
	$effect(() => { pendingWarCount.set(drawerPendingCount); });
	$effect(() => { activeWarCount.set(drawerActiveCount); });

	const drawerCtx = $derived<PlanContext>({
		gameID: game.id,
		currentRow: game.current_row,
		plans, assets, players, rankings,
		currentPlayerID,
		isFocusPlayer,
		rollActive,
		rollOutcome,
		activeRoll,
		onRollCreated: onPlanRollCreated,
		onPlansChanged,
		onPlanPrepared,
		// Drawer ctx only renders resolve-mode panels (delay reveal, active
		// war drawer); prep-draft mirroring doesn't apply.
		readOnly: false,
		prepDraft: null,
		emitPrepDraft: () => {},
	});
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
			<!-- ── Delay-reveal play-area takeover ──────────────────────────
				While a Make War or Clandestinely Liaise plan is awaiting its
				simultaneous reveal, every player sees the same panel for the
				blocking plan. Inputs inside each panel are gated by participant
				identity; non-participants watch.
			-->
			{#if delayRevealActive && delayRevealPlan}
				{#if delayRevealPlan.plan_type === 'make_war'}
					<MakeWarPanel ctx={drawerCtx} plan={delayRevealPlan} mode="delayReveal" />
				{:else if delayRevealPlan.plan_type === 'clandestinely_liaise'}
					<ClandestinelyLiaisePanel ctx={drawerCtx} plan={delayRevealPlan} mode="delayReveal" />
				{/if}
			{:else if activeScene}
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
			{:else if rowState?.kind === 'scene_setting' && game.focus_player_id != null}
				<SceneSetupForm
					gameID={game.id}
					{assets}
					{players}
					focusPlayerID={game.focus_player_id}
					prompt={followOnPromptForRow(plans, game.current_row)}
					onSceneStarted={onSceneRefresh}
					readOnly={!isFocusPlayer}
					draft={sceneSetupDraft}
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
				{planTokens}
				{assets}
				{players}
				{rankings}
				{currentPlayerID}
				{isFocusPlayer}
				prepEnabled={rowState?.kind === 'post_scene_action'}
				suppressPrep={delayRevealActive}
				{rollActive}
				{rollOutcome}
				{activeRoll}
				onRollCreated={onPlanRollCreated}
				{onPlansChanged}
				{onPlanPrepared}
				{preparePlanDraft}
			/>

			<!-- ── Dice roll panel ───────────────────────────────────────────── -->
			{#if activeRoll}
				<DiceRollPanel
					bind:roll={activeRoll}
					bind:dice={activeRollDice}
					bind:votes={activeRollVotes}
					bind:participants={activeRollParticipants}
					bind:bankedDice
					{assets}
					{currentPlayerID}
					{players}
					{playerNameMap}
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
			{#if isFocusPlayer && rowState?.kind === 'post_scene_action' && !delayRevealActive}
				<div class="action-bar">
					<div class="action-step">
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
					</div>
				</div>
			{/if}
		</div>

	</div>
</div>

<RetinueSheet open={$warDrawerOpen} onClose={() => warDrawerOpen.set(false)}>
	<div class="war-sheet">
		<h3>Active Wars ({drawerWarPlans.length})</h3>
		{#if drawerWarPlans.length === 0}
			<p class="muted">No active wars.</p>
		{:else}
			{#each drawerWarPlans as p (p.id)}
				<MakeWarPanel ctx={drawerCtx} plan={p} mode="resolve" />
			{/each}
		{/if}
	</div>
</RetinueSheet>

<style>
	.war-sheet h3 { margin: 0 0 0.5rem; }
	.main-event-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		min-height: 0;
	}

	.local-error {
		color: var(--color-danger);
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
		background: var(--color-border-warm);
	}
	.or-label {
		font-size: 0.78rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	/* ── Action bar ──────────────────────────────────────────────────────────── */

	.action-bar {
		flex-shrink: 0;
		padding: 0.6rem 0 0;
		border-top: 1px solid var(--color-border-warm);
		margin-top: 0.25rem;
	}

	.action-step {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.action-buttons {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		justify-content: center;
	}

	.action-btn {
		min-height: 44px;
		padding: 0.4rem 1rem;
		border-radius: 5px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.action-btn.primary {
		background: var(--color-accent);
		color: var(--color-bg);
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
		min-height: 44px;
		padding: 0.4rem 0.6rem;
		border-radius: 4px;
		background: var(--color-surface);
		cursor: pointer;
		font-size: 0.85rem;
		border: 1px solid transparent;
	}

	.refresh-item.selected {
		border-color: var(--color-accent);
		background: #2e2510;
	}

	.refresh-item input[type="checkbox"] {
		accent-color: var(--color-accent);
		width: 18px;
		height: 18px;
		cursor: pointer;
	}

	.refresh-asset-name {
		flex: 1;
		color: var(--color-text);
	}

	.refresh-asset-type {
		font-size: 0.72rem;
		color: var(--color-text-faint);
		text-transform: capitalize;
	}
</style>
