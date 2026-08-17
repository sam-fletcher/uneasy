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
	import {
		playerName, activeDemandAgainst, demandWinnersFromPlan,
		parseResolutionData, targetHasPreparerRoll, PLAN_SHORT,
	} from '../shared';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import FormField from '../FormField.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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

	// Whether this target resolves through a roll of its own preparer's. When it
	// doesn't (Host Festivity, Propose Duel, Clandestinely Liaise) the
	// control_leverage option has nothing to attach to and the server refuses it —
	// the rules expect the four options to land unevenly, so say "dud" plainly
	// rather than offering a picker that 409s.
	const targetRolls = $derived(targetHasPreparerRoll(plan.plan_type));

	// The two pre-roll finalize flags, written on THIS plan's resolution_data by
	// /demand-leverage and /demand-retarget. Until each flips, its winner is
	// seeded unready and holds the roll open.
	const targetResData = $derived(parseResolutionData(plan));
	const leverageFinalized = $derived(targetResData.demand_leverage_finalized === true);
	const retargetFinalized = $derived(targetResData.demand_retarget_finalized === true);

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

	// Both branches finalize: "keep" is as much a decision as re-aiming, and the
	// roll is held open until one of them lands (see demandRetarget's doc).
	async function submitRetarget(keep: boolean) {
		if (retargetBusy) return;
		retargetBusy = true; retargetError = '';
		try {
			await demandRetarget(plan.id, keep
				? { keep: true }
				: { target_player_id: retargetPlayerID, target_asset_id: retargetAssetID });
		} catch (e) {
			retargetError = e instanceof Error ? e.message : 'Could not settle this plan’s target.';
		} finally { retargetBusy = false; }
	}

	// ── Leverage picker ───────────────────────────────────────────────────────
	// The control_leverage winner picks how many of the target preparer's
	// non-destroyed assets are leveraged onto the target plan's roll. The
	// backend silently skips assets already on the roll, so we don't need to
	// fetch the roll to filter — show all of the target preparer's intact
	// assets. Unsupported (no open roll, plan resolved, etc.) → backend 409.
	//
	// ZERO IS A REAL SUBMISSION, not an empty form: "none of them do, possibly
	// guaranteeing its failure" is one of the two outcomes the rules single out
	// as the most dramatic. So the finalize button always renders and is never
	// disabled at zero — including when the preparer has no intact assets at all,
	// where finalizing is the only way to stop holding the roll open.

	let selectedLeverageIDs = $state<number[]>([]);
	let leverageBusy = $state(false);
	let leverageError = $state('');

	const leverageableTargetAssets = $derived(
		assets.filter(a => a.owner_id === plan.preparer_id && !a.is_destroyed),
	);

	async function submitLeverage() {
		if (leverageBusy) return;
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
					{#if targetRolls}
						{playerName(players, winners.control_leverage)}
						controls leverage of {playerName(players, plan.preparer_id)}'s
						assets on this plan's roll.
					{:else}
						{playerName(players, winners.control_leverage)}
						drew leverage control, but {PLAN_SHORT[plan.plan_type]} has no roll
						of its own — that option came to nothing here.
					{/if}
				</li>
			{/if}
			{#if winners.keep_or_change_target != null && winners.keep_or_change_target !== plan.preparer_id}
				<li>
					{playerName(players, winners.keep_or_change_target)}
					decides whether this plan keeps or changes its target{targetRolls
						? ', and the roll waits until they say'
						: ''}.
				</li>
			{/if}
		</ul>

		<!-- ── Retarget picker ─────────────────────────────────────────── -->
		{#if amKeepOrChangeTargetWinner}
			<div class="demand-form">
				<p class="choices-header">Keep or change this plan's target</p>
				{#if retargetFinalized}
					<p class="choices-note">
						You've settled where this plan is aimed{targetRolls
							? ' — the roll is no longer held for you'
							: ''}. You may still adjust it until the plan resolves.
					</p>
				{:else}
					<p class="choices-note">
						Keeping the target is as much your call as re-aiming it — but the
						table needs to hear which{targetRolls ? ', so the roll waits on you' : ''}.
					</p>
				{/if}
				{#if retargetError}<ErrorText message={retargetError} variant="panel" />{/if}
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

				<div class="demand-actions">
					<button class="action-btn primary" onclick={() => submitRetarget(false)}
						disabled={retargetBusy}>
						{retargetBusy ? '…' : 'Re-aim this plan'}
					</button>
					<button class="action-btn secondary" onclick={() => submitRetarget(true)}
						disabled={retargetBusy}>
						{retargetBusy ? '…' : 'Keep the current target'}
					</button>
				</div>
			</div>
		{/if}

		<!-- ── Leverage picker ─────────────────────────────────────────── -->
		{#if amControlLeverageWinner && plan.status === 'resolving'}
			<div class="demand-form">
				<p class="choices-header">
					Leverage {playerName(players, plan.preparer_id)}'s assets onto the roll
				</p>
				{#if !targetRolls}
					<!-- D7: nothing of the preparer's to leverage into. The rules expect
					     the four options to land unevenly, so name the dud instead of
					     offering a picker the server would refuse. -->
					<p class="choices-note">
						{PLAN_SHORT[plan.plan_type]} resolves without a roll of
						{playerName(players, plan.preparer_id)}'s own, so there is nothing
						here to leverage. This option drew a dud.
					</p>
				{:else}
					{#if leverageFinalized}
						<p class="choices-note">
							Your leverage decision is in — the roll is no longer held for you.
						</p>
					{:else}
						<p class="choices-note">
							Commit as much or as little of it as you like. Leveraging none is a
							real move: it can guarantee the roll fails.
						</p>
					{/if}
					{#if leverageError}<ErrorText message={leverageError} variant="panel" />{/if}
					<CardPicker
						label="Pick any number of assets"
						items={leverageableTargetAssets}
						{players}
						emptyMessage="No leverageable assets on the target preparer."
						ownerLabel={(a) => a.is_leveraged ? 'already leveraged' : undefined}
						multi
						selectedMulti={selectedLeverageIDs}
						onSelectMulti={(ids) => (selectedLeverageIDs = ids)}
					/>
					<div class="demand-actions">
						<button class="action-btn primary" onclick={submitLeverage} disabled={leverageBusy}>
							{#if leverageBusy}
								…
							{:else if selectedLeverageIDs.length === 0}
								Leverage none — let the roll fail
							{:else}
								Leverage {selectedLeverageIDs.length} asset{selectedLeverageIDs.length === 1 ? '' : 's'}
							{/if}
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.demand-banner {
		border: 1px solid var(--color-warning-ink);
		background: var(--parchment-50);
		padding: 0.75rem;
		margin-bottom: 0.75rem;
		border-radius: 4px;
		/* The card-face fill is the app's one light ground, so it needs the ink
		   colour set with it — inherited text is tuned for dark panels and lands
		   at about 1.1:1 here. Same pairing as .res-warning in planPanel.css. */
		color: var(--color-bg);
	}
	/* planPanel.css's muted-note grey is likewise a dark-panel colour; re-ink it
	   to the warm faint token, which clears AA on this fill. */
	.demand-banner .choices-note {
		color: var(--color-text-faint-warm);
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
		border-top: 1px dashed var(--parchment-300);
	}
	/* Wraps to one button per line in a narrow column, so both stay full-width
	   targets on a phone rather than shrinking to sit side by side. */
	.demand-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
</style>
