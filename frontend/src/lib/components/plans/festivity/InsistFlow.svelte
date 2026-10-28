<!-- Festivity/InsistFlow.svelte
  Renders when the current player holds an IOU from rolling make. The book
  (p.61) lets them "insist that the host choose one option from the mar list at
  any time" — the HOST chooses which, so there is nothing to pick here: one
  button, one use. The guest's leverage is *when* they insist.

  The host answers on their pending-mar card (HostPendingMar).
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { insistHostMar, type Plan } from '$lib/api';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { plan, onPlansChanged }: {
		plan: Plan;
		onPlansChanged: () => void;
	} = $props();

	let busy = $state(false);
	let error = $state('');

	async function insist() {
		if (busy) return;
		busy = true; error = '';
		try {
			await insistHostMar(plan.id);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not insist.';
		} finally { busy = false; }
	}
</script>

<div class="insist-flow">
	<p class="choices-note muted">
		You can insist at any point that the host take a Mar — they choose which.
		You only get one, so pick your moment.
	</p>
	{#if error}<ErrorText message={error} variant="panel" />{/if}
	<button class="action-btn" onclick={insist} disabled={busy}>
		{busy ? '…' : 'Insist the host take a Mar'}
	</button>
</div>

<style>
	.insist-flow {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
</style>
