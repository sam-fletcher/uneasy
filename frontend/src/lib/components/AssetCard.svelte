<script lang="ts">
	import type { Asset, Marginalium } from '$lib/api';

	const assetTypeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	let {
		asset,
		compact = false,
		mode = 'default',
		onTear,
		onToggleLeverage,
		onRollLeverage,
		rollLeverageDisabled = false,
	}: {
		asset: Asset;
		compact?: boolean;
		mode?: 'default' | 'roll-leverage';
		onTear: (asset: Asset, m: Marginalium) => void;
		onToggleLeverage?: (asset: Asset) => void;
		onRollLeverage?: (asset: Asset) => void;
		rollLeverageDisabled?: boolean;
	} = $props();
</script>

<div
	class="asset-card"
	class:compact
	class:main-char={asset.is_main_character && !compact}
	class:leveraged={asset.is_leveraged && compact}
>
	<div class="asset-header">
		<span class="asset-name">
			{asset.name}
			{#if asset.is_main_character}
				<span class="main-badge">{compact ? '★' : '★ main'}</span>
			{/if}
		</span>
		<div class="asset-header-right">
			<span class="asset-type-badge">{assetTypeLabels[asset.asset_type]}</span>
			{#if mode === 'roll-leverage' && onRollLeverage}
				<button
					class="roll-lev-btn"
					onclick={() => onRollLeverage!(asset)}
					disabled={asset.is_leveraged || rollLeverageDisabled}
					aria-label="Commit this asset for +1 die"
					title={asset.is_leveraged
						? 'Already leveraged elsewhere'
						: rollLeverageDisabled
							? 'Unready yourself first'
							: 'Commit this asset for +1 die'}
				>
					+<span class="die-icon" aria-hidden="true">🎲</span>
				</button>
			{:else if compact && onToggleLeverage}
				<button
					class="lev-btn"
					class:active={asset.is_leveraged}
					onclick={() => onToggleLeverage!(asset)}
					title={asset.is_leveraged ? 'Refresh (un-leverage)' : 'Leverage'}
				>
					{asset.is_leveraged ? '⊙ leveraged' : '○ leverage'}
				</button>
			{/if}
		</div>
	</div>

	{#if asset.marginalia.length > 0}
		<ul class="marginalia-list">
			{#each asset.marginalia as m (m.id)}
				<li class:torn={m.is_torn}>
					<span class="m-text">{m.text}</span>
					{#if m.is_torn}
						<span class="torn-label">torn</span>
					{:else if mode !== 'roll-leverage'}
						<button class="tear-btn" title="Tear this marginalia" onclick={() => onTear(asset, m)}>
							✂
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{:else if !compact}
		<p class="no-marginalia">No marginalia yet.</p>
	{/if}
</div>

<style>
	.asset-card {
		background: #242420;
		border: 1px solid #444;
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.asset-card.compact {
		padding: 0.4rem 0.6rem;
		gap: 0.3rem;
	}

	.asset-card.main-char  { border-color: #c8a96e; }
	.asset-card.leveraged  { border-color: #6090c8; opacity: 0.75; }

	.asset-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.asset-name {
		font-weight: 600;
		font-size: 0.9rem;
		color: #e8e4d9;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.main-badge {
		font-size: 0.7rem;
		background: #4a3010;
		color: #e8c080;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
	}

	.asset-header-right {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}

	.asset-type-badge {
		font-size: 0.7rem;
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
	}

	.lev-btn {
		font-size: 0.75rem;
		color: #888;
		padding: 0.15rem 0.4rem;
		border: 1px solid #555;
		border-radius: 3px;
	}

	.lev-btn.active {
		color: #6090c8;
		border-color: #6090c8;
	}

	.roll-lev-btn {
		color: #e8e4d9;
		background: #3a3020;
		min-width: 44px;
		min-height: 44px;
		padding: 0 0.45rem;
		border: 1px solid #c8a96e;
		border-radius: 4px;
		font-weight: 700;
		font-size: 0.95rem;
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		line-height: 1;
	}

	.roll-lev-btn .die-icon {
		font-size: 1.1rem;
	}

	.roll-lev-btn:disabled {
		color: #666;
		background: #222;
		border-color: #444;
		cursor: not-allowed;
	}

	.marginalia-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.marginalia-list li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: 0.82rem;
		color: #bbb;
		gap: 0.4rem;
	}

	.asset-card.compact .marginalia-list li { font-size: 0.78rem; }

	.marginalia-list li.torn { opacity: 0.45; }

	.m-text { flex: 1; }

	.tear-btn {
		background: none;
		color: #c07070;
		font-size: 0.8rem;
		padding: 0;
		flex-shrink: 0;
		opacity: 0.6;
	}

	.tear-btn:hover { opacity: 1; }

	.torn-label {
		font-size: 0.7rem;
		color: #666;
		flex-shrink: 0;
	}

	.no-marginalia {
		font-size: 0.8rem;
		color: #666;
		margin: 0;
	}
</style>
