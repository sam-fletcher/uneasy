<!-- ResolvingCard.svelte
  The frame of a resolving plan: its steps, in order.

      ⚑ otto prepared Seek Answers          ← finished steps, one line each
      ✎ otto: “the harbour ledgers”
      ⚄ The roll: Make …                    ← the roll, from PlanPanel's frame
      Your move — pick make options         ← the open step's heading
      …the plan's own body…                 ← the open step

  Each of the twelve plan panels wraps its body in this card, so the order is
  decided here once. The card draws what it knows (who prepared the plan, and
  what they wrote), then the roll step and the heading from the frame PlanPanel
  provides (plans/shared/stepFrame), then the panel's body. There is no box and
  no tint: nobody presses a plan as a whole, so it is a run of steps with a
  heading and some space. The old "Resolving" badge and the "by …" tag are
  gone; the lines say it in words.
-->
<script lang="ts">
	import './planPanel.css';
	import { untrack, type Snippet } from 'svelte';
	import type { Plan, Player } from '$lib/api';
	import { PLAN_SHORT, playerName } from './shared';
	import { useStepFrame } from './shared/stepFrame';
	import { useHeadingSlot } from './shared/headingSlot';
	import FinishedStep from './shared/FinishedStep.svelte';
	import LogMark from '../LogMark.svelte';
	import StepHeading from '$lib/components/shared/StepHeading.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	interface Props {
		plan: Plan;
		players: Player[];
		error?: string;
		/** Set false to suppress the generic preparation-notes line and render
		    a custom version in the body (e.g. Spread Rumors' quoted rumor, which
		    must not show a secret rumor's draft as though it were stated). */
		showNotes?: boolean;
		children: Snippet;
	}
	let { plan, players, error = '', showNotes = true, children }: Props = $props();

	const frame = useStepFrame();
	const f = $derived(frame?.() ?? null);

	// Tell the page the roll is drawn in here, for as long as this card is on
	// screen, so it does not draw it again below. Claimed ONCE, on mount: the
	// frame changes with every heading, and an effect that tracked it would let go
	// and re-claim each time, which would move the roll out and back in (and lose
	// what a player had drafted in it).
	$effect(() => untrack(() => frame?.().claimRoll()));

	// The card draws the heading when it has one and the roll is not open (an open
	// roll is the open step and draws it); it claims it from the page while it does,
	// so the page's default steps aside. A card with no heading to draw claims
	// nothing, or the state would show none.
	const slot = useHeadingSlot();
	const drawsHeading = $derived(!!f && !f.rollOpen && f.heading != null);
	$effect(() => {
		if (!drawsHeading) return;
		return untrack(() => slot?.claim());
	});

	const preparer = $derived(playerName(players, plan.preparer_id));
	const notes = $derived(showNotes ? (plan.preparation_notes ?? '').trim() : '');
	// A note that is probably longer than one line gets the whole text a tap away.
	const LONG_NOTE = 40;
</script>

<div class="plan-panel resolving">
	<div class="done-steps">
		<FinishedStep>
			{#snippet mark()}<LogMark family="plan" />{/snippet}
			{preparer} prepared {PLAN_SHORT[plan.plan_type] ?? plan.plan_type}
		</FinishedStep>

		{#if notes}
			{#if notes.length > LONG_NOTE}
				<FinishedStep>
					{#snippet mark()}<LogMark family="marginalia" />{/snippet}
					{preparer}: “{notes}”
					{#snippet detail()}<p class="step-quote">“{notes}”</p>{/snippet}
				</FinishedStep>
			{:else}
				<FinishedStep>
					{#snippet mark()}<LogMark family="marginalia" />{/snippet}
					{preparer}: “{notes}”
				</FinishedStep>
			{/if}
		{/if}

		{#if f?.roll}{@render f.roll()}{/if}
	</div>

	<!-- The open step. While the roll is open it IS the open step and carries its
	     own heading, so the card draws none. -->
	{#if f && !f.rollOpen}
		<StepHeading heading={f.heading} />
	{/if}

	{#if error}
		<ErrorText message={error} variant="panel" />
	{/if}

	{@render children()}
</div>
