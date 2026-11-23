<!-- Seek/AskQuestion.svelte
  Sub-step: ask a player a question (they must be truthful). A two-sided flow: the
  target answers (or, if they outrank the preparer on knowledge, vetoes the first
  question once — AnswerQuestion). While one is pending this reads who is being waited
  on; after a veto the replacement question stays with the same player.
-->
<script lang="ts">
	import { askQuestion, type Plan } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import type { SeekScene } from './shared';
	import type { SeekDrafts } from './drafts.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan, scene, drafts, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: SeekScene;
		drafts: SeekDrafts;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const canSubflow = $derived(scene.amChoiceActor);
	const pendingQuestion = $derived(scene.pendingQuestion);
	const askVetoedTargetID = $derived(scene.askVetoedTargetID);

	// A veto locks the target: "ask another [question] in its stead" goes to the
	// same player, so the chip row collapses to them, pre-selected.
	const aqTargetID = $derived(askVetoedTargetID ?? drafts.aqPickedID);
	const aqChoices = $derived(
		askVetoedTargetID != null ? scene.questionTargets.filter(p => p.id === askVetoedTargetID) : scene.questionTargets
	);
	let aqBusy = $state(false);
	async function submitAskQuestion(p: Plan) {
		if (aqBusy || aqTargetID == null || !drafts.aqQuestion.trim()) return;
		aqBusy = true; error = '';
		try {
			await askQuestion(p.id, aqTargetID, drafts.aqQuestion.trim());
			drafts.aqPickedID = null; drafts.aqQuestion = '';
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not ask question.';
		} finally { aqBusy = false; }
	}
</script>

<div class="plan-form">
	<p class="choices-header">
		Ask a player a question ({scene.aqRemaining} remaining)
	</p>
	{#if pendingQuestion != null}
		<p class="choices-note">
			Waiting for {playerName(players, pendingQuestion.target_id)} to answer
			{#if pendingQuestion.vetoable}or veto{' '}{/if}{canSubflow ? 'your' : 'the'} question:
		</p>
		<p class="q-quote">“{pendingQuestion.question}”</p>
	{:else}
		{#if askVetoedTargetID != null}
			<p class="choices-note muted">
				{canSubflow ? 'Your' : 'The'} last question was vetoed by
				{playerName(players, askVetoedTargetID)} — ask them another (it can't be vetoed).
			</p>
		{/if}
		<PlayerChips
			players={aqChoices}
			isActive={(p) => p.id === aqTargetID}
			onSelect={(p) => (drafts.aqPickedID = p.id)}
			readOnly={!canSubflow || askVetoedTargetID != null}
		/>
		<label class="form-label">
			Your question:
			<textarea rows={2} bind:value={drafts.aqQuestion} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
				placeholder="Ask a question they must answer truthfully…"></textarea>
		</label>
		{#if canSubflow}
			<button class="action-btn primary"
				onclick={() => submitAskQuestion(plan)}
				disabled={aqBusy || aqTargetID == null || !drafts.aqQuestion.trim()}>
				{aqBusy ? '…' : 'Ask question'}
			</button>
		{/if}
	{/if}
</div>

<style>
	/* The question put to a player, in quotation marks. Quoted player text is not
	   styled (the style guide keeps italic for asset names), so it is body text
	   with no margin of its own: the same as the rumor in Spread Rumors. */
	.q-quote { margin: 0; }
</style>
