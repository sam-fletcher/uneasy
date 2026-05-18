<!-- Duel/BoutsPhase.svelte
  Bout loop: side-by-side stake columns with per-side accumulated dice,
  the latest bout summary once resolved, and the current actor's
  declare/respond picker. Ties accumulate into a pending pool that goes
  to the winner of the next non-tie bout (backend mirrors this logic).
-->
<script lang="ts">
	import { boutDeclare, boutRespond, type Asset, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import FormField from '../FormField.svelte';
	import { playerName } from '../shared';
	import { stakeLabel, computeAccumulated, type DuelRes } from './shared';

	let { plan, duelRes, players, assets, currentPlayerID, amParticipant, preparerStakes, targetStakes, bouts, myUnresolvedStakes, onPlansChanged, onRefresh }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		amParticipant: boolean;
		preparerStakes: DuelStake[];
		targetStakes: DuelStake[];
		bouts: DuelBout[];
		myUnresolvedStakes: DuelStake[];
		onPlansChanged: () => void;
		onRefresh: () => Promise<void> | void;
	} = $props();

	const accumulated = $derived(computeAccumulated(bouts, plan.preparer_id));

	const latestBout = $derived(bouts.length === 0 ? null : bouts[bouts.length - 1]);
	const boutInProgress = $derived(latestBout != null && latestBout.resolved_at == null);

	const currentActorID = $derived(
		boutInProgress ? (latestBout?.responder_id ?? null)
			: duelRes.initiativeID,
	);
	const isMyTurn = $derived(
		amParticipant && currentPlayerID != null && currentActorID === currentPlayerID,
	);

	let pickedStakeID = $state<number | null>(null);

	const boutStakeAssets = $derived(
		myUnresolvedStakes
			.map(s => assets.find(a => a.id === s.asset_id))
			.filter((a): a is NonNullable<typeof a> => a != null),
	);
	const pickedStakeAssetID = $derived(
		myUnresolvedStakes.find(s => s.id === pickedStakeID)?.asset_id ?? null,
	);
	function pickBoutStakeByAssetID(assetID: number | null) {
		const s = assetID == null ? null : myUnresolvedStakes.find(x => x.asset_id === assetID);
		pickedStakeID = s?.id ?? null;
	}
	function boutStakeLabel(a: { id: number }): string {
		const s = myUnresolvedStakes.find(x => x.asset_id === a.id);
		return s?.hidden_die != null ? `hidden d${s.hidden_die}` : 'hidden';
	}

	let pickedDeclaration = $state<'high' | 'low'>('high');
	let boutBusy = $state(false);
	let boutError = $state('');

	async function submitDeclare() {
		if (boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutDeclare(plan.id, pickedStakeID, pickedDeclaration);
			pickedStakeID = null;
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not declare bout.';
		} finally { boutBusy = false; }
	}
	async function submitRespond() {
		if (boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutRespond(plan.id, pickedStakeID);
			pickedStakeID = null;
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not respond.';
		} finally { boutBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">
		Bout {duelRes.currentBout + (boutInProgress ? 0 : 1)}
		· to act: <strong>{playerName(players, currentActorID)}</strong>
		{#if boutInProgress}(responding){:else}(declaring){/if}
	</p>

	<div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin:0.5rem 0;">
		<div>
			<p class="choices-note"><strong>{playerName(players, plan.preparer_id)}</strong></p>
			<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
				{#each preparerStakes as s}
					<li class:muted={s.is_resolved}>{stakeLabel(s, assets, bouts)}</li>
				{/each}
			</ul>
			<p class="choices-note">
				accumulated dice: {accumulated.prep.length === 0
					? '—'
					: accumulated.prep.join(', ')}
			</p>
		</div>
		<div>
			<p class="choices-note"><strong>{playerName(players, plan.target_player_id)}</strong></p>
			<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
				{#each targetStakes as s}
					<li class:muted={s.is_resolved}>{stakeLabel(s, assets, bouts)}</li>
				{/each}
			</ul>
			<p class="choices-note">
				accumulated dice: {accumulated.targ.length === 0
					? '—'
					: accumulated.targ.join(', ')}
			</p>
		</div>
	</div>

	{#if accumulated.pending.length > 0}
		<p class="choices-note">
			Pending tied dice (go to next bout winner): {accumulated.pending.join(', ')}
		</p>
	{/if}

	{#if latestBout && latestBout.resolved_at != null}
		<p class="choices-note">
			Last bout:
			{playerName(players, latestBout.declarer_id)}
			declared <strong>{latestBout.declaration}</strong>
			({latestBout.declarer_die}) vs
			{playerName(players, latestBout.responder_id)} ({latestBout.responder_die})
			{#if latestBout.is_match}
				→ tie, dice set aside
			{:else if latestBout.winner_id != null}
				→ <strong>{playerName(players, latestBout.winner_id)}</strong> wins
			{/if}
		</p>
	{/if}

	{#if isMyTurn && myUnresolvedStakes.length > 0}
		<div class="plan-form" style="margin-top:0.5rem;">
			<p class="choices-note">
				{boutInProgress ? 'Pick one of your stakes to respond.' : 'Pick a stake and declare high or low.'}
			</p>
			<CardPicker
				label="Pick a stake"
				items={boutStakeAssets}
				{players}
				ownerLabel={boutStakeLabel}
				selected={pickedStakeAssetID}
				onSelect={pickBoutStakeByAssetID}
			/>
			{#if !boutInProgress}
				<FormField label="Declare">
					<div class="chip-row">
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'high'}
							onclick={() => (pickedDeclaration = 'high')}
						>High</button>
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'low'}
							onclick={() => (pickedDeclaration = 'low')}
						>Low</button>
					</div>
				</FormField>
			{/if}
			{#if boutError}<p class="res-error">{boutError}</p>{/if}
			<button class="action-btn primary"
				onclick={boutInProgress ? submitRespond : submitDeclare}
				disabled={boutBusy || pickedStakeID == null}>
				{boutBusy ? '…' : boutInProgress ? 'Respond' : 'Declare'}
			</button>
		</div>
	{:else if amParticipant}
		<p class="choices-note muted">
			Waiting for {playerName(players, currentActorID)}…
		</p>
	{/if}
</div>
