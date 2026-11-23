<!-- Seek/PrepForm.svelte
  Prep: notes textarea (required) — the preparer's research methods and topics.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import { preparePlan } from '$lib/api';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { ctx }: { ctx: PlanContext } = $props();

	const gameID = $derived(ctx.gameID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as { notes?: string } | null);

	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe your research methods.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'seek_answers',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => { if (readOnly) prepNotes = prepDraft?.notes ?? ''; });
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly) return;
		void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({ notes: prepNotes });
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });
</script>

<fieldset class="plan-form-fieldset" disabled={readOnly}>
	<div class="plan-form">
		{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
		<label class="form-label">
			Research methods and topics:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
				placeholder="What are you learning, and how?" required></textarea>
		</label>
		{#if !readOnly}
			<div class="form-actions">
				<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy || !prepNotes.trim()}>
					{prepBusy ? '…' : 'Prepare Plan'}
				</button>
			</div>
		{/if}
	</div>
</fieldset>
