<!-- ClandestinelyLiaisePanel.svelte
  Prep + resolve UI for Clandestinely Liaise (Tier 2, Knowledge, variable delay).

  Unlike other plans, CL does not use a standard dice roll. Delay is set by a
  simultaneous reveal at prep time; resolution walks through four phases
  (together_at_last → secrets_we_keep → things_we_share →
  when_will_i_see_you_again → done).

  This panel is the frame. Each step's UI lives in `liaise/`: PrepForm, DelayReveal,
  TogetherAtLast, SecretsWeKeep, ThingsWeShare, WhenWillISeeYouAgain. The frame picks
  the open step from resolution_data.liaise.phase, works out which SEATS the viewer
  fills (`liaiseSeats`, handed to each step), and keeps the Make Demands winner the
  demand overlay reports and the error line ResolvingCard draws.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import './planPanel.css';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, pendingAimSettler } from './shared';

	import type { PlanPanelProps } from './types';

	import PrepForm from './liaise/PrepForm.svelte';
	import DelayReveal from './liaise/DelayReveal.svelte';
	import TogetherAtLast from './liaise/TogetherAtLast.svelte';
	import SecretsWeKeep from './liaise/SecretsWeKeep.svelte';
	import ThingsWeShare from './liaise/ThingsWeShare.svelte';
	import WhenWillISeeYouAgain from './liaise/WhenWillISeeYouAgain.svelte';
	import { clStateOf, liaiseSeats } from './liaise/shared';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);

	// A Make Demands perform_steps winner stands in for the preparer's seat: the demand
	// overlay reports them, and every step reads the seats that follow from it.
	let performStepsWinnerID = $state<number | null>(null);

	const clState = $derived(clStateOf(plan));

	// The error line ResolvingCard draws; every resolve step writes it.
	let resError = $state('');

	// The Make Demands winner still to settle the partner: the liaison's first step waits.
	const aimSettler = $derived(plan ? pendingAimSettler(plan, plans) : null);
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if mode === 'delayReveal' && plan}
	<DelayReveal {ctx} {plan} />

{:else if plan}
	{@const seats = liaiseSeats(ctx, plan, clState, performStepsWinnerID)}
	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if aimSettler != null}
			<p class="ft-prompt muted">
				<PlayerName {players} id={aimSettler} />, the Make Demands winner, is settling who
				<PlayerName {players} id={plan.preparer_id} /> meets. The liaison begins once they have.
			</p>
		{:else if !seats.amParticipant && !seats.amPreparer && !seats.amPartner}
			<!-- A bystander has nothing to do or to read here: the heading says whose
			     move it is and which phase the liaison is in. -->

		{:else if clState.phase === 'together_at_last'}
			<TogetherAtLast {ctx} {plan} {seats} bind:error={resError} />

		{:else if clState.phase === 'secrets_we_keep'}
			<SecretsWeKeep {ctx} {plan} {seats} bind:error={resError} />

		{:else if clState.phase === 'things_we_share'}
			<ThingsWeShare {ctx} {plan} {seats} bind:error={resError} />

		{:else if clState.phase === 'when_will_i_see_you_again'}
			<WhenWillISeeYouAgain {ctx} {plan} {seats} />

		{:else if clState.phase === 'done'}
			<!-- The liaison auto-resolves the moment the redelay reveal finalizes
			     (no make/mar roll, no decision left), so this is only ever a brief
			     transient before the plan flips to 'resolved'. No Complete button —
			     the chat log carries the outcome. -->
			<div class="complete-section">
				<p class="choices-applied">The liaison is complete.</p>
			</div>

		{:else}
			<p class="ft-prompt muted">Phase: {clState.phase || '(unknown)'}</p>
		{/if}
	</ResolvingCard>
{/if}
