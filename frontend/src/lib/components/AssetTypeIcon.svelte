<!--
	AssetTypeIcon.svelte

	Compact glyph for an asset's type, used where horizontal space is tight
	(the AssetCardSelectable picker rows). The full type word stays in the
	Retinue panel; here the icon plus a tooltip/aria-label carry the meaning.

	Line icons in the house style: 24×24 viewBox, stroke=currentColor,
	stroke-width 2, round caps/joins (paths from Tabler Icons, MIT). They
	inherit colour from the parent and a caller-set width/height.
-->
<script lang="ts">
	import type { Asset } from '$lib/api';

	let { type, size = 16 }: { type: Asset['asset_type']; size?: number } = $props();

	const typeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};
</script>

<span class="type-icon" title={typeLabels[type]} aria-label={typeLabels[type]} role="img">
	<svg
		viewBox="0 0 24 24"
		width={size}
		height={size}
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if type === 'peer'}
			<path d="M8 7a4 4 0 1 0 8 0a4 4 0 0 0 -8 0" />
			<path d="M6 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2" />
		{:else if type === 'holding'}
			<path d="M15 19v-2a3 3 0 0 0 -6 0v2a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1v-14h4v3h3v-3h4v3h3v-3h4v14a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1z" />
			<path d="M3 11l18 0" />
		{:else if type === 'artifact'}
			<path d="M20 4v5l-9 7l-4 4l-3 -3l4 -4l7 -9z" />
			<path d="M6.5 11.5l6 6" />
		{:else}
			<path d="M9 14c0 1.657 2.686 3 6 3s6 -1.343 6 -3s-2.686 -3 -6 -3s-6 1.343 -6 3z" />
			<path d="M9 14v4c0 1.656 2.686 3 6 3s6 -1.344 6 -3v-4" />
			<path d="M3 6c0 1.072 1.144 2.062 3 2.598s4.144 .536 6 0c1.856 -.536 3 -1.526 3 -2.598c0 -1.072 -1.144 -2.062 -3 -2.598s-4.144 -.536 -6 0c-1.856 .536 -3 1.526 -3 2.598z" />
			<path d="M3 6v10c0 .888 .772 1.45 2 2" />
			<path d="M3 11c0 .888 .772 1.45 2 2" />
		{/if}
	</svg>
</span>

<style>
	.type-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		color: var(--color-text);
	}
</style>
