<!-- MakeWar/SurrenderClaims.svelte
  Per-claim asset pickers for surrendered opponents' remaining assets.
  Only renders when the current player holds at least one open claim.
-->
<script lang="ts">
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
			setError(e instanceof Error ? e.message : 'Could not claim asset.');
		} finally { claimBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">Surrender claims</p>
	{#each claims as c (c.id)}
		{@const claimable = targetAssetsFor(c.surrendered_id)}
		{@const picked = claimAssetByClaim[c.id]}
		<div style="display:block;margin-bottom:0.5rem;">
			<CardPicker
				label={`${playerName(players, c.surrendered_id)} surrendered. Pick one of their assets to take`}
				items={claimable}
				{players}
				emptyMessage="No eligible assets to claim."
				selected={picked ?? null}
				onSelect={(id) => (claimAssetByClaim = { ...claimAssetByClaim, [c.id]: id })}
			/>
			<button class="action-btn primary" style="margin-top:0.4rem;"
				onclick={() => submitClaim(c.id, c.surrendered_id)}
				disabled={claimBusy || claimAssetByClaim[c.id] == null}>
				{claimBusy ? '…' : 'Claim'}
			</button>
		</div>
	{/each}
</div>
