<!-- TargetPlanDemandOverlay.svelte
  Cross-cutting Stage 4 UI rendered at the top of every per-plan resolve view
  (except Make Demands itself and Make War, which can't be a demand target).

  When a resolved+made Make Demands plan targets the rendered plan, this
  component:
   - Surfaces the four draft winners as a banner.
   - Renders the keep_or_change_target retarget picker for that winner.
   - Renders the control_leverage picker for that winner.
   - Notes the keep_assets winner (display-only — backend routing).
   - Exposes performStepsWinnerID and controlLeverageWinnerID via bindable
     props so the parent panel can gate its make/mar / leverage UI.

  Self-contained: discovers the demand from the plans list and decodes
  winners from the demand's resolution_data — no extra fetches.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { demandLeverage, demandRetarget, type Plan, type Player, type Asset } from '$lib/api';
	import { playerName, activeDemandAgainst, demandWinnersFromPlan } from '../shared';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import FormField from '../FormField.svelte';

	interface Props {
		/** The plan being targeted (the resolve panel renders this). */
		plan: Plan;
		plans: Plan[];
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		/** Bindable: parent uses this to gate "amResponsibleForChoice" on the
		 *  standard make/mar picker. null when no perform_steps winner exists. */
		performStepsWinnerID?: number | null;
		/** Bindable: parent uses this to hide the target preparer's own
		 *  leverage button (backend would 403). null when no winner exists. */
		controlLeverageWinnerID?: number | null;
	}

	let {
		plan, plans, players, assets, currentPlayerID,
		performStepsWinnerID = $bindable(null),
		controlLeverageWinnerID = $bindable(null),
	}: Props = $props();

	const demand = $derived(activeDemandAgainst(plan, plans));
	const winners = $derived(demand ? demandWinnersFromPlan(demand) : {});

	// Sync bindables out to the parent.
	$effect(() => { performStepsWinnerID = winners.perform_steps ?? null; });
	$effect(() => { controlLeverageWinnerID = winners.control_leverage ?? null; });

	const amKeepOrChangeTargetWinner = $derived(
		currentPlayerID != null && winners.keep_or_change_target === currentPlayerID,
	);
	const amControlLeverageWinner = $derived(
		currentPlayerID != null && winners.control_leverage === currentPlayerID,
	);
	const amPerformStepsWinner = $derived(
		currentPlayerID != null && winners.perform_steps === currentPlayerID,
	);

	const draftComplete = $derived(
		demand != null
		&& winners.control_leverage != null
		&& winners.keep_or_change_target != null
		&& winners.keep_assets != null
		&& winners.perform_steps != null,
	);

	// ── Retarget form ─────────────────────────────────────────────────────────

	let retargetPlayerID = $state<number | null>(null);
	let retargetAssetID = $state<number | null>(null);
	let retargetBusy = $state(false);
	let retargetError = $state('');

	// Initialize retarget pickers to current values when the plan changes.
	let lastRetargetPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastRetargetPlanID) {
			lastRetargetPlanID = plan.id;
			retargetPlayerID = plan.target_player_id;
			retargetAssetID = plan.target_asset_id;
		}
	});

	// Filter the asset picker by the currently-chosen target player. If no
	// player is chosen the asset list is hidden (re-aiming to a null player
	// means clearing the target entirely).
	const retargetCandidateAssets = $derived(
		retargetPlayerID == null
			? []
			: assets.filter(a => a.owner_id === retargetPlayerID && !a.is_destroyed),
	);

	function selectRetargetPlayer(p: Player) {
		if (retargetPlayerID === p.id) {
			retargetPlayerID = null;
			retargetAssetID = null;
		} else {
			retargetPlayerID = p.id;
			// Clear asset on player change unless it already belongs to them.
			if (retargetAssetID != null) {
				const a = assets.find(x => x.id === retargetAssetID);
				if (!a || a.owner_id !== p.id) retargetAssetID = null;
			}
		}
	}

	async function submitRetarget() {
		if (retargetBusy) return;
		retargetBusy = true; retargetError = '';
		try {
			await demandRetarget(plan.id, {
				target_player_id: retargetPlayerID,
				target_asset_id: retargetAssetID,
			});
		} catch (e) {
			retargetError = e instanceof Error ? e.message : 'Could not retarget plan.';
		} finally { retargetBusy = false; }
	}

	// ── Leverage picker ───────────────────────────────────────────────────────
	// The control_leverage winner picks one or more of the target preparer's
	// non-destroyed assets to leverage onto the target plan's roll. The
	// backend silently skips assets already on the roll, so we don't need to
	// fetch the roll to filter — show all of the target preparer's intact
	// assets. Unsupported (no open roll, plan resolved, etc.) → backend 409.

	let selectedLeverageIDs = $state<number[]>([]);
	let leverageBusy = $state(false);
	let leverageError = $state('');

	const leverageableTargetAssets = $derived(
		assets.filter(a => a.owner_id === plan.preparer_id && !a.is_destroyed),
	);

	async function submitLeverage() {
		if (leverageBusy || selectedLeverageIDs.length === 0) return;
		leverageBusy = true; leverageError = '';
		try {
			await demandLeverage(plan.id, selectedLeverageIDs);
			selectedLeverageIDs = [];
		} catch (e) {
			leverageError = e instanceof Error ? e.message : 'Could not leverage assets.';
		} finally { leverageBusy = false; }
	}
