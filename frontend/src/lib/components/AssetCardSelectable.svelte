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
	import { hiddenCount } from '$lib/secretCounts';
	import { useSuccession } from '$lib/successionContext';
	import AssetTypeIcon from './AssetTypeIcon.svelte';
	import CrownGlyph from './CrownGlyph.svelte';

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
		 * Number of secrets on this asset whose content the viewer can read.
		 * The total (existence) comes from `asset.secret_count`; the difference
		 * is shown as a struck-eye "hidden from you" count. Both are passive
		 * indicators after the name. Visibility is author/grant-based, not
		 * owner-based: a secret you authored on any asset (including a foreign
		 * secret planted on your own asset that you can't read) resolves
		 * correctly as long as you pass the real per-viewer count from the
		 * secret-counts seam. Leaving this undefined hides the indicators
		 * entirely — only do that when you have no known-count source; passing
		 * a fake 0 would force every existing secret to read as hidden.
		 */
		knownSecretCount?: number;
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
		knownSecretCount,
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

	// Secret indicators: open eye = secrets you can read, struck eye = secrets
	// that exist but are hidden from you (total − known). Only shown when the
	// caller supplies the viewer's known count (see prop doc); each eye then
	// renders only if its own count is non-zero. Clamp so a stale known count
	// can't push hidden negative.
	const showSecrets = $derived(knownSecretCount !== undefined);
	const knownSecrets = $derived(Math.max(0, knownSecretCount ?? 0));
	const hiddenSecrets = $derived(hiddenCount(asset, knownSecrets));

	// `defaultExpanded` is intentionally a one-shot initial-value hint, not a
	// live binding — once the user toggles the card we want their state to
	// stick even if the parent passes a new default. `untrack` makes that
	// intent explicit and silences the state_referenced_locally warning.
	let expandedRaw = $state(untrack(() => defaultExpanded));
	// In marginalia-pick mode the card is always expanded: the marginalia
	// list IS the picker, so hiding it would defeat the mode.
	const expanded = $derived(marginaliaSelectable ? true : expandedRaw);

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

	// Line-of-succession crown lookup (ADR-007). Undefined when no provider is
	// mounted (isolated stories/tests) → no crowns render.
	const succession = useSuccession();
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

		<span class="name-block">
			<span class="name">
				<span class="name-text">{asset.name}</span>
			</span>
			{#if ownerLabel}
				<span class="owner-label">{ownerLabel}</span>
			{/if}
		</span>

		<!-- Status glyphs + type live in one right-aligned cluster, generously
		     separated from the name by the 1fr name-block. -->
		<span class="meta">
			{#if asset.is_main_character}
				<span class="main-badge" title="Main character">★</span>
			{/if}
			{#if asset.is_leveraged}
				<!-- Passive status glyph: this asset is spent for a roll until
				     refreshed. Echoes the "+🎲" leverage chip; tinted
				     --color-leveraged. Shown in every mode (leverage mode filters
				     leveraged assets out of its own list, so no double-die there). -->
				<span class="lev-badge" title="Leveraged — spent for a roll until refreshed" aria-label="Leveraged">
					<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<rect x="3" y="3" width="18" height="18" rx="3" />
						<circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" />
						<circle cx="16" cy="8" r="1.2" fill="currentColor" stroke="none" />
						<circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" />
						<circle cx="8" cy="16" r="1.2" fill="currentColor" stroke="none" />
						<circle cx="16" cy="16" r="1.2" fill="currentColor" stroke="none" />
					</svg>
				</span>
			{/if}
			{#if showSecrets && knownSecrets > 0}
				<!-- Open eye: secrets whose content you can read. -->
				<span class="sec-badge known" title={`${knownSecrets} secret${knownSecrets === 1 ? '' : 's'} you can read`} aria-label={`${knownSecrets} secrets you can read`}>
					<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
						<circle cx="12" cy="12" r="3" />
					</svg><span class="sec-num">{knownSecrets}</span>
				</span>
			{/if}
			{#if showSecrets && hiddenSecrets > 0}
				<!-- Struck eye: secrets that exist but are hidden from you. -->
				<span class="sec-badge hidden" title={`${hiddenSecrets} secret${hiddenSecrets === 1 ? '' : 's'} hidden from you`} aria-label={`${hiddenSecrets} secrets hidden from you`}>
					<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
						<circle cx="12" cy="12" r="3" />
						<line x1="3" y1="21" x2="21" y2="3" />
					</svg><span class="sec-num">{hiddenSecrets}</span>
				</span>
			{/if}
			<AssetTypeIcon type={asset.asset_type} />
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
							<!-- Text + crown share a centred line so the glyph sits on the
							     text's vertical middle, independent of the row's baseline
							     alignment and 32px min-height. -->
							<span class="m-line">
								<span class="m-text">{m.text}</span>
								{#if m.title}
									{@const crown = succession?.crown(m.id)}
									{#if crown}
										<CrownGlyph mark={crown} size={14} />
									{/if}
								{/if}
							</span>
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
		background: var(--color-surface-sunken);
		overflow: hidden;
	}

	.card.selectable.selected {
		border-color: var(--owner-color, var(--color-accent));
		background: var(--color-surface-active);
	}

	.card.disabled {
		opacity: 0.5;
	}

	.header {
		display: grid;
		grid-template-columns: auto 1fr auto;
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

	/* select/marginalia/leverage modes all add one left "act" slot, giving 3
	   children — the base .header layout above already fits that. Plain
	   display-only cards drop the slot, so they get the narrower 2-column
	   override below. (The owner dot used to be a separate column; it was
	   dropped as redundant with the card's owner-color left border, see
	   .card above.) */

	/* Header click is a no-op in marginalia-pick mode (the card stays open)
	   so don't suggest a pointer affordance there. */
	.card.marginalia-selectable .header { cursor: default; }
	.card.marginalia-selectable .caret { display: none; }
	.card:not(.selectable):not(.marginalia-selectable):not(.has-leverage) .header { grid-template-columns: 1fr auto; }

	.select-tap-placeholder {
		width: 22px;
		height: 22px;
		flex-shrink: 0;
	}

	/* Highlight a card that has the picked marginalia, so the user can
	   see at a glance which asset their selection belongs to. */
	.card.marginalia-selectable:has(.marginalia li.picked) {
		border-color: var(--owner-color, var(--color-accent));
		background: var(--color-surface-active);
	}

	/* The marginalia row's own checkbox + selected state. In pick mode the row
	   carries a checkbox, so centre-align the line with it rather than sitting
	   the text on its baseline (which leaves the box floating high). */
	.marginalia li {
		min-height: 32px;
	}
	.card.marginalia-selectable .marginalia li {
		align-items: center;
	}
	.marginalia li.picked {
		color: var(--color-accent-bright);
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
		border: 1px solid var(--owner-color, var(--color-neutral));
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
		line-height: 1;
	}

	.name-block {
		display: flex;
		flex-direction: column;
		min-width: 0;
		gap: 0.05rem;
	}

	/* Name holds only the (truncating) text now; status glyphs moved to the
	   right-aligned .meta cluster. */
	.name {
		display: flex;
		align-items: center;
		min-width: 0;
		font-size: 0.92rem;
		color: var(--color-text);
	}

	.name-text {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Expanded card: the header is already showing the full marginalia list,
	   so let the name breathe onto a second line instead of hard-truncating —
	   engaging with a card should reveal the full name. Collapsed rows keep
	   the single-line ellipsis above so list density stays intact. */
	.header[aria-expanded='true'] .name-text {
		overflow: hidden;
		white-space: normal;
		text-overflow: clip;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	.owner-label {
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.main-badge {
		color: var(--owner-color, var(--color-accent));
		font-size: 0.78rem;
		flex-shrink: 0;
	}

	/* Leveraged status glyph in the right-aligned meta cluster. Passive — not a
	   tap target. */
	.lev-badge {
		color: var(--color-leveraged);
		flex-shrink: 0;
	}
	.lev-badge svg { vertical-align: -0.18em; }

	/* Secret indicators in the meta cluster. Open eye (known) reads in gold like
	   the Retinue eye; struck eye (hidden) is muted to read as "not available to
	   you". Passive — not tap targets here. */
	.sec-badge {
		display: inline-flex;
		align-items: center;
		gap: 1px;
		flex-shrink: 0;
	}
	.sec-badge svg { vertical-align: -0.18em; }
	.sec-badge.known { color: var(--color-accent); }
	.sec-badge.hidden { color: var(--color-text-muted); }
	.sec-num { font-size: 0.72rem; font-weight: 600; }

	.meta {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	/* The marginalia count reads like a note in the margin: a vertical hairline
	   (echoing the Laws/Rumors count divider) separates it from the type icon,
	   and it stretches to the row height so the rule has presence. Default text
	   colour matches the name; only the at-risk case goes red. */
	.count {
		align-self: stretch;
		display: flex;
		align-items: center;
		padding-left: 0.45rem;
		border-left: 1px solid var(--color-border-warm);
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
	}

	.caret { font-size: 0.8rem; color: var(--color-text); }

	/* Needlessly-at-risk: red count + caret, matching the header-chip risk
	   badge. Title on .count carries the meaning for non-colour users. */
	.count.at-risk { color: var(--color-at-risk); font-weight: 600; }
	.caret.at-risk { color: var(--color-at-risk); }

	/* Narrow phones (iPhone SE and similar): claw back a few px from the meta
	   cluster's gaps so more of the name survives before ellipsis. Icon sizes
	   are untouched — legibility of the glyphs is a hard requirement. */
	@media (max-width: 400px) {
		.meta { gap: 0.25rem; }
		.count { padding-left: 0.3rem; }
	}

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
		color: var(--color-text-secondary);
	}

	.marginalia li {
		display: flex;
		gap: 0.5rem;
		align-items: baseline;
	}

	.marginalia li.torn { color: var(--color-text-faint); text-decoration: line-through; }

	/* Text + trailing crown on one centred line. align-items:center keeps the
	   glyph on the text's vertical middle even though the row aligns to baseline
	   and is taller (min-height 32px) than the text. */
	.m-line {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		flex: 1;
	}

	.bullet { color: var(--owner-color, var(--color-text-muted)); }
	.torn-mark { color: #a05050; font-size: 0.78rem; }

	.empty {
		font-size: 0.82rem;
		color: var(--color-text-faint);
		margin: 0.4rem 0;
		font-style: italic;
	}
</style>
