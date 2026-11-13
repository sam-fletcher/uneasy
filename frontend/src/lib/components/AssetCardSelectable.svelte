<!--
	AssetCardSelectable.svelte

	Standardized expandable asset card for places where the player needs to
	read or pick assets (scene setup, scene details, future plan UI). Comes
	in two variants:

	- Display-only (default): name + type, expand to read marginalia.
	- Selectable: a checkbox in the header toggles the card's selected state.
	  Use the `selected`/`onToggle` props (multi-select) or pair `selected` with
	  a parent that clears other cards before flipping (single-select).

	A card is a game piece: the warm piece border, the piece corner and the
	raised surface. SELECTED is the 2px bright gold border with no fill, and a
	tick in the checkbox (ToneMark's success tick, the same one a success notice
	uses, painted gold by the card). Gold
	alone says "you picked this"; the owner's colour is kept for saying whose
	the card is. That shows as the owner's shape beside the name, derived from
	`ownerColor` (so callers need not change); the card's own left edge no
	longer wears the colour.

	Status reads as words, not badges: "Leveraged" sits under the name in
	orange beside the die. The card's colour and weight carry nothing the words
	don't also say.

	The component is intentionally small: it doesn't mutate the asset or
	tear marginalia. The one exception is leverage during a dice roll — pass
	`leverageMode` to surface a die toggle in the header's left slot (used by
	DiceRollPanel's draft-then-submit flow). The die toggle is a chip, so it
	has the chip's three looks:
	  - outlined        → not selected; tap to add it to your draft
	  - gold-bordered   → drafted (leverageDrafted) or already committed
	    (asset.is_leveraged); committed dice are locked
	The parent owns draft state and the actual commit (on Ready).
