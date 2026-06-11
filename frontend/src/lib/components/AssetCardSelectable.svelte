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

	The component is intentionally small: it doesn't mutate the asset or
	tear marginalia. The one exception is leverage during a dice roll — pass
	`leverageMode` to surface a die toggle in the header's left slot (used by
	DiceRollPanel's draft-then-submit flow). The die has three states:
	  - empty outline  → not selected; tap to add it to your draft
	  - filled (owner)  → drafted (leverageDrafted) or already committed
	    (asset.is_leveraged); committed dice are locked
	The parent owns draft state and the actual commit (on Ready).
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import type { Asset } from '$lib/api';
	import { isNeedlesslyAtRisk } from '$lib/assetRisk';

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
		/**
		 * Marginalia-pick mode. Mutually exclusive with `selectable`. The
		 * header checkbox is hidden; the card auto-expands; each intact
		 * marginalia line gets its own checkbox tap target. Torn lines
		 * remain non-interactive. Asset identity is implicit — callers
		 * derive it from the marginalia ID.
		 */
		marginaliaSelectable?: boolean;
		selectedMarginaliaID?: number | null;
		onMarginaliaToggle?: (marginaliaID: number, asset: Asset) => void;
		/** Disable interaction (e.g. peer already claimed by someone else). */
		disabled?: boolean;
		/** Render expanded by default. */
		defaultExpanded?: boolean;
		/**
		 * Leverage draft toggle (DiceRollPanel). When true, a die appears in
		 * the header's left slot. `leverageDrafted` fills it as a pending pick;
		 * an already-committed asset (asset.is_leveraged) shows it filled and
		 * locked. `leverageDisabled` (e.g. you're readied) blocks toggling.
		 */
		leverageMode?: boolean;
		leverageDrafted?: boolean;
		leverageDisabled?: boolean;
		onToggleLeverage?: (asset: Asset) => void;
	}

	let {
		asset,
		ownerColor,
		ownerLabel,
		selectable = false,
		selected = false,
		onToggle,
		marginaliaSelectable = false,
		selectedMarginaliaID = null,
		onMarginaliaToggle,
		disabled = false,
		defaultExpanded = false,
		leverageMode = false,
		leverageDrafted = false,
		leverageDisabled = false,
		onToggleLeverage,
	}: Props = $props();

	// Committed dice are locked; drafted/committed both render filled.
	const leverageCommitted = $derived(asset.is_leveraged);
	const leverageFilled = $derived(leverageCommitted || leverageDrafted);
	const leverageLocked = $derived(leverageCommitted || leverageDisabled);
	function handleLeverage() {
		if (leverageLocked) return;
		onToggleLeverage?.(asset);
	}

	// `defaultExpanded` is intentionally a one-shot initial-value hint, not a
	// live binding — once the user toggles the card we want their state to
	// stick even if the parent passes a new default. `untrack` makes that
	// intent explicit and silences the state_referenced_locally warning.
	let expandedRaw = $state(untrack(() => defaultExpanded));
	// In marginalia-pick mode the card is always expanded: the marginalia
	// list IS the picker, so hiding it would defeat the mode.
	const expanded = $derived(marginaliaSelectable ? true : expandedRaw);

	const typeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	function toggleExpand(e: MouseEvent) {
		// Don't toggle expand when the click came from the select tap area.
		if ((e.target as HTMLElement).closest('.select-tap')) return;
		// Marginalia-pick mode keeps the card open; header is non-collapsing.
		if (marginaliaSelectable) return;
		expandedRaw = !expandedRaw;
	}

	function handleSelect() {
		if (disabled) return;
		onToggle?.(asset);
	}

	function handleMarginaliaSelect(marginaliaID: number) {
		if (disabled) return;
		onMarginaliaToggle?.(marginaliaID, asset);
	}

	const liveMarginalia = $derived(asset.marginalia.filter(m => !m.is_torn));
	const tornCount = $derived(asset.marginalia.length - liveMarginalia.length);

	// One tear from destruction but a slot is still fillable — tint the count
	// and caret red so the fragility reads at a glance. See isNeedlesslyAtRisk.
	const atRisk = $derived(isNeedlesslyAtRisk(asset));
