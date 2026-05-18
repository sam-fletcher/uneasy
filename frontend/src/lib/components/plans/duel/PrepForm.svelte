<!-- Duel/PrepForm.svelte
  Prep: pick a challenger, choose arms vs wits, describe the location.
-->
<script lang="ts">
	import { preparePlan, type Player } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import FormField from '../FormField.svelte';
	import { playersExcept } from '../shared';

	let { gameID, players, currentPlayerID, onPlanPrepared }: {
		gameID: number;
		players: Player[];
		currentPlayerID: number | null;
		onPlanPrepared: () => void;
	} = $props();

	let prepTargetPlayerID = $state<number | null>(null);
	let prepDuelType = $state<'arms' | 'wits'>('arms');
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));

	async function submitPrep() {
		if (prepBusy) return;
		if (prepTargetPlayerID == null) { prepError = 'Pick a challenger.'; return; }
		if (!prepNotes.trim()) { prepError = 'Describe the location of the duel.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'propose_duel',
				target_player_id: prepTargetPlayerID,
				duel_type: prepDuelType,
				preparation_notes: prepNotes.trim(),
			});
			prepTargetPlayerID = null;
			prepDuelType = 'arms';
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}
</script>

<div class="plan-form">
	{#if prepError}<p class="res-error">{prepError}</p>{/if}
	<FormField label="Challenger">
		<PlayerChips
			players={otherPlayers}
			isActive={(p) => prepTargetPlayerID === p.id}
			onSelect={(p) => (prepTargetPlayerID = prepTargetPlayerID === p.id ? null : p.id)}
		/>
	</FormField>
	<FormField label="Duel of">
		<div class="chip-row">
			<button
				type="button"
				class="chip-btn"
				class:active={prepDuelType === 'arms'}
				onclick={() => (prepDuelType = 'arms')}
			>Arms</button>
			<button
				type="button"
				class="chip-btn"
				class:active={prepDuelType === 'wits'}
				onclick={() => (prepDuelType = 'wits')}
			>Wits / Trial</button>
		</div>
	</FormField>
	<label class="form-label">
		Location:
		<textarea rows={2} bind:value={prepNotes} class="form-textarea"
			placeholder="Where will the duel take place?"></textarea>
	</label>
	<div class="form-actions">
		<button class="action-btn primary" onclick={submitPrep}
			disabled={prepBusy || prepTargetPlayerID == null}>
			{prepBusy ? '…' : 'Prepare Plan'}
		</button>
	</div>
</div>