-->
<script lang="ts">
	import './shared/chip.css';
	import './shared/statusText.css';
	import { untrack } from 'svelte';
	import type { Asset } from '$lib/api';
	import { isBlankAsset, isNeedlesslyAtRisk } from '$lib/assetRisk';
	import { seatForColor } from '$lib/playerColor';
	import { hiddenCount } from '$lib/secretCounts';
	import { useSuccession } from '$lib/successionContext';
	import AssetTypeIcon from './AssetTypeIcon.svelte';
	import CrownGlyph from './CrownGlyph.svelte';
	import LogMark from './LogMark.svelte';
	import DieMark from './shared/DieMark.svelte';
	import PlayerShape from './shared/PlayerShape.svelte';
	import RowMark from './shared/RowMark.svelte';
	import StarMark from './shared/StarMark.svelte';
	import ToneMark from './shared/ToneMark.svelte';

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
		 *
		 * A *blank* asset (no marginalia at all) renders a single
		 * "destroy it outright" row instead, toggling with the sentinel id 0
		 * — there is nothing to tear, so the break takes the whole asset
		 * (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D3). Callers must read
		 * the asset from the second argument, not from the id, in that case.
		 */
		marginaliaSelectable?: boolean;
		selectedMarginaliaID?: number | null;
		/**
		 * Which asset the current marginalia pick belongs to. Only needed in
		 * marginalia-pick mode, and only to disambiguate the blank-asset
		 * sentinel (id 0) between two blank cards in the same list — real
		 * marginalia ids are already unique.
		 */
		selectedAssetID?: number | null;
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
		selectedAssetID = null,
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

	// A blank asset has no marginalia line to pick, so in pick mode it offers a
	// single whole-asset row instead: breaking it destroys it outright. Sentinel
	// id 0, disambiguated by selectedAssetID (see the prop doc).
	const blank = $derived(isBlankAsset(asset));
	const blankPickable = $derived(marginaliaSelectable && blank);
	const blankPicked = $derived(
		blankPickable && selectedMarginaliaID === 0 && selectedAssetID === asset.id,
	);

	// One tear from destruction but a slot is still fillable — tint the count
	// and caret red so the fragility reads at a glance. See isNeedlesslyAtRisk.
	const atRisk = $derived(isNeedlesslyAtRisk(asset));

	// The owner's shape. Null for a colour that isn't one of the five seat
	// colours, in which case no mark is drawn rather than a wrong one.
	const ownerSeat = $derived(seatForColor(ownerColor));

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
				{#if selected}<ToneMark tone="success" />{/if}
			</span>
		{:else if marginaliaSelectable}
			<!-- Placeholder keeps the header grid columns aligned with the
			     selectable variant; the actual checkbox lives on each
			     marginalia line below. -->
			<span class="select-tap-placeholder" aria-hidden="true"></span>
		{:else if leverageMode}
			<!-- Leverage draft toggle. Shares the left "act on this asset"
			     slot with the checkbox; the two modes never appear together.
			     It is a chip: picked is the bright gold border, locked is the
			     chip's own disabled look. -->
			<span
				class="lev-tap chip"
				class:selected={leverageFilled}
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
			>+<DieMark size={18} /></span>
		{/if}

		<span class="name-block">
			<span class="name">
				{#if ownerSeat != null}
					<!-- Whose it is: the owner's shape, in their colour. The name
					     (or the "Owned by" line) is nearby, so it is decoration
					     to a screen reader. -->
					<PlayerShape seat={ownerSeat} color={ownerColor} />
				{/if}
				<span class="name-text">{asset.name}</span>
			</span>
			{#if ownerLabel || asset.is_leveraged}
				<span class="sub">
					{#if ownerLabel}
						<span class="owner-label">{ownerLabel}</span>
					{/if}
					{#if asset.is_leveraged}
						<!-- Passive status: this asset is spent for a roll until
						     refreshed. Orange, beside its word, so the colour never
						     has to say it alone. Shown in every mode (leverage
						     mode filters leveraged assets out of its own list, so
						     no double-die there). -->
						<span class="status-word lev-word" title="Spent for a roll until refreshed">
							<DieMark size={14} />Leveraged
						</span>
					{/if}
				</span>
			{/if}
		</span>

		<!-- Status glyphs + type live in one right-aligned cluster, generously
		     separated from the name by the 1fr name-block. -->
		<span class="meta">
			{#if asset.is_main_character}
				<span class="main-badge" title="Main character"><StarMark size={14} label="Main character" /></span>
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
			{#if !marginaliaSelectable}
				<!-- A caret: tapping the header expands the card in place. -->
				<span class="caret-slot" class:at-risk={atRisk}><RowMark kind="caret" open={expanded} /></span>
			{/if}
		</span>
	</button>

	{#if expanded}
		<div class="body">
			{#if blankPickable}
				<!-- Nothing to tear: the pick is the asset itself. Same checkbox
				     affordance as a marginalia line so the row reads as one of them. -->
				<ul class="marginalia">
					<li class:picked={blankPicked}>
						<span
							class="select-tap m-tap"
							role="checkbox"
							tabindex={disabled ? -1 : 0}
							aria-checked={blankPicked}
							aria-disabled={disabled}
							onclick={(e) => { e.stopPropagation(); handleMarginaliaSelect(0); }}
							onkeydown={(e) => {
								if (e.key === ' ' || e.key === 'Enter') {
									e.preventDefault();
									handleMarginaliaSelect(0);
								}
							}}
						>
							{#if blankPicked}<ToneMark tone="success" size={14} />{/if}
						</span>
						<span class="m-line">
							<span class="m-text destroys">No marginalia — breaking destroys this asset.</span>
						</span>
					</li>
				</ul>
			{:else if asset.marginalia.length === 0}
				<p class="empty-note">No marginalia recorded.</p>
			{:else}
				<ul class="marginalia">
					{#each asset.marginalia as m (m.id)}
						{@const isPickable = marginaliaSelectable && !m.is_torn}
						{@const isPicked = isPickable && selectedMarginaliaID === m.id}
						<li class:torn={m.is_torn} class:picked={isPicked}>
							{#if m.is_torn}
								<!-- The house tear, in red: a torn note is a loss. The line
								     through the words says it too, without colour. -->
								<span class="torn-mark" role="img" aria-label="torn"><LogMark family="tear" /></span>
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
									{#if isPicked}<ToneMark tone="success" size={14} />{/if}
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
	/* A card is a game piece: warm border, piece corner, raised surface. */
	.card {
		border: 1px solid var(--color-border-piece);
		border-radius: var(--radius-piece);
		background: var(--color-surface);
		overflow: hidden;
	}

	/* A card that can't be picked (a peer already claimed, say) drops to the
	   neutral hairline and faint text. It is not faded with opacity, which would
	   take the words under the contrast floor. */
	.card.disabled { border-color: var(--color-border); }
	.card.disabled .name,
	.card.disabled .owner-label,
	.card.disabled .count,
	.card.disabled .meta { color: var(--color-text-faint); }

	/* SELECTED: the 2px bright gold border with no fill. The second pixel is an
	   inset shadow so selecting never moves the card or its neighbours. It stays
	   gold when the card is also disabled: it still shows which one was picked.
	   (In marginalia-pick mode the card is the one that holds the picked line.) */
	.card.selected,
	.card.marginalia-selectable:has(.marginalia li.picked) {
		border-color: var(--color-you);
		box-shadow: inset 0 0 0 1px var(--color-you);
	}

	.header {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		padding: var(--space-2) var(--space-3);
		background: none;
		border: none;
		text-align: left;
		color: inherit;
		cursor: pointer;
		min-height: var(--size-tap);
	}
	.header:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: -2px;
	}

	/* select/marginalia/leverage modes all add one left "act" slot, giving 3
	   children — the base .header layout above already fits that. Plain
	   display-only cards drop the slot, so they get the narrower 2-column
	   override below. */

	/* Header click is a no-op in marginalia-pick mode (the card stays open)
	   so don't suggest a pointer affordance there. */
	.card.marginalia-selectable .header { cursor: default; }
	.card:not(.selectable):not(.marginalia-selectable):not(.has-leverage) .header { grid-template-columns: 1fr auto; }

	.select-tap-placeholder {
		width: 24px;
		height: 24px;
		flex-shrink: 0;
	}

	/* The marginalia row's own checkbox + selected state. In pick mode the row
	   carries a checkbox, so centre-align the line with it rather than sitting
	   the text on its baseline (which leaves the box floating high). */
	.marginalia li {
		min-height: 32px;
	}
	/* Pick mode: every live row is a 44px tap target (docs/STYLE_GUIDE.md). The
	   24px box stays the visible affordance; its ::after is stretched over the
	   whole row (the li is the positioned ancestor — .m-tap deliberately isn't),
	   so tapping the text picks it too, and neighbouring rows never overlap
	   because each is a full 44px tall. */
	.card.marginalia-selectable .marginalia li {
		align-items: center;
	}
	.card.marginalia-selectable .marginalia li:not(.torn) {
		min-height: var(--size-tap);
		position: relative;
	}
	.card.marginalia-selectable .m-tap::after {
		content: '';
		position: absolute;
		inset: 0;
	}
	/* A picked line is bright gold, the one place a pick reaches the text. */
	.marginalia li.picked {
		color: var(--color-you);
	}

	/* The checkbox: a control standing on a raised card, so it takes the
	   raised-twice fill and a neutral control border. Picked is the bright gold
	   border plus the tick; no fill. */
	.select-tap {
		width: 24px;
		height: 24px;
		box-sizing: border-box;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		cursor: pointer;
		color: var(--color-you);
	}
	.m-tap { width: 20px; height: 20px; }
	.select-tap[aria-checked='true'] {
		border-color: var(--color-you);
		box-shadow: inset 0 0 0 1px var(--color-you);
	}
	.select-tap[aria-disabled='true'] { cursor: not-allowed; }
	.select-tap:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	/* Invisible tap target around the header checkbox: extends the 24px box
	   out to ~44px. Scoped to .header: the per-line marginalia checkboxes
	   (.m-tap) stretch over their whole 44px row instead (see above). */
	.header .select-tap { position: relative; }
	.header .select-tap::after { content: ''; position: absolute; inset: -10px; }

	.name-block {
		display: flex;
		flex-direction: column;
		min-width: 0;
		gap: var(--space-1);
	}

	/* Name holds the owner's shape and the (truncating) text; status glyphs
	   live in the right-aligned .meta cluster and status words in .sub. */
	.name {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		font-size: var(--text-body);
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

	/* Under the name: who owns it, and any status words. */
	.sub {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		column-gap: var(--space-3);
		row-gap: 0;
	}
	.owner-label {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	/* Leveraged: orange, beside the die and its word. The status-word recipe
	   (small capitals) with the leveraged colour. */
	.lev-word {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--color-leveraged);
	}

	.main-badge {
		display: inline-flex;
		color: var(--color-accent);
		flex-shrink: 0;
	}

	/* Secret indicators in the meta cluster. Open eye (known) reads in gold like
	   the Retinue eye; struck eye (hidden) is quiet to read as "not available to
	   you". Passive — not tap targets here. */
	.sec-badge {
		display: inline-flex;
		align-items: center;
		gap: 0;
		flex-shrink: 0;
	}
	.sec-badge svg { vertical-align: -0.18em; }
	.sec-badge.known { color: var(--color-accent); }
	.sec-badge.hidden { color: var(--color-text-secondary); }
	.sec-num { font-size: var(--text-label); font-weight: 600; }

	.meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-label);
		color: var(--color-text-secondary);
		flex-shrink: 0;
	}

	/* The marginalia count reads like a note in the margin: a vertical hairline
	   separates it from the type icon, and it stretches to the row height so the
	   rule has presence. Default text colour matches the name; only the at-risk
	   case goes red. */
	.count {
		align-self: stretch;
		display: flex;
		align-items: center;
		padding-left: var(--space-2);
		border-left: 1px solid var(--color-border);
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
	}

	.caret-slot { display: inline-flex; }

	/* Needlessly-at-risk: red count + caret. Red is "something could be lost";
	   the title on .count carries the meaning for non-colour users, and the
	   retinue says it in words. RowMark sets its own colour, so the override
	   reaches in with :global. */
	.count.at-risk { color: var(--color-danger); font-weight: 600; }
	.caret-slot.at-risk :global(.row-mark) { color: var(--color-danger); }

	/* Narrow columns (small phones; any record-phase column): claw back a few
	   px from the meta cluster's gaps so more of the name survives before
	   ellipsis. Icon sizes are untouched — legibility of the glyphs is a hard
	   requirement. Container query, not viewport (docs/STYLE_GUIDE.md
	   "Layout widths"). */
	@container column (max-width: 400px) {
		.meta { gap: var(--space-1); }
		.count { padding-left: var(--space-1); }
	}

	/* Leverage draft toggle: the house chip, drawn 36px tall inside a 44px
	   invisible tap target (::after) so it is comfortable to hit without
	   inflating the header. Picked, locked and hover come from chip.css; only the
	   size is this card's. Sits in the same left slot the checkbox uses. */
	.lev-tap.chip {
		position: relative;
		gap: var(--space-1);
		min-height: 36px;
		padding: 0 var(--space-2);
		line-height: 1;
	}
	/* Invisible tap target: extends the 36px chip out to ~44px on all sides. */
	.lev-tap::after { content: ''; position: absolute; inset: -4px; }

	.body {
		padding: 0 var(--space-3) var(--space-3) var(--space-4);
		border-top: 1px solid var(--color-border);
	}

	.marginalia {
		list-style: none;
		margin: var(--space-2) 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	.marginalia li {
		display: flex;
		gap: var(--space-2);
		align-items: baseline;
	}

	/* A torn note: faint words with a line through them. The red is the tear mark. */
	.marginalia li.torn { color: var(--color-text-faint); text-decoration: line-through; }

	/* Text + trailing crown on one centred line. align-items:center keeps the
	   glyph on the text's vertical middle even though the row aligns to baseline
	   and is taller (min-height 32px) than the text. */
	.m-line {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		flex: 1;
	}

	.bullet { color: var(--color-text-faint); }
	.torn-mark {
		display: inline-flex;
		flex: none;
		width: 14px;
		height: 14px;
		align-self: center;
		color: var(--color-danger);
	}

	/* Blank-asset pick row: picking it destroys the asset, so the line reads in
	   the danger colour rather than as ordinary marginalia text. Italic matches
	   the .empty-note placeholder it replaces. */
	.m-text.destroys { color: var(--color-danger); font-style: italic; }

	/* Named -note, not .empty: a bare state-class name collides with the
	   marginalia tile's .empty slot (see RetinueView's .empty-note). */
	.empty-note {
		font-size: var(--text-small);
		color: var(--color-text-faint);
		margin: var(--space-2) 0;
		font-style: italic;
	}
</style>
