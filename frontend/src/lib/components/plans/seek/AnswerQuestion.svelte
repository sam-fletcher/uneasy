<!-- Seek/AnswerQuestion.svelte
  The questioned player answers truthfully — or vetoes, if they outrank the preparer on
  knowledge (once, on the first question). Shown in place of the sub-step area to the
  player a question is pending for.
-->
<script lang="ts">
	import Quote from '$lib/components/shared/Quote.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { vetoQuestion, answerQuestion, type Plan } from '$lib/api';
	import EndsNote from '../shared/EndsNote.svelte';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import type { SeekScene } from './shared';
	import type { SeekDrafts } from './drafts.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan, scene, pendingQuestion, drafts, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: SeekScene;
		pendingQuestion: NonNullable<SeekScene['pendingQuestion']>;
		drafts: SeekDrafts;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	// Target's answer/veto of the open question.
	let answerBusy = $state(false);
	async function submitAnswer(p: Plan) {
		if (answerBusy || !drafts.answerText.trim()) return;
		answerBusy = true; error = '';
		try {
			await answerQuestion(p.id, drafts.answerText.trim());
			drafts.answerText = '';
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not answer.';
		} finally { answerBusy = false; }
	}
	async function submitVeto(p: Plan) {
		if (answerBusy) return;
		answerBusy = true; error = '';
		try {
			await vetoQuestion(p.id);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not veto.';
		} finally { answerBusy = false; }
	}
</script>

<div class="plan-form">
	<p class="choices-header">A question for you</p>
	<p class="ft-prompt"><PlayerName {players} id={plan.preparer_id} /> asks:</p>
	<p class="q-quote"><Quote text={pendingQuestion.question} /></p>
	<label class="form-label">
		Your truthful answer
		<textarea rows={3} bind:value={drafts.answerText} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
			placeholder="Answer the question…"></textarea>
	</label>
	<div class="form-actions start">
		<button class="action-btn primary"
			onclick={() => submitAnswer(plan)}
			disabled={answerBusy || !drafts.answerText.trim()}>
			{answerBusy ? '…' : 'Answer'}
		</button>
		{#if pendingQuestion.vetoable}
			<button class="action-btn secondary"
				onclick={() => submitVeto(plan)} disabled={answerBusy}>
				Veto (ask another)
			</button>
		{/if}
	</div>
	<EndsNote show={scene.totalOwed === 1 && scene.aqRemaining === 1}
		text="Answering is the last step: Seek Answers resolves when you answer. A veto ends nothing." />
	{#if pendingQuestion.vetoable}
		<p class="choices-note muted">
			You outrank <PlayerName {players} id={plan.preparer_id} /> on knowledge, so you
			may veto this first question — they'll have to ask another.
		</p>
	{/if}
</div>

<style>
	/* The question put to a player, in quotation marks. Quoted player text is not
	   styled (the style guide keeps italic for asset names), so it is body text
	   with no margin of its own: the same as the rumor in Spread Rumors. */
	.q-quote { margin: 0; }
</style>
