<!--
	AssetCardSelectable.svelte

	Standardized expandable asset card for places where the player needs to
	read or pick assets (scene setup, scene details, future plan UI). Comes
	in two variants:

	- Display-only (default): name + type, expand to read marginalia.
	- Selectable: tap target toggles a checkmark; the card grows a coloured
	  border in the owner's player color when selected. Use the
	  `selected`/`onToggle` props (multi-select) or pair `selected` with a
	  parent that clears other cards before flipping (single-select).

	Owner color is provided by the caller via `ownerColor` so this component
	stays decoupled from the player list. The caller resolves the color via
	`playerColor()` from $lib/playerColor.

	The component is intentionally small: it doesn't mutate the asset and
	doesn't know about leverage / tearing. For those, see AssetCard.svelte.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import type { Asset } from '$lib/api';

	interface Props {
		asset: Asset;
		/** Hex color to use for the owner's dot and the selected border. */
		ownerColor: string;
		/** Subtitle to render under the name (e.g. "Owned by Alice"). Optional. */
		ownerLabel?: string;
		/** When false, the card has no checkmark or selected state. */
		selectable?: boolean;
		selected?: boolean;
		onToggle?: (asset: Asset) => void;
		/** Disable interaction (e.g. peer already claimed by someone else). */
		disabled?: boolean;
		/** Render expanded by default. */
		defaultExpanded?: boolean;
	}

	let {
		asset,
		ownerColor,
		ownerLabel,
		selectable = false,
		selected = false,
		onToggle,
		disabled = false,
		defaultExpanded = false,
	}: Props = $props();

	// `defaultExpanded` is intentionally a one-shot initial-value hint, not a
	// live binding — once the user toggles the card we want their state to
	// stick even if the parent passes a new default. `untrack` makes that
	// intent explicit and silences the state_referenced_locally warning.
	let expanded = $state(untrack(() => defaultExpanded));

	const typeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	function toggleExpand(e: MouseEvent) {
		// Don't toggle expand when the click came from the select tap area.
		if ((e.target as HTMLElement).closest('.select-tap')) return;
		expanded = !expanded;
	}

	function handleSelect() {
		if (disabled) return;
		onToggle?.(asset);
	}

	const liveMarginalia = $derived(asset.marginalia.filter(m => !m.is_torn));
	const tornCount = $derived(asset.marginalia.length - liveMarginalia.length);
</script>

<div
	class="card"
	class:selectable
	class:selected={selectable && selected}
	class:disabled
	style:--owner-color={ownerColor}
>
	<button
		type="button"
		class="header"
		onclick={toggleExpand}
		aria-expanded={expanded}
	>
		{#if selectable}
			<span
				class="select-tap"
				role="checkbox"
				tabindex={disabled ? -1 : 0}
				aria-checked={selected}
				aria-disabled={disabled}
				onclick={(e) => { e.stopPropagation(); handleSelect(); }}
				onkeydown={(e) => {
					if (e.key === ' ' || e.key === 'Enter') {
						e.preventDefault();
						handleSelect();
					}
				}}
			>
				<span class="check">{selected ? '✓' : ''}</span>
			</span>
		{/if}

		<span class="dot" aria-hidden="true"></span>

		<span class="name-block">
			<span class="name">
				{asset.name}
				{#if asset.is_main_character}
					<span class="main-badge" title="Main character">★</span>
				{/if}
			</span>
			{#if ownerLabel}
				<span class="owner-label">{ownerLabel}</span>
			{/if}
		</span>

		<span class="meta">
			<span class="type">{typeLabels[asset.asset_type]}</span>
			<span class="count">
				{liveMarginalia.length}{tornCount > 0 ? ` / ${asset.marginalia.length}` : ''}
			</span>
			<span class="caret" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
		</span>
	</button>

	{#if expanded}
		<div class="body">
			{#if asset.marginalia.length === 0}
				<p class="empty">No marginalia recorded.</p>
			{:else}
				<ul class="marginalia">
					{#each asset.marginalia as m (m.id)}
						<li class:torn={m.is_torn}>
							{#if m.is_torn}
								<span class="torn-mark" aria-label="torn">✗</span>
							{:else}
								<span class="bullet" aria-hidden="true">•</span>
							{/if}
							<span class="m-text">{m.text}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</div>

<style>
	.card {
		border: 1px solid #2a2a2a;
		border-left: 3px solid var(--owner-color, #444);
		border-radius: 5px;
		background: #1d1d1d;
		overflow: hidden;
	}

	.card.selectable.selected {
		border-color: var(--owner-color, #c8a96e);
		background: #221d10;
	}

	.card.disabled {
		opacity: 0.5;
	}

	.header {
		display: grid;
		grid-template-columns: auto auto 1fr auto;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.55rem 0.65rem;
		background: none;
		border: none;
		text-align: left;
		color: inherit;
		cursor: pointer;
		min-height: 44px; /* tap target */
	}

	.card.selectable .header { grid-template-columns: auto auto auto 1fr auto; }
	.card:not(.selectable) .header { grid-template-columns: auto 1fr auto; }

	.select-tap {
		width: 22px;
		height: 22px;
		border: 1px solid var(--owner-color, #555);
		border-radius: 4px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		cursor: pointer;
	}

	.select-tap:focus { outline: 2px solid var(--owner-color, #c8a96e); outline-offset: 1px; }

	.card.selectable.selected .select-tap {
		background: var(--owner-color, #c8a96e);
	}

	.check {
		color: #1a1a1a;
		font-size: 0.85rem;
		font-weight: 700;
		line-height: 1;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--owner-color, #555);
		flex-shrink: 0;
	}

	.name-block {
		display: flex;
		flex-direction: column;
		min-width: 0;
		gap: 0.05rem;
	}

	.name {
		font-size: 0.92rem;
		color: #e8e4d9;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.owner-label {
		font-size: 0.72rem;
		color: #888;
	}

	.main-badge {
		color: var(--owner-color, #c8a96e);
		font-size: 0.78rem;
		margin-left: 0.2rem;
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.72rem;
		color: #888;
		flex-shrink: 0;
	}

	.type { text-transform: uppercase; letter-spacing: 0.05em; }

	.count {
		font-variant-numeric: tabular-nums;
		color: #aaa;
	}

	.caret { font-size: 0.8rem; color: #888; }

	.body {
		padding: 0 0.7rem 0.6rem 1rem;
		border-top: 1px dashed #2a2a2a;
	}

	.marginalia {
		list-style: none;
		margin: 0.4rem 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.85rem;
		color: #d8d4c9;
	}

	.marginalia li {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
	}

	.marginalia li.torn { color: #777; text-decoration: line-through; }

	.bullet { color: var(--owner-color, #888); }
	.torn-mark { color: #a05050; font-size: 0.78rem; }

	.empty {
		font-size: 0.82rem;
		color: #666;
		margin: 0.4rem 0;
		font-style: italic;
	}
</style>
