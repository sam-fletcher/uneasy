<!-- SuggestionPicker.svelte
  Shared "pick one of a few examples, or write your own" control. Renders a
  fixed grid of suggestion tiles (blanks fill any missing slots) plus a Custom
  tile that reveals a free-text field. The chosen text is exposed via the
  bindable `value` prop, so callers just read value.trim().

  Used wherever a player authors asset text from scratch (Prologue claims,
  Retinue marginalia, peer/asset naming) — see asset_suggestions.go for the
  matching type-keyed example pools.

  Callers that pass `onReroll` get a square refresh button sharing the fourth
  grid cell with a narrowed "Custom…" — deliberately inside the existing 2×2
  footprint so the control costs no vertical height on mobile.
-->
<script lang="ts">
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		/** Up to `slots` example strings to offer. */
		suggestions: string[];
		/** The resulting text (a picked suggestion or the custom entry). */
		value: string;
		/** Placeholder for the custom free-text field. */
		customPlaceholder?: string;
		/** When true, show a loading note instead of the grid. */
		loading?: boolean;
		/** Fixed number of suggestion slots; missing ones render as blanks. */
		slots?: number;
		/** Max length for the custom field. Defaults to the marginalia tier —
		 *  every current caller is writing a marginalia. */
		maxlength?: number;
		/** Render the custom field as a multi-line textarea (for marginalia). */
		multiline?: boolean;
		disabled?: boolean;
		/** Refetch this picker's suggestion pool. Omit to hide the reroll
		 *  button entirely — the pre-reroll layout is unchanged. */
		onReroll?: () => void | Promise<void>;
		/** True while a reroll is in flight; disables the button. */
		rerolling?: boolean;
	}

	let {
		suggestions,
		value = $bindable(''),
		customPlaceholder = 'Write your own…',
		loading = false,
		slots = 3,
		maxlength = TEXT_LIMITS.MARGINALIA,
		multiline = false,
		disabled = false,
		onReroll = undefined,
		rerolling = false,
	}: Props = $props();

	// Whether the custom free-text field is active. A picked suggestion turns it
	// off; choosing Custom turns it on (clearing a previously-picked suggestion
	// so the field starts empty).
	let customMode = $state(false);

	const pickSuggestion = (s: string) => {
		customMode = false;
		value = s;
	};
	const pickCustom = () => {
		if (!customMode) {
			customMode = true;
			if (suggestions.includes(value)) value = '';
		}
	};

	// A reroll swaps all three tiles, so a pick made from the old set is no
	// longer on offer — drop it rather than leave `value` holding text with no
	// tile left to highlight. Custom-authored text is the player's own writing,
	// so it survives untouched (and the field stays open).
	const reroll = async () => {
		if (!onReroll || rerolling) return;
		if (!customMode) value = '';
		await onReroll();
	};
</script>

{#if loading}
	<p class="sp-loading">Loading suggestions…</p>
{:else}
	<div class="sp-grid">
		{#each Array(slots) as _, i (i)}
			{#if i < suggestions.length}
				<button
					type="button"
					class="sp-tile"
					class:selected={!customMode && value === suggestions[i]}
					{disabled}
					onclick={() => pickSuggestion(suggestions[i])}
				>
					{suggestions[i]}
				</button>
			{:else}
				<span class="sp-tile blank" aria-hidden="true"></span>
			{/if}
		{/each}
		<div class="sp-last">
			<button
				type="button"
				class="sp-tile custom"
				class:selected={customMode}
				{disabled}
				onclick={pickCustom}
			>
				Custom…
			</button>
			{#if onReroll}
				<button
					type="button"
					class="sp-tile sp-reroll"
					title="New suggestions"
					aria-label="New suggestions"
					disabled={disabled || rerolling}
					onclick={reroll}
				>
					<svg
						viewBox="0 0 24 24"
						width="18"
						height="18"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M20 11a8.1 8.1 0 0 0 -15.5 -2m-.5 -4v4h4" />
						<path d="M4 13a8.1 8.1 0 0 0 15.5 2m.5 4v-4h-4" />
					</svg>
				</button>
			{/if}
		</div>
	</div>
	{#if customMode}
		{#if multiline}
			<textarea
				class="sp-custom-input"
				bind:value
				placeholder={customPlaceholder}
				{maxlength}
				{disabled}
				rows={2}
			></textarea>
		{:else}
			<input
				type="text"
				class="sp-custom-input"
				bind:value
				placeholder={customPlaceholder}
				{maxlength}
				{disabled}
			/>
		{/if}
	{/if}
{/if}

<style>
	.sp-loading { color: var(--color-text-muted); font-size: 0.8rem; margin: 0; }

	.sp-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.4rem;
	}
	.sp-tile {
		min-height: 44px;
		padding: 0.4rem 0.6rem;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-strong);
		border-radius: 6px;
		color: var(--color-text);
		font-size: 0.9rem;
		font-family: inherit;
		text-align: center;
		cursor: pointer;
		word-break: break-word;
		transition: background-color 120ms ease, border-color 120ms ease;
	}
	/* :not(.selected) keeps the hover rule from out-specifying .selected —
	   without it, sticky touch-hover turned a just-tapped tile grey. */
	.sp-tile:hover:not(.blank):not(.selected) { background: var(--color-surface-hover); }
	.sp-tile.selected {
		background: var(--color-chip-gold-bg);
		border-color: var(--color-accent);
		color: var(--white);
	}
	.sp-tile:disabled { opacity: 0.4; cursor: not-allowed; }
	.sp-tile.custom { color: var(--color-accent); } /* font-style: italic; */
	.sp-tile.custom.selected { color: var(--white); font-style: normal; }
	.sp-tile.blank {
		background: transparent;
		border-style: dashed;
		border-color: var(--color-border);
		cursor: default;
	}

	/* The fourth grid cell, split between Custom… and the reroll square. With
	   no reroll button the lone flex:1 child fills the cell exactly as the
	   bare button used to. */
	.sp-last {
		display: flex;
		gap: 0.4rem;
		min-width: 0;
	}
	.sp-last .sp-tile.custom {
		flex: 1;
		min-width: 0;
	}
	/* Unfilled + warm border so the reroll reads as an action, not a fifth
	   choice — the pickable tiles are the ones with a surface fill. */
	.sp-reroll {
		flex: none;
		width: 44px;
		padding: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: transparent;
		border-color: var(--color-border-warm-strong);
		color: var(--color-accent);
	}

	.sp-custom-input {
		margin-top: 0.4rem;
		width: 100%;
		box-sizing: border-box;
		background: var(--color-surface-2);
		color: var(--color-text);
		border: 1px solid var(--color-border-strong);
		border-radius: 4px;
		padding: 0.4rem 0.5rem;
		font-size: 0.9rem;
		font-family: inherit;
	}
</style>
