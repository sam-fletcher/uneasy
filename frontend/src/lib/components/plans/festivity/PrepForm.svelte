<!-- Festivity/PrepForm.svelte
  Describes the event type as the prep notes. Notes are required.
-->
<script lang="ts">
	import { preparePlan } from '$lib/api';

	let { gameID, onPlanPrepared }: {
		gameID: number;
		onPlanPrepared: () => void;
	} = $props();

	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe the event.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'host_festivity',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}
</script>

<div class="plan-form">
	{#if prepError}<p class="res-error">{prepError}</p>{/if}
	<label class="form-label">
		Event type:
		<textarea rows={3} bind:value={prepNotes} class="form-textarea"
			placeholder="A gala or a ball? A big feast, a hunting party, a tournament?" required></textarea>
	</label>
	<div class="form-actions">
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy || !prepNotes.trim()}>
			{prepBusy ? '…' : 'Prepare Plan'}
		</button>
	</div>
</div>
