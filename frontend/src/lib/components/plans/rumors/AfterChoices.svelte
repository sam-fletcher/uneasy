<!-- Rumors/AfterChoices.svelte
  Resolve, once the choices are committed: what was picked and what each step did, then
  the sub-flows that remain — break_target (marginalia picker on the rumor's subject)
  and hide_source (the actor's own asset picker) — and the preparer's Complete.

  take_asset has no post-commit sub-flow: the transfer happens at consent time, so only
  break_target and hide_source remain here. Their completion is server-authoritative
  (resolution_data counters), so a refresh/remount can't re-prompt — and re-submit — a
  step that already ran.

  The committed state is drawn at full opacity for EVERY viewer, outside the greyed
  picker area; only the actor works the pickers.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import {
		completePlan, breakTarget, hideSource, forfeitSpreadRumorsStep,
		type Plan,
	} from '$lib/api';
	import ChoicesApplied from '../ChoicesApplied.svelte';
	import CardPicker from '../CardPicker.svelte';
	import EndsNote from '../shared/EndsNote.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { destructionWarning } from '$lib/assetRisk';
	import { parseResolutionData } from '../shared';
	import type { PlanContext } from '../types';
	import ResolvedSoFar from './ResolvedSoFar.svelte';
	import { countIn, LAST_STEP_TEXT as lastText, type RumorScene } from './shared';

	let { ctx, plan, scene, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const isActor = $derived(scene.isActor);
	const srData = $derived(scene.srData);

	const existingChoices = $derived((parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option));
	const btDone = $derived(srData.break_target_done ?? 0);
	const hsDone = $derived(srData.hide_source_done ?? 0);
	const btRemaining = $derived(Math.max(0, countIn(existingChoices, 'break_target') - btDone));
	const hsRemaining = $derived(Math.max(0, countIn(existingChoices, 'hide_source') - hsDone));
	const subflowsDone = $derived(btRemaining === 0 && hsRemaining === 0);
	// Every route here auto-completes the plan the moment nothing is owed, so the
	// submit that leaves nothing owed is the one that ends Spread Rumors.
	const subflowsOwed = $derived(btRemaining + hsRemaining);

	// On a mar, break_target lands on the preparer's main character and take_asset
	// picks from the preparer's assets; on a make they operate on the plan's target
	// asset. btAssetID only feeds the pre-tear destruction warning.
	let btAssetID = $state<number | null>(null);
	let btMargID = $state<number | null>(null);
	let btBusy = $state(false);
	const preparerAssets = $derived(
		assets.filter(a => a.owner_id === plan.preparer_id && !a.is_destroyed),
	);
	// Pre-tear warning for the currently-selected break target.
	const btWarn = $derived(destructionWarning(assets.find(a => a.id === btAssetID)));

	async function submitBreakTarget(p: Plan) {
		if (btBusy || btMargID == null) return;
		btBusy = true; error = '';
		try {
			await breakTarget(p.id, btMargID);
			btMargID = null; btAssetID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not break target.';
		} finally { btBusy = false; }
	}

	// hide_source: actor hides their own role as source on one of their own
	// assets. Actor = preparer on make, target-asset owner on mar.
	let hsAssetID = $state<number | null>(null);
	let hsBusy = $state(false);
	// A Make Demands winner standing in for the preparer on a make picks one of the
	// PREPARER's assets, not their own (design principle 9).
	const standingIn = $derived(
		isActor && rollOutcome !== 'mar' && currentPlayerID !== plan.preparer_id,
	);
	const hsAssetOptions = $derived(
		scene.mar
			? assets.filter(a => a.owner_id === scene.targetAssetOwnerID && !a.is_destroyed)
			: preparerAssets
	);
	async function submitHideSource(p: Plan) {
		if (hsBusy || hsAssetID == null) return;
		hsBusy = true; error = '';
		try {
			await hideSource(p.id, hsAssetID);
			hsAssetID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not hide source.';
		} finally { hsBusy = false; }
	}

	// Discharge a depletable step that has remaining picks but no live target —
	// break_target with no intact marginalia, or hide_source with no own asset.
	// The server re-checks no target exists, so this only resolves a genuine
	// dead-end. Without it the plan wedges (completion is now server-gated).
	let forfeitBusy = $state(false);
	async function forfeitStep(p: Plan, step: 'break_target' | 'hide_source') {
		if (forfeitBusy) return;
		forfeitBusy = true; error = '';
		try {
			await forfeitSpreadRumorsStep(p.id, step);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not skip step.';
		} finally { forfeitBusy = false; }
	}

	// ── Complete ─────────────────────────────────────────────────────────────
	let resBusy = $state(false);
	async function onComplete(p: Plan) {
		if (resBusy) return;
		resBusy = true; error = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { resBusy = false; }
	}
</script>

<div class="complete-section">
	<!-- Committed state (Tier-1, ADR-006): the picked options and what each
	     step actually did, at full opacity for EVERY viewer, outside the
	     greyed picker area. -->
	<ChoicesApplied choices={existingChoices} options={scene.options} />
	<ResolvedSoFar {ctx} {plan} {scene} />

	<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!isActor} disabled={!isActor}>

	{#if btRemaining > 0}
		<div class="plan-form">
			<p class="choices-header">{scene.mar ? `Break ${scene.mcName}` : 'Break the target asset'}</p>
			<p class="choices-note">{btRemaining} remaining.</p>
			<CardPicker
				label="Marginalium to tear"
				items={scene.btMarginaliaAssets}
				{players}
				readOnly={!isActor}
				emptyMessage="Nothing left to break."
				marginaliaMode
				selectedMarginaliaID={btMargID}
				selectedAssetID={btAssetID}
				onSelectMarginalia={(mID, parent) => {
					btMargID = mID;
					btAssetID = parent?.id ?? null;
				}}
			/>
			{#if btWarn}<Notice>{btWarn}</Notice>{/if}
			{#if isActor}
				{#if scene.btMarginaliaAssets.length === 0}
					<p class="choices-note muted">Nothing left to break — this pick has no valid target.</p>
					<button class="action-btn primary"
						onclick={() => forfeitStep(plan, 'break_target')} disabled={forfeitBusy}>
						{forfeitBusy ? '…' : 'Skip — no valid targets'}
					</button>
					<EndsNote show={subflowsOwed === btRemaining} text={lastText} />
				{:else}
					<button class="action-btn primary"
						onclick={() => submitBreakTarget(plan)}
						disabled={btBusy || btMargID == null}>
						{btBusy ? '…' : 'Tear marginalia'}
					</button>
					<EndsNote show={subflowsOwed === 1} text={lastText} />
				{/if}
			{/if}
		</div>
	{/if}

	{#if hsRemaining > 0}
		<div class="plan-form">
			<p class="choices-header">Hide the source</p>
			<p class="choices-note">{hsRemaining} remaining.</p>
			<p class="choices-note muted">
				{#if standingIn}
					As the Make Demands winner you choose which of <PlayerName {players} id={scene.shelterID} />'s assets hides
					the Secret that <PlayerName {players} id={scene.shelterID} /> spread the rumor.
				{:else if isActor}
					An asset of your choice will hide the Secret that you spread the rumor.
				{:else if scene.shelterName !== scene.actorName}
					<PlayerName {players} id={scene.actorID} />, the Make Demands winner, chooses which of <PlayerName {players} id={scene.shelterID} />'s assets
					hides the Secret that <PlayerName {players} id={scene.shelterID} /> spread the rumor — their character stays
					unnamed at court, but the table sees it.
				{:else}
					An asset of <PlayerName {players} id={scene.actorID} />'s choice will hide the Secret that they spread
					the rumor — their character stays unnamed at court, but the table sees it.
				{/if}
			</p>
			<CardPicker
				label={isActor && !standingIn ? 'Hide on one of your assets' : `Hide on one of $<PlayerName {players} id={scene.shelterID} />'s assets`}
				items={hsAssetOptions}
				{players}
				readOnly={!isActor}
				selected={hsAssetID}
				onSelect={(id) => (hsAssetID = id)}
			/>
			{#if isActor}
				{#if hsAssetOptions.length === 0}
					<p class="choices-note muted">{standingIn ? `$<PlayerName {players} id={scene.shelterID} /> has` : 'You have'} no asset to hide the source under — this pick has no valid target.</p>
					<button class="action-btn primary"
						onclick={() => forfeitStep(plan, 'hide_source')} disabled={forfeitBusy}>
						{forfeitBusy ? '…' : 'Skip — no valid targets'}
					</button>
					<EndsNote show={subflowsOwed === hsRemaining} text={lastText} />
				{:else}
					<button class="action-btn primary"
						onclick={() => submitHideSource(plan)}
						disabled={hsBusy || hsAssetID == null}>
						{hsBusy ? '…' : 'Hide source'}
					</button>
					<EndsNote show={subflowsOwed === 1} text={lastText} />
				{/if}
			{/if}
		</div>
	{/if}

	</fieldset>

	<!-- Completion is always the preparer's, even on a mar where the
	     target-asset owner drove the sub-flows. -->
	{#if subflowsDone}
		{#if scene.isPreparer}
			<p class="complete-note">
				Post any follow-scene narration in the chat, then complete.
			</p>
			<button class="action-btn primary"
				onclick={() => onComplete(plan)} disabled={resBusy}>
				{resBusy ? '…' : 'Complete plan'}
			</button>
		{/if}
	{/if}
</div>
