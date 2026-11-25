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
	import QuotedText from './shared/QuotedText.svelte';
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

	{#snippet aboutTarget()}about <AssetName {assets} id={plan.target_asset_id} />{/snippet}
	<!-- What the rumor was prepared about stays in the preparation line in a mar too: it is a fact
	     of preparation, and public (the plan list shows the target); the mar only changes whom
	     the rumor that spreads is about, which the counter-rumor's own label says. -->
	<ResolvingCard {plan} {players} error={resError} showNotes={false}
		against={plan.target_asset_id ? aboutTarget : undefined}>
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
		<!-- The rumor is the text this step is about, so it is centred under a small label
		     (Round 6); what it is about is in the preparation line above. Before the
		     pre-roll the prep note is only a draft (principle 2), and a secret rumor's text
		     is nobody's business yet — so neither is shown as though it were the rumor.
		     After it, the STATED rumor is. -->
		{#if preRollDone && srData.rumor_text}
			<QuotedText text={srData.rumor_text} note={srData.how_spread ? `How it's spreading: ${srData.how_spread}` : null}>
				{#snippet label()}{scene.mar ? 'The rumor that backfired' : 'The rumor'}{/snippet}
			</QuotedText>
		{:else if !preRollDone && srData.is_secret}
			<div class="rumor-secret">
				<p class="choices-header">A secret rumor</p>
				<p class="choices-note">
					Kept under {#if srData.secret_asset_id != null}<AssetName {assets} id={srData.secret_asset_id} />{:else}an asset{/if}
					until it is stated before the roll.
				</p>
			</div>
		{:else if plan.preparation_notes && (preRollDone || !scene.isPreparer)}
			<!-- (The preparer's own draft is not repeated here: it is already in the
			     field below, where they are about to restate it.) -->
			<QuotedText text={plan.preparation_notes}>
				{#snippet label()}{preRollDone ? 'The rumor' : 'A draft of the rumor'}{/snippet}
			</QuotedText>
		{/if}
		{#if srData.counter_rumor_text}
			<QuotedText text={srData.counter_rumor_text}>
				{#snippet label()}The rumor that spreads instead{/snippet}
			</QuotedText>
		{/if}

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
	.rumor-secret { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
