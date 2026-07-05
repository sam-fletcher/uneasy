<!-- Festivity/InsistFlow.svelte
  Renders when the current player holds an IOU from rolling make. They
  may force a single mar option on the host (rumor_about_you /
  disagreement / accept_duels / break_self).

  Mars that hinge on a choice about the host's OWN assets (break_self → which
  marginalia; disagreement → which peer) aren't decided here: the insister only
  picks the option, and the host settles the specifics later (HostPendingMar).
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { insistHostMar, type Plan } from '$lib/api';
	import FormField from '../FormField.svelte';
	import { MAR_OPTS } from './options';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { plan, onPlansChanged }: {
		plan: Plan;
		onPlansChanged: () => void;
	} = $props();

	let insistOpen = $state(false);
	let insistChoice = $state<string | null>(null);
	let insistRumor = $state('');
	let insistBusy = $state(false);
	let insistError = $state('');

	// rumor_about_you needs the rumor written; the other mars carry no input
	// here (the host settles break_self / disagreement specifics later).
	const insistReady = $derived(
		insistChoice != null &&
		(insistChoice !== 'rumor_about_you' || insistRumor.trim().length > 0),
	);

	async function submitInsist() {
		if (insistBusy || !insistReady) return;
		insistBusy = true; insistError = '';
		try {
			const body: { mar_option: string; rumor_text?: string } = {
				mar_option: insistChoice!,
			};
			if (insistChoice === 'rumor_about_you') body.rumor_text = insistRumor.trim();
			await insistHostMar(plan.id, body);
			insistOpen = false;
			insistChoice = null;
			insistRumor = '';
			onPlansChanged();
		} catch (e) {
			insistError = e instanceof Error ? e.message : 'Could not insist.';
		} finally { insistBusy = false; }
	}
</script>

<div class="insist-flow">
	{#if !insistOpen}
		<button class="action-btn" onclick={() => (insistOpen = true)}>
			Insist on a mar
		</button>
	{:else}
		<FormField label="Force a mar option on the host">
			<div class="chip-row">
				{#each MAR_OPTS as o}
					<button
						type="button"
						class="chip-btn"
						class:active={insistChoice === o.key}
						onclick={() => {
							insistChoice = insistChoice === o.key ? null : o.key;
						}}
					>{o.label}</button>
				{/each}
			</div>
		</FormField>
		{#if insistChoice === 'rumor_about_you'}
			<label class="form-label">
				Rumor text (about the host):
				<textarea rows={2} bind:value={insistRumor} class="form-textarea" maxlength={TEXT_LIMITS.LONG_TEXT}></textarea>
			</label>
		{:else if insistChoice === 'disagreement'}
			<p class="choices-note muted">
				The host will choose which of their peers falls out and storms to the
				center of the table.
			</p>
		{:else if insistChoice === 'accept_duels'}
			<p class="choices-note muted">
				The host must accept any duel challenges from guests for the rest of the event.
			</p>
		{:else if insistChoice === 'break_self'}
			<p class="choices-note muted">
				The host will choose a marginalia to tear on their own main character.
			</p>
		{/if}
		{#if insistError}<p class="res-error">{insistError}</p>{/if}
		<div class="form-row">
			<button class="action-btn primary"
				onclick={submitInsist}
				disabled={insistBusy || !insistReady}>
				{insistBusy ? '…' : 'Insist'}
			</button>
			<button class="action-btn" onclick={() => { insistOpen = false; insistChoice = null; }}>
				Cancel
			</button>
		</div>
	{/if}
</div>

<style>
	/* Stack the chip picker, the rumor/peer sub-input, and the action row
	   with consistent vertical rhythm — without this the children butt up
	   against each other and the buttons crowd the element above them. */
	.insist-flow {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
</style>