</script>

{#if demand && draftComplete}
	<div class="demand-banner">
		<p class="demand-banner-header">
			Demand in effect from
			{playerName(players, demand.preparer_id)}
		</p>
		<ul class="demand-winners">
			{#if winners.perform_steps != null && winners.perform_steps !== plan.preparer_id}
				<li>
					{#if amPerformStepsWinner}
						You will submit this plan's make/mar choices
						in {playerName(players, plan.preparer_id)}'s place.
					{:else}
						{playerName(players, winners.perform_steps)}
						will submit this plan's make/mar choices in
						{playerName(players, plan.preparer_id)}'s place.
					{/if}
				</li>
			{/if}
			{#if winners.keep_assets != null && winners.keep_assets !== plan.preparer_id}
				<li>
					Any assets this plan would have given
					{playerName(players, plan.preparer_id)}
					go to {playerName(players, winners.keep_assets)} instead.
				</li>
			{/if}
			{#if winners.control_leverage != null && winners.control_leverage !== plan.preparer_id}
				<li>
					{playerName(players, winners.control_leverage)}
					controls leverage of {playerName(players, plan.preparer_id)}'s
					assets on this plan's roll.
				</li>
			{/if}
			{#if winners.keep_or_change_target != null && winners.keep_or_change_target !== plan.preparer_id}
				<li>
					{playerName(players, winners.keep_or_change_target)}
					may re-aim this plan's target before the roll resolves.
				</li>
			{/if}
		</ul>

		<!-- ── Retarget picker ─────────────────────────────────────────── -->
		{#if amKeepOrChangeTargetWinner}
			<div class="demand-form">
				<p class="choices-header">Re-aim this plan</p>
				{#if retargetError}<p class="res-error">{retargetError}</p>{/if}
				<FormField label="Target player">
					<PlayerChips
						{players}
						isActive={(p) => retargetPlayerID === p.id}
						onSelect={selectRetargetPlayer}
					/>
				</FormField>

				{#if retargetPlayerID != null}
					<CardPicker
						label="Target asset (optional)"
						items={retargetCandidateAssets}
						{players}
						emptyMessage="This player has no intact assets."
						selected={retargetAssetID}
						onSelect={(id) => (retargetAssetID = id)}
					/>
				{/if}

				<button class="action-btn primary" onclick={submitRetarget} disabled={retargetBusy}>
					{retargetBusy ? '…' : 'Apply retarget'}
				</button>
			</div>
		{/if}

		<!-- ── Leverage picker ─────────────────────────────────────────── -->
		{#if amControlLeverageWinner && plan.status === 'resolving'}
			<div class="demand-form">
				<p class="choices-header">
					Leverage {playerName(players, plan.preparer_id)}'s assets onto the roll
				</p>
				{#if leverageError}<p class="res-error">{leverageError}</p>{/if}
				<CardPicker
					label="Pick one or more assets"
					items={leverageableTargetAssets}
					{players}
					emptyMessage="No leverageable assets on the target preparer."
					ownerLabel={(a) => a.is_leveraged ? 'already leveraged' : undefined}
					multi
					selectedMulti={selectedLeverageIDs}
					onSelectMulti={(ids) => (selectedLeverageIDs = ids)}
				/>
				{#if leverageableTargetAssets.length > 0}
					<button class="action-btn primary" onclick={submitLeverage}
						disabled={leverageBusy || selectedLeverageIDs.length === 0}>
						{leverageBusy ? '…' : `Leverage ${selectedLeverageIDs.length} asset${selectedLeverageIDs.length === 1 ? '' : 's'}`}
					</button>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.demand-banner {
		border: 1px solid var(--accent, #b8860b);
		background: var(--accent-bg, #fdf6e3);
		padding: 0.75rem;
		margin-bottom: 0.75rem;
		border-radius: 4px;
	}
	.demand-banner-header {
		margin: 0 0 0.5rem 0;
	}
	.demand-winners {
		margin: 0 0 0.5rem 0;
		padding-left: 1.25rem;
		font-size: 0.95em;
	}
	.demand-form {
		margin-top: 0.75rem;
		padding-top: 0.5rem;
		border-top: 1px dashed var(--border, #d9d9d9);
	}
</style>
