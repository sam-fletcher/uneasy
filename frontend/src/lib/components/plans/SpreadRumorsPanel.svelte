<!-- SpreadRumorsPanel.svelte
  Prep + resolve UI for Spread Rumors (Tier 1, Esteem).

  Prep: target asset (any asset at the table) + the rumor text (a DRAFT, and
    optionally kept secret for now).
  Pre-roll: the preparer states the rumor and how it is spreading; the server
    posts it for the table and only then opens the roll. Stating a secret rumor
    makes it public.
  Resolve: counts picker (repeatable, total = dice result) with options
    break_target / leverage_target / take_asset / hide_source / reveal_source.
    After choices are applied, sub-flows cover break_target (marginalia
    picker on the rumor's subject), take_asset (consent button — the aggressor
    names the victim's assets), and hide_source (the actor's own asset picker).

  On make the preparer (focus player) drives the picker. On mar the
  target-asset owner drives everything: first they WRITE the counter-rumor (the
  rumor that spreads about the preparer instead), then pick options — all aimed
  at the preparer's main character, which is the counter-rumor's subject.

  This panel is the frame. Each step's UI lives in `rumors/`: PrepForm, StateRumor,
  CounterRumor, TakeConsent, TakeAssets, PickOptions, AfterChoices. The frame draws
  the rumor as the table has it, picks which step is open, and keeps what has to
  outlive a step: the picks (the picker is off screen during the take-asset screen
  and the consent wait, and the counts must still be there when it comes back) and
  the error line.
