<!-- MakeWar/SurrenderClaims.svelte
  Per-claim asset pickers for surrendered opponents' remaining assets.
  Only renders when the current player holds at least one open claim.
-->
<script lang="ts">
	import EndsNote from '../shared/EndsNote.svelte';
	import '$lib/components/shared/actionButton.css';
	import { takeSurrenderAsset, type Asset, type Player, type WarStateResponse } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName } from '../shared';

	type Claim = WarStateResponse['open_claims'][number];

	let { claims, planID, players, assets, onChanged, setError }: {
		claims: Claim[];
		planID: number;
		players: Player[];
		assets: Asset[];
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	let claimAssetByClaim = $state<Record<number, number | null>>({});
	let claimBusy = $state(false);

	function targetAssetsFor(surrenderedID: number): Asset[] {
		return assets.filter(a => a.owner_id === surrenderedID && !a.is_destroyed);
	}

	async function submitClaim(claimID: number, surrenderedID: number) {
		if (claimBusy) return;
		const assetID = claimAssetByClaim[claimID];
		if (assetID == null) return;
		claimBusy = true; setError('');
		try {
			await takeSurrenderAsset(planID, surrenderedID, assetID);
			claimAssetByClaim = { ...claimAssetByClaim, [claimID]: null };
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not take the asset.');
		} finally { claimBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">Spoils of war</p>
	{#each claims as c (c.id)}
		{@const claimable = targetAssetsFor(c.surrendered_id)}
		{@const picked = claimAssetByClaim[c.id]}
		<div class="claim">
			<CardPicker
				label={`${playerName(players, c.surrendered_id)} surrendered. Pick one of their assets to take`}
				items={claimable}
				{players}
				emptyMessage="No assets left to take."
				selected={picked ?? null}
				onSelect={(id) => (claimAssetByClaim = { ...claimAssetByClaim, [c.id]: id })}
			/>
			<button class="action-btn primary"
				onclick={() => submitClaim(c.id, c.surrendered_id)}
				disabled={claimBusy || claimAssetByClaim[c.id] == null}>
				{claimBusy ? '…' : 'Take'}
			</button>
			<EndsNote text="Play goes on once every battle cost is paid and every surrender's spoils are taken." />
		</div>
	{/each}
</div>

<style>
	.claim {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
</style>
