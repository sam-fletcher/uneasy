<!-- MakeWar/PrepForm.svelte
  Declare-war prep: pick one or more enemies + notes (required).
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

	let enemyIDs = $state<Set<number>>(new Set());
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));

	function toggleEnemy(id: number) {
		const next = new Set(enemyIDs);
		if (next.has(id)) next.delete(id); else next.add(id);
		enemyIDs = next;
	}

	async function submitPrep() {
		if (prepBusy) return;
		if (enemyIDs.size === 0) { prepError = 'Pick at least one enemy.'; return; }
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'make_war',
				enemy_player_ids: [...enemyIDs],
				preparation_notes: prepNotes.trim(),
			});
			enemyIDs = new Set();
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}
</script>

<div class="plan-form">
	{#if prepError}<p class="res-error">{prepError}</p>{/if}
	<FormField label="Declare war on (one or more)">
		<PlayerChips
			players={otherPlayers}
			isActive={(p) => enemyIDs.has(p.id)}
			onSelect={(p) => toggleEnemy(p.id)}
		/>
	</FormField>
	<label class="form-label">
		Motivation:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="Casus belli, opening move, rally cry, et cetera" required></textarea>
	</label>
	<p class="choices-note muted">
		Once declared, all involved players reveal a die to set the delay (average rounded up).
		Other players may join either side whenever the Public Record advances.
	</p>
	<div class="form-actions">
		<button class="action-btn primary" onclick={submitPrep}
			disabled={prepBusy || enemyIDs.size === 0 || !prepNotes.trim()}>
			{prepBusy ? '…' : 'Prepare Plan'}
		</button>
	</div>
</div>