-->
<script lang="ts">
	import Quote from '$lib/components/shared/Quote.svelte';
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import './planPanel.css';
	import { parseSpreadRumorsData } from '$lib/plans/resolutionData/spread_rumors';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { choicesCommitted, parseResolutionData, assetName } from './shared';

	import type { PlanPanelProps } from './types';
	import Buffet from './shared/Buffet.svelte';
	import PlanDifficulty from './shared/PlanDifficulty.svelte';
	import { diceNotYetLanded } from '$lib/plans/difficulty';

	import PrepForm from './rumors/PrepForm.svelte';
	import StateRumor from './rumors/StateRumor.svelte';
	import CounterRumor from './rumors/CounterRumor.svelte';
	import TakeConsent from './rumors/TakeConsent.svelte';
	import TakeAssets from './rumors/TakeAssets.svelte';
	import PickOptions from './rumors/PickOptions.svelte';
	import AfterChoices from './rumors/AfterChoices.svelte';
	import { RumorPicks } from './rumors/picks.svelte';
	import { rumorScene } from './rumors/shared';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);

	let performStepsWinnerID = $state<number | null>(null);

	// The error line ResolvingCard draws; every resolve step writes it.
	let resError = $state('');

	const srData = $derived(parseSpreadRumorsData(plan));

	// ── Pre-roll: state the rumor, say how it is spreading ───────────────────
	// OnResolve parks the plan with no roll. The preparer's narration is posted for
	// the table — who read it to decide whether to help or interfere — and only then
	// does the roll open. Plans that rolled before the step existed carry no flag, so
	// "a roll exists for this plan" counts as done too.
	const rollForThisPlan = $derived(plan != null && activeRoll != null && activeRoll.plan_id === plan.id);
	const hasCommittedChoices = $derived(
		plan != null && choicesCommitted(parseResolutionData(plan)),
	);
	const preRollDone = $derived((srData.pre_roll_done ?? false) || rollForThisPlan || hasCommittedChoices);

	// ── The picks, held here ─────────────────────────────────────────────────
	const picks = new RumorPicks();

	// Once the owner declines, blank any take_asset picks so the quota re-fills
	// with other options and the disabled control reads 0.
	const takeAssetDenied = $derived(srData.take_asset_denied ?? false);
	$effect(() => {
		if (takeAssetDenied && (picks.counts.take_asset ?? 0) > 0) {
			picks.counts = { ...picks.counts, take_asset: 0 };
		}
	});

	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			picks.takeStep = 'options'; picks.takeSelected = [];
		}
	});
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan}
	{@const scene = rumorScene(ctx, plan, srData)}
	{@const pendingConsent = srData.pending_take_consent ?? null}
	{@const choicesDone = hasCommittedChoices}

	<ResolvingCard {plan} {players} error={resError} showNotes={false}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<!-- How hard the roll is, for everyone, until the dice land: a rumor about a main
		     character reads the owner's esteem status, any other the preparer's own rank. -->
		{#if diceNotYetLanded(plan, ctx)}
			<PlanDifficulty {plan} {ctx} />
		{/if}
		{#if !choicesDone}
			<Buffet tabs={scene.buffet} />
		{/if}
		<div class="rumor-summary">
			<!-- Before the pre-roll the prep note is only a draft (principle 2), and a
			     secret rumor's text is nobody's business yet — so neither is shown as
			     though it were the rumor. After it, the STATED rumor is. -->
			{#if preRollDone && srData.rumor_text}
				<p class="rumor-label">{scene.mar ? 'The rumor that backfired' : 'The rumor, as stated'}</p>
				<p class="rumor-quote"><Quote text={srData.rumor_text} /></p>
				{#if srData.how_spread}
					<p class="rumor-target">How it's spreading: {srData.how_spread}</p>
				{/if}
			{:else if !preRollDone && srData.is_secret}
				<p class="rumor-label">A secret rumor</p>
				<p class="rumor-target">
					Kept under {#if srData.secret_asset_id != null}<AssetName {assets} id={srData.secret_asset_id} />{:else}an asset{/if}
					until it is stated before the roll.
				</p>
			{:else if plan.preparation_notes && (preRollDone || !scene.isPreparer)}
				<!-- (The preparer's own draft is not repeated here: it is already in the
				     field below, where they are about to restate it.) -->
				<p class="rumor-label">{preRollDone ? 'Rumor' : 'As prepared — a draft, restated before the roll'}</p>
				<p class="rumor-quote"><Quote text={plan.preparation_notes} /></p>
			{/if}
			{#if srData.counter_rumor_text}
				<p class="rumor-label">The rumor that spreads instead</p>
				<p class="rumor-quote"><Quote text={srData.counter_rumor_text} /></p>
			{/if}
			{#if plan.target_asset_id && !scene.mar}
				<p class="rumor-target">
					Targeting: <AssetName {assets} id={plan.target_asset_id} />
				</p>
			{/if}
		</div>

		{#if !preRollDone}
			<StateRumor {ctx} {plan} bind:error={resError} />

		<!-- While the dice are out the roll is the open step and carries its own
		     heading, so there is nothing to say until it lands. -->
		{:else if scene.mar && !choicesDone && !srData.counter_rumor_text && pendingConsent == null}
			<CounterRumor {ctx} {plan} {scene} bind:error={resError} />

		{:else if pendingConsent != null}
			<TakeConsent {ctx} {plan} {scene} {pendingConsent} {picks} bind:error={resError} />

		{:else if rollOutcome != null && !choicesDone && scene.isActor && picks.takeStep === 'assets'}
			<TakeAssets {ctx} {plan} {scene} {picks} bind:error={resError} />

		{:else if rollOutcome != null && !choicesDone}
			<PickOptions {ctx} {plan} {scene} {picks} bind:error={resError} />

		{:else if choicesDone}
			<AfterChoices {ctx} {plan} {scene} bind:error={resError} />
		{/if}
	</ResolvingCard>
{/if}

<style>
	/* The rumor as the table heard it, in quotation marks. Quoted player text is
	   not styled (the style guide keeps italic for asset names), so it is body
	   text with no margin of its own. */
	.rumor-summary { display: flex; flex-direction: column; gap: var(--space-2); }
	.rumor-label {
		font-size: var(--text-label);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-text-faint);
		margin: 0;
	}
	.rumor-quote { margin: 0; }
	.rumor-target {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		margin: 0;
	}
</style>
