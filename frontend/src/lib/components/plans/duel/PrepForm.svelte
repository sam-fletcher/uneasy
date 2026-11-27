<!-- Duel/PrepForm.svelte
  Prep: pick a challenger, choose arms vs wits, describe the location.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { onDestroy } from 'svelte';
	import { preparePlan } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import FormField from '../FormField.svelte';
	import DuelTermsFields from './DuelTermsFields.svelte';
	import { playersExcept } from '../shared';
	import type { PlanContext } from '../types';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { ctx }: { ctx: PlanContext } = $props();

	const gameID = $derived(ctx.gameID);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const focusPlayerID = $derived(ctx.focusPlayerID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);
	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		target_player_id?: number | null;
		duel_type?: 'arms' | 'wits';
		notes?: string;
	} | null);

	let prepTargetPlayerID = $state<number | null>(null);
	let prepDuelType = $state<'arms' | 'wits'>('arms');
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	// The player whose prep form this is. The preparer's own form is the viewer's;
	// a watcher's read-only mirror is the focus player's, so the chips are the ones
	// the preparer chooses from, not the watcher's.
	const draftPreparerID = $derived(readOnly ? focusPlayerID : currentPlayerID);
	const otherPlayers = $derived(playersExcept(players, draftPreparerID));

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

	$effect(() => {
		if (!readOnly) return;
		prepTargetPlayerID = prepDraft?.target_player_id ?? null;
		prepDuelType = prepDraft?.duel_type ?? 'arms';
		prepNotes = prepDraft?.notes ?? '';
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly) return;
		void prepTargetPlayerID; void prepDuelType; void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({
				target_player_id: prepTargetPlayerID,
				duel_type: prepDuelType,
				notes: prepNotes,
			});
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });
</script>

<fieldset class="plan-form-fieldset" disabled={readOnly}>
	<div class="plan-form">
		{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
		<FormField label="Challenger">
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => prepTargetPlayerID === p.id}
				onSelect={(p) => (prepTargetPlayerID = prepTargetPlayerID === p.id ? null : p.id)}
				{readOnly}
			/>
		</FormField>
		<DuelTermsFields bind:duelType={prepDuelType} bind:place={prepNotes} />
		{#if !readOnly}
			<div class="form-actions">
				<button class="action-btn primary" onclick={submitPrep}
					disabled={prepBusy || prepTargetPlayerID == null || !prepNotes.trim()}>
					{prepBusy ? '…' : 'Prepare Plan'}
				</button>
			</div>
		{/if}
	</div>
</fieldset>
