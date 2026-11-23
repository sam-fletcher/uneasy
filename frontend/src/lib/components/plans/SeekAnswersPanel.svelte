<!-- SeekAnswersPanel.svelte
  Prep + resolve UI for Seek Answers (Tier 1, Knowledge).

  Flow:
  - Prep: notes textarea (required).
  - Resolve: dice roll → pick option counts (repeatable, total = dice result)
    → submit via makeChoice → fill a sub-form per pick (break_resource,
    reveal_secret, declare_truth, ask_question) → complete. ask_question is a
    two-sided flow: the target answers (or, if they outrank the preparer on
    knowledge, vetoes the first question once).

  The option list is the same for make and mar (per spec). Each resource may
  be flawed at most once ("overlooked until now"), so flawed resources drop
  out of the pickers. On a mar the preparer must additionally describe a flaw
  in (difficulty − result) of their OWN resources; that penalty sub-flow blocks
  completion until satisfied.

  This panel is the frame. Each step's UI lives in `seek/`: PrepForm, PreRoll,
  PickOptions, AnswerQuestion and AfterChoices (which holds the sub-steps: BreakResource,
  RevealSecret, DeclareTruth, AskQuestion, MarPenalty). The frame picks the open step,
  works out the facts several steps read (`seekScene`, handed to each), and keeps what
  has to outlive a step: the picks and typed text (`SeekDrafts`) and the error line.
-->
<script lang="ts">
	import './planPanel.css';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { useSecretCounts } from '$lib/secretCountsContext';

	import type { PlanPanelProps } from './types';
	import Buffet from './shared/Buffet.svelte';
	import PlanDifficulty from './shared/PlanDifficulty.svelte';
	import { diceNotYetLanded } from '$lib/plans/difficulty';

	import PrepForm from './seek/PrepForm.svelte';
	import PreRoll from './seek/PreRoll.svelte';
	import PickOptions from './seek/PickOptions.svelte';
	import AnswerQuestion from './seek/AnswerQuestion.svelte';
	import AfterChoices from './seek/AfterChoices.svelte';
	import { SeekDrafts } from './seek/drafts.svelte';
	import { BUFFET_TABS, seekScene } from './seek/shared';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollOutcome = $derived(ctx.rollOutcome);

	let performStepsWinnerID = $state<number | null>(null);

	// The error line ResolvingCard draws; every resolve step writes it.
	let resError = $state('');

	// What the viewer has picked or typed and not yet sent, held here (see drafts.svelte.ts).
	const drafts = new SeekDrafts();

	// The viewer's own lookup of how many of an asset's secrets they can already read,
	// which "learn an asset's secrets" needs; it comes from context, so it is fetched here.
	const secretCounts = useSecretCounts();
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan}
	{@const scene = seekScene(ctx, plan, (id) => secretCounts?.known(id) ?? 0)}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<!-- How hard the roll is, for everyone, until the dice land. -->
		{#if diceNotYetLanded(plan, ctx)}
			<PlanDifficulty {plan} {ctx} />
		{/if}
		{#if !scene.choicesDone}
			<Buffet tabs={BUFFET_TABS} />
		{/if}
		{#if !scene.preRollDone}
			<PreRoll {ctx} {plan} {scene} bind:error={resError} />

		<!-- While the dice are out the roll is the open step and carries its own
		     heading, so there is nothing to say until it lands. -->
		{:else if rollOutcome != null && !scene.choicesDone}
			<PickOptions {ctx} {plan} {scene} {drafts} bind:error={resError} />

		{:else if scene.pendingQuestion != null && scene.pendingQuestion.target_id === currentPlayerID}
			<AnswerQuestion {ctx} {plan} {scene} pendingQuestion={scene.pendingQuestion} {drafts}
				bind:error={resError} />

		{:else if scene.choicesDone}
			<AfterChoices {ctx} {plan} {scene} {drafts} bind:error={resError} />
		{/if}
	</ResolvingCard>
{/if}
