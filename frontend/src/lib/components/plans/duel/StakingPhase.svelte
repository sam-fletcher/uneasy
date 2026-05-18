<!-- Duel/StakingPhase.svelte
  Each duelist selects exactly N peer assets (their submitted stake_count)
  to wager. The backend tucks a hidden die under each. Only unleveraged
  peers are eligible per the rules.
-->
<script lang="ts">
	import { selectStakes, type Asset, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName } from '../shared';
	import { stakeLabel, type DuelRes } from './shared';
	import type { DuelBout } from '$lib/api';

	let { plan, duelRes, players, assets, currentPlayerID, amParticipant, amPreparer, amTarget, myStakes, bouts, onPlansChanged, onRefresh }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		amParticipant: boolean;
		amPreparer: boolean;
		amTarget: boolean;
		myStakes: DuelStake[];
		bouts: DuelBout[];
		onPlansChanged: () => void;
		onRefresh: () => Promise<void> | void;
	} = $props();

	const myStakeCount = $derived(
		amPreparer ? duelRes.prepStakeCount
		: amTarget ? duelRes.targStakeCount
		: 0,
	);
	const iHaveStaked = $derived(myStakes.length > 0);

	const myStakeableAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed
				&& !a.is_leveraged),
	);

	let stakeSelectionIDs = $state<number[]>([]);
	let stakeSubmitBusy = $state(false);
	let stakeSubmitError = $state('');

	async function submitStakes() {
		if (stakeSubmitBusy) return;
		if (stakeSelectionIDs.length !== myStakeCount) {
			stakeSubmitError = `Pick exactly ${myStakeCount} asset${myStakeCount === 1 ? '' : 's'}.`;
			return;
		}
		stakeSubmitBusy = true; stakeSubmitError = '';
		try {
			await selectStakes(plan.id, stakeSelectionIDs);
			stakeSelectionIDs = [];
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			stakeSubmitError = e instanceof Error ? e.message : 'Could not select stakes.';
		} finally { stakeSubmitBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">
		Selecting stakes
		({playerName(players, plan.preparer_id)}: {duelRes.prepStakeCount},
		{playerName(players, plan.target_player_id)}: {duelRes.targStakeCount})
	</p>

	{#if amParticipant}
		{#if iHaveStaked}
			<p class="choices-note">
				You've staked {myStakes.length} asset{myStakes.length === 1 ? '' : 's'}.
				Waiting for your opponent…
			</p>
			<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
				{#each myStakes as s}
					<li>{stakeLabel(s, assets, bouts)}</li>
				{/each}
			</ul>
		{:else}
			<p class="choices-note">
				Pick exactly {myStakeCount} peer asset{myStakeCount === 1 ? '' : 's'} to stake.
				A hidden die will be tucked under each.
			</p>
			<CardPicker
				label="Pick {myStakeCount} peer{myStakeCount === 1 ? '' : 's'} to stake"
				items={myStakeableAssets}
				{players}
				emptyMessage="You have no unleveraged peers available."
				multi
				max={myStakeCount}
				selectedMulti={stakeSelectionIDs}
				onSelectMulti={(ids) => (stakeSelectionIDs = ids)}
			/>
			{#if myStakeableAssets.length > 0}
				{#if stakeSubmitError}<p class="res-error">{stakeSubmitError}</p>{/if}
				<button class="action-btn primary"
					onclick={submitStakes}
					disabled={stakeSubmitBusy || stakeSelectionIDs.length !== myStakeCount}>
					{stakeSubmitBusy ? '…' : `Stake ${stakeSelectionIDs.length}/${myStakeCount}`}
				</button>
			{/if}
		{/if}
	{:else}
		<p class="choices-note muted">The duelists are selecting their stakes.</p>
	{/if}
</div>
