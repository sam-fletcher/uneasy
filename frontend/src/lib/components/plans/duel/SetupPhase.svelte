<!-- Duel/SetupPhase.svelte
  Setup phase: champion election (initiative-gated) + secret stake-count
  reveal. Both sub-blocks are visible at once; the participant interacts
  with whichever step they haven't completed.
-->
<script lang="ts">
	import { electChampion, stakeReveal, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, assetName } from '../shared';
	import type { DuelRes } from './shared';

	let { plan, duelRes, players, assets, currentPlayerID, amParticipant, amPreparer, amTarget, myMaxStakes, onPlansChanged }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		amParticipant: boolean;
		amPreparer: boolean;
		amTarget: boolean;
		myMaxStakes: number;
		onPlansChanged: () => void;
	} = $props();

	// Peers I own, available as champion.
	const myPeerAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed),
	);

	const iHaveChampionDeclared = $derived(
		amPreparer ? duelRes.prepChampDeclared
		: amTarget ? duelRes.targChampDeclared
		: false,
	);
	const iHaveInitiative = $derived(
		currentPlayerID != null && duelRes.initiativeID === currentPlayerID,
	);
	const initiativeDeclared = $derived(
		duelRes.initiativeID === plan.preparer_id ? duelRes.prepChampDeclared
			: duelRes.initiativeID === plan.target_player_id ? duelRes.targChampDeclared
			: false,
	);
	const canElectNow = $derived(
		amParticipant && !iHaveChampionDeclared
		&& (iHaveInitiative || initiativeDeclared),
	);

	let championAssetID = $state<number | null>(null);
	let championBusy = $state(false);
	let championError = $state('');

	async function submitChampion(assetID: number | null) {
		if (championBusy) return;
		championBusy = true; championError = '';
		try {
			await electChampion(plan.id, assetID);
			championAssetID = null;
			onPlansChanged();
		} catch (e) {
			championError = e instanceof Error ? e.message : 'Could not elect champion.';
		} finally { championBusy = false; }
	}

	let stakeCountPicked = $state<number | null>(null);
	let stakeCountBusy = $state(false);
	let stakeCountError = $state('');
	const iSubmittedStakeCount = $derived(
		currentPlayerID != null && duelRes.stakeCounts[currentPlayerID] != null,
	);
	async function submitStakeCount() {
		if (stakeCountPicked == null || stakeCountBusy) return;
		stakeCountBusy = true; stakeCountError = '';
		try {
			await stakeReveal(plan.id, stakeCountPicked);
			onPlansChanged();
		} catch (e) {
			stakeCountError = e instanceof Error ? e.message : 'Could not submit stake count.';
		} finally { stakeCountBusy = false; }
	}
</script>

<!-- Champions -->
<div class="choices-section">
	<p class="choices-header">Champions</p>
	<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
		<li>
			{playerName(players, plan.preparer_id)}:
			{#if duelRes.prepChampDeclared}
				{#if duelRes.prepChampID != null}
					fights through <strong>{assetName(assets, duelRes.prepChampID)}</strong>
				{:else}
					fights in person
				{/if}
			{:else}
				<span class="muted">(not yet declared)</span>
			{/if}
		</li>
		<li>
			{playerName(players, plan.target_player_id)}:
			{#if duelRes.targChampDeclared}
				{#if duelRes.targChampID != null}
					fights through <strong>{assetName(assets, duelRes.targChampID)}</strong>
				{:else}
					fights in person
				{/if}
			{:else}
				<span class="muted">(not yet declared)</span>
			{/if}
		</li>
	</ul>

	{#if amParticipant && !iHaveChampionDeclared}
		{#if canElectNow}
			<div class="plan-form" style="margin-top:0.5rem;">
				<p class="choices-note">
					{iHaveInitiative
						? 'You have initiative — choose first.'
						: 'Your opponent has declared. Make your choice.'}
				</p>
				<CardPicker
					label="Pick a champion"
					items={myPeerAssets}
					{players}
					emptyMessage="You have no peers available as champion."
					selected={championAssetID}
					onSelect={(id) => (championAssetID = id)}
				/>
				{#if championError}<p class="res-error">{championError}</p>{/if}
				<div class="form-row">
					<button class="action-btn primary"
						onclick={() => submitChampion(championAssetID)}
						disabled={championBusy || championAssetID == null}>
						{championBusy ? '…' : 'Elect as champion'}
					</button>
					<button class="action-btn"
						onclick={() => submitChampion(null)}
						disabled={championBusy}>
						Fight yourself
					</button>
				</div>
			</div>
		{:else}
			<p class="choices-note muted" style="margin-top:0.5rem;">
				Waiting for {playerName(players, duelRes.initiativeID)} to declare first.
			</p>
		{/if}
	{/if}
</div>

<!-- Stake-count reveal -->
<div class="choices-section">
	<p class="choices-header">Stake count</p>
	<p class="choices-note">
		Each duelist secretly commits to a number of assets to stake
		(min 1, max {myMaxStakes} for you). Revealed once both submit.
	</p>
	{#if amParticipant}
		{#if iSubmittedStakeCount}
			<p class="choices-note">You've submitted. Waiting for your opponent…</p>
		{:else}
			<div class="chip-row" style="margin:0.5rem 0;">
				{#each Array.from({ length: myMaxStakes }, (_, i) => i + 1) as n}
					<button
						type="button"
						class="chip-btn"
						class:active={stakeCountPicked === n}
						onclick={() => (stakeCountPicked = n)}
					>
						{n}
					</button>
				{/each}
			</div>
			{#if stakeCountError}<p class="res-error">{stakeCountError}</p>{/if}
			<button class="action-btn primary"
				onclick={submitStakeCount}
				disabled={stakeCountBusy || stakeCountPicked == null}>
				{stakeCountBusy ? '…' : 'Submit stake count'}
			</button>
		{/if}
	{:else}
		<p class="choices-note muted">
			{duelRes.prepStakeCount > 0 && duelRes.targStakeCount > 0
				? `Counts revealed: ${playerName(players, plan.preparer_id)} ${duelRes.prepStakeCount}, `
				  + `${playerName(players, plan.target_player_id)} ${duelRes.targStakeCount}.`
				: 'Counts not yet revealed.'}
		</p>
	{/if}
</div>
