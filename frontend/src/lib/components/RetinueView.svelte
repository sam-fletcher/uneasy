<!--
  Retinue view for any player. Shows the player's assets as tiles with a
  2x2 marginalia sub-grid. The tile is the primary interactive surface —
  step 4 will hook interactions (add/tear marginalia, leverage, steal)
  directly onto the tile parts, gated by phase and viewer identity.

  Step 3: read-only tile rendering.
-->
<script lang="ts">
	import type { Asset, Player, PresenceMember, Marginalium } from '$lib/api';

	let {
		playerId,
		players,
		members,
		assets,
		viewerPlayerId,
	}: {
		playerId: number;
		players: Player[];
		members: PresenceMember[];
		assets: Asset[];
		viewerPlayerId: number | null;
	} = $props();

	const player = $derived(players.find(p => p.id === playerId) ?? null);
	const presence = $derived(members.find(m => m.id === playerId) ?? null);
	const isSelf = $derived(viewerPlayerId === playerId);

	const ownedAssets = $derived(
		assets.filter(a => a.owner_id === playerId && !a.is_destroyed)
	);

	const assetTypeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	// Build a 4-slot marginalia array (filled by position, padded with null).
	function slotsFor(asset: Asset): (Marginalium | null)[] {
		const slots: (Marginalium | null)[] = [null, null, null, null];
		for (const m of asset.marginalia) {
			if (m.position >= 0 && m.position < 4) slots[m.position] = m;
		}
		return slots;
	}
</script>

<div class="retinue-view">
	{#if player}
		<header class="retinue-header">
			<h2>{isSelf ? 'Your Retinue' : `${player.display_name}'s Retinue`}</h2>
			<div class="meta">
				<span class="dot" class:online={presence?.online}></span>
				<span class="status">{presence?.online ? 'online' : 'offline'}</span>
				{#if player.is_facilitator}
					<span class="tag">facilitator</span>
				{/if}
			</div>
		</header>

		{#if ownedAssets.length === 0}
			<p class="empty">No assets yet.</p>
		{:else}
			<ul class="asset-grid">
				{#each ownedAssets as asset (asset.id)}
					<li
						class="asset-tile"
						class:main-char={asset.is_main_character}
						class:leveraged={asset.is_leveraged}
					>
						<div class="tile-head">
							<span class="asset-name">
								{asset.name}
								{#if asset.is_main_character}<span class="main-badge">★</span>{/if}
							</span>
							<span class="asset-type">{assetTypeLabels[asset.asset_type]}</span>
						</div>
						<div class="m-grid">
							{#each slotsFor(asset) as slot, i (i)}
								{#if slot}
									<div class="m-tile" class:torn={slot.is_torn}>
										<span class="m-tile-text">{slot.text}</span>
									</div>
								{:else}
									<div class="m-tile empty" aria-label="empty marginalia slot"></div>
								{/if}
							{/each}
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	{:else}
		<p class="empty">Player not found.</p>
	{/if}
</div>

<style>
	.retinue-view {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}

	.retinue-header h2 {
		color: #c8a96e;
		font-size: 1.1rem;
		margin: 0 0 0.3rem;
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8rem;
		color: #888;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
	}
	.dot.online { background: #6dbf7a; }

	.tag {
		font-size: 0.7rem;
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.empty {
		color: #777;
		font-size: 0.9rem;
		font-style: italic;
		margin: 0;
	}

	/* ── Asset tiles ─────────────────────────────────────────────────────── */

	.asset-grid {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.asset-tile {
		background: #242420;
		border: 1px solid #444;
		border-radius: 8px;
		padding: 0.6rem 0.7rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.asset-tile.main-char { border-color: #c8a96e; }
	.asset-tile.leveraged { border-color: #6090c8; opacity: 0.78; }

	.tile-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.asset-name {
		font-weight: 600;
		font-size: 0.95rem;
		color: #e8e4d9;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.main-badge {
		font-size: 0.7rem;
		background: #4a3010;
		color: #e8c080;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.asset-type {
		font-size: 0.7rem;
		background: #3a3020;
		color: #c8a96e;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		flex-shrink: 0;
	}

	/* ── Marginalia 2×2 grid ─────────────────────────────────────────────── */

	.m-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.35rem;
	}

	.m-tile {
		min-height: 44px;
		padding: 0.35rem 0.45rem;
		background: #1d1d1a;
		border: 1px solid #383530;
		border-radius: 5px;
		font-size: 0.78rem;
		line-height: 1.25;
		color: #cfcabd;
		display: flex;
		align-items: center;
		overflow: hidden;
	}
	.m-tile.empty {
		background: transparent;
		border: 1px dashed #3a3a36;
	}
	.m-tile.torn {
		opacity: 0.45;
		text-decoration: line-through;
	}

	.m-tile-text {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		word-break: break-word;
	}
</style>
