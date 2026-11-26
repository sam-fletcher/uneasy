<!-- ResolvingCard.svelte
  The frame of a resolving plan: its steps, in order.

      ⚑ otto prepared Make Demands against bob's Seek Answers (row 5)
                                            ← finished steps, one line each; the
                                              preparation line also carries the facts
                                              fixed at preparation (`against`)
      ✎ otto: “the harbour ledgers”
      ⚄ The roll: Make …                    ← the roll, from PlanPanel's frame
      Your move · also ◆ bob                ← the open step's who-line
      Pick make options                     ← and its heading
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
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import Quote from '$lib/components/shared/Quote.svelte';
	import './planPanel.css';
	import { untrack, type Snippet } from 'svelte';
	import type { Plan, Player } from '$lib/api';
	import { PLAN_SHORT, playerName, parseResolutionData } from './shared';
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
		/** What the plan was prepared against (when `moved`, what it is aimed at now), said in the preparation line ("against
		    alice's Seek Answers (row 5)"). Facts fixed at preparation live here, not as
		    a lead line in the open step, so a resolving plan carries its own facts. */
		against?: Snippet;
		/** Set false to suppress the generic preparation-notes line and render
		    a custom version in the body (e.g. Spread Rumors' quoted rumor, which
		    must not show a secret rumor's draft as though it were stated). */
		showNotes?: boolean;
		/** Finished steps a plan adds after the preparation line and its notes: a step the
		    plan has settled whose outcome the later steps carry (Propose Duel's pairing).
		    Pass one <FinishedStep> per line. */
		finished?: Snippet;
		children: Snippet;
	}
	let { plan, players, error = '', showNotes = true, against, finished, children }: Props = $props();

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

	// A plan's aim can move after preparation: its preparer re-aims it (a lost mark), or a Make
	// Demands winner settles where it points, and a kept target can't be told from a changed
	// one. The data holds only the current aim, so the line then says "now against …" rather
	// than claiming what was prepared.
	const aimMoved = $derived.by(() => {
		const rd = parseResolutionData(plan);
		return rd.reaim != null || rd.demand_retarget_finalized === true;
	});

	const notes = $derived(showNotes ? (plan.preparation_notes ?? '').trim() : '');
	// A note that is probably longer than one line gets the whole text a tap away.
	const LONG_NOTE = 40;
</script>

<div class="plan-panel resolving">
	<div class="done-steps">
		<FinishedStep wrap={!!against}>
			{#snippet mark()}<LogMark family="plan" />{/snippet}
			<PlayerName {players} id={plan.preparer_id} /> prepared {PLAN_SHORT[plan.plan_type] ?? plan.plan_type}{#if against}{#if aimMoved},{' '}now{/if}{' '}{@render against()}{/if}
		</FinishedStep>

		{#if notes}
			{#if notes.length > LONG_NOTE}
				<FinishedStep>
					{#snippet mark()}<LogMark family="marginalia" />{/snippet}
					<PlayerName {players} id={plan.preparer_id} />: <Quote text={notes} />
					{#snippet detail()}<p class="step-quote"><Quote text={notes} /></p>{/snippet}
				</FinishedStep>
			{:else}
				<FinishedStep>
					{#snippet mark()}<LogMark family="marginalia" />{/snippet}
					<PlayerName {players} id={plan.preparer_id} />: <Quote text={notes} />
				</FinishedStep>
			{/if}
		{/if}

		{#if finished}{@render finished()}{/if}

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

	<!-- The panel's own body: a column of groups, with the group gap between them. -->
	<div class="step-groups">
		{@render children()}
	</div>
</div>