</script>

<div
	class="card"
	class:selectable
	class:selected={selectable && selected}
	class:marginalia-selectable={marginaliaSelectable}
	class:has-leverage={leverageMode && !selectable && !marginaliaSelectable}
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
		{:else if marginaliaSelectable}
			<!-- Placeholder keeps the header grid columns aligned with the
			     selectable variant; the actual checkbox lives on each
			     marginalia line below. -->
			<span class="select-tap-placeholder" aria-hidden="true"></span>
		{:else if leverageMode}
			<!-- Leverage draft toggle. Shares the left "act on this asset"
			     slot with the checkbox; the two modes never appear together. -->
			<span
				class="lev-tap"
				class:filled={leverageFilled}
				class:locked={leverageLocked}
				role="button"
				tabindex={leverageLocked ? -1 : 0}
				aria-pressed={leverageFilled}
				aria-disabled={leverageLocked}
				aria-label="Leverage this asset for +1 die"
				title={leverageCommitted
					? 'Committed to this roll'
					: leverageDisabled
						? 'Unready yourself to change your dice'
						: leverageDrafted
							? 'Selected — commits when you press Ready'
							: 'Select to leverage (+1 die)'}
				onclick={(e) => { e.stopPropagation(); handleLeverage(); }}
				onkeydown={(e) => {
					if (e.key === ' ' || e.key === 'Enter') {
						e.preventDefault();
						e.stopPropagation();
						handleLeverage();
					}
				}}
			>+<span class="die-icon" aria-hidden="true">🎲</span></span>
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
			<span
				class="count"
				class:at-risk={atRisk}
				title={atRisk ? 'Too few notes — one break could destroy this asset' : undefined}
			>
				{liveMarginalia.length}{tornCount > 0 ? ` / ${asset.marginalia.length}` : ''}
			</span>
			<span class="caret" class:at-risk={atRisk} aria-hidden="true">{expanded ? '▾' : '▸'}</span>
		</span>
	</button>

	{#if expanded}
		<div class="body">
			{#if asset.marginalia.length === 0}
				<p class="empty">No marginalia recorded.</p>
			{:else}
				<ul class="marginalia">
					{#each asset.marginalia as m (m.id)}
						{@const isPickable = marginaliaSelectable && !m.is_torn}
						{@const isPicked = isPickable && selectedMarginaliaID === m.id}
						<li class:torn={m.is_torn} class:picked={isPicked}>
							{#if m.is_torn}
								<span class="torn-mark" aria-label="torn">✗</span>
							{:else if marginaliaSelectable}
								<span
									class="select-tap m-tap"
									role="checkbox"
									tabindex={disabled ? -1 : 0}
									aria-checked={isPicked}
									aria-disabled={disabled}
									onclick={(e) => { e.stopPropagation(); handleMarginaliaSelect(m.id); }}
									onkeydown={(e) => {
										if (e.key === ' ' || e.key === 'Enter') {
											e.preventDefault();
											handleMarginaliaSelect(m.id);
										}
									}}
								>
									<span class="check">{isPicked ? '✓' : ''}</span>
								</span>
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
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid var(--owner-color, var(--color-border-strong));
		border-radius: 5px;
		background: #1d1d1d;
		overflow: hidden;
	}

	.card.selectable.selected {
		border-color: var(--owner-color, var(--color-accent));
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

	.card.selectable .header,
	.card.marginalia-selectable .header { grid-template-columns: auto auto auto 1fr auto; }

	/* Header click is a no-op in marginalia-pick mode (the card stays open)
	   so don't suggest a pointer affordance there. */
	.card.marginalia-selectable .header { cursor: default; }
	.card.marginalia-selectable .caret { display: none; }
	.card:not(.selectable):not(.marginalia-selectable):not(.has-leverage) .header { grid-template-columns: auto 1fr auto; }
	/* Leverage mode keeps the base 4-column layout: the "+🎲" button takes the
	   same left "act on this asset" slot the checkbox uses in selectable mode. */

	.select-tap-placeholder {
		width: 22px;
		height: 22px;
		flex-shrink: 0;
	}

	/* Highlight a card that has the picked marginalia, so the user can
	   see at a glance which asset their selection belongs to. */
	.card.marginalia-selectable:has(.marginalia li.picked) {
		border-color: var(--owner-color, var(--color-accent));
		background: #221d10;
	}

	/* The marginalia row's own checkbox + selected state. */
	.marginalia li {
		min-height: 32px;
	}
	.marginalia li.picked {
		color: #ffe8b8;
	}
	.m-tap {
		width: 20px;
		height: 20px;
	}
	.marginalia li.picked .m-tap {
		background: var(--owner-color, var(--color-accent));
	}

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

	.select-tap:focus { outline: 2px solid var(--owner-color, var(--color-accent)); outline-offset: 1px; }

	/* Invisible tap target around the header checkbox: extends the 22px box
	   out to ~44px. Scoped to .header so the tight per-line marginalia
	   checkboxes (.m-tap) keep their own small hit areas and don't overlap
	   neighbouring rows. */
	.header .select-tap { position: relative; }
	.header .select-tap::after { content: ''; position: absolute; inset: -11px; }

	.card.selectable.selected .select-tap {
		background: var(--owner-color, var(--color-accent));
	}

	.check {
		color: var(--color-bg);
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
		color: var(--color-text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.owner-label {
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.main-badge {
		color: var(--owner-color, var(--color-accent));
		font-size: 0.78rem;
		margin-left: 0.2rem;
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.type { text-transform: uppercase; letter-spacing: 0.05em; }

	.count {
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}

	.caret { font-size: 0.8rem; color: var(--color-text-muted); }

	/* Needlessly-at-risk: red count + caret, matching the header-chip risk
	   badge. Title on .count carries the meaning for non-colour users. */
	.count.at-risk { color: #d65a5a; font-weight: 700; }
	.caret.at-risk { color: #d65a5a; }

	/* Leverage draft toggle. A ~36px-tall "+🎲" chip wrapped in a 44px
	   invisible tap target (::after) so it's comfortable to hit without
	   inflating the header. Outlined = not picked; filled (owner colour) =
	   drafted or already committed. Sits in the same left slot the checkbox
	   uses. */
	.lev-tap {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.05rem;
		height: 36px;
		padding: 0 0.4rem;
		border: 1px solid var(--owner-color, var(--color-accent));
		border-radius: 4px;
		color: var(--color-text);
		font-weight: 700;
		font-size: 1rem;
		line-height: 1;
		flex-shrink: 0;
		cursor: pointer;
	}
	/* Match the 🎲 glyph in the dashed Actor-pool / Interference boxes (1.1rem). */
	.lev-tap .die-icon { font-size: 1.1rem; }
	/* Invisible tap target: extends the 36px chip out to ~44px on all sides. */
	.lev-tap::after { content: ''; position: absolute; inset: -4px; }
	.lev-tap:focus-visible { outline: 2px solid var(--owner-color, var(--color-accent)); outline-offset: 1px; }
	/* Drafted or committed → filled with owner colour. */
	.lev-tap.filled { background: var(--owner-color, var(--color-accent)); color: var(--color-bg); }
	/* Locked: committed (filled, dimmed) or readied-empty (faint outline). */
	.lev-tap.locked { cursor: not-allowed; }
	.lev-tap.locked.filled { opacity: 0.6; }
	.lev-tap.locked:not(.filled) {
		border-color: var(--color-border-strong);
		color: var(--color-text-faint);
		opacity: 0.55;
	}

	.body {
		padding: 0 0.7rem 0.6rem 1rem;
		border-top: 1px dashed var(--color-surface-2);
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

	.marginalia li.torn { color: var(--color-text-faint); text-decoration: line-through; }

	.bullet { color: var(--owner-color, var(--color-text-muted)); }
	.torn-mark { color: #a05050; font-size: 0.78rem; }

	.empty {
		font-size: 0.82rem;
		color: var(--color-text-faint);
		margin: 0.4rem 0;
		font-style: italic;
	}
</style>
