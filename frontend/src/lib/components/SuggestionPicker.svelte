<!-- SuggestionPicker.svelte
  Shared "pick one of a few examples, or write your own" control. Renders a
  fixed grid of suggestion tiles (blanks fill any missing slots) plus a Custom
  tile that reveals a free-text field. The chosen text is exposed via the
  bindable `value` prop, so callers just read value.trim().

  Used wherever a player authors asset text from scratch (Prologue claims,
  Retinue marginalia, peer/asset naming) — see asset_suggestions.go for the
  matching type-keyed example pools.
-->
<script lang="ts">
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
		/** Max length for the custom field. */
		maxlength?: number;
		/** Render the custom field as a multi-line textarea (for marginalia). */
		multiline?: boolean;
		disabled?: boolean;
	}

	let {
		suggestions,
		value = $bindable(''),
		customPlaceholder = 'Write your own…',
		loading = false,
		slots = 3,
		maxlength = 280,
		multiline = false,
		disabled = false,
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
		<button
			type="button"
			class="sp-tile custom"
			class:selected={customMode}
			{disabled}
			onclick={pickCustom}
		>
			Custom…
		</button>
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
	.sp-loading { color: #999; font-size: 0.8rem; margin: 0; }

	.sp-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.4rem;
	}
	.sp-tile {
		min-height: 44px;
		padding: 0.4rem 0.6rem;
		background: #2a2a28;
		border: 1px solid #3a3a3a;
		border-radius: 6px;
		color: #e8e4d9;
		font-size: 0.9rem;
		font-family: inherit;
		text-align: center;
		cursor: pointer;
		word-break: break-word;
		transition: background-color 120ms ease, border-color 120ms ease;
	}
	.sp-tile:hover:not(.blank) { background: #34332f; }
	.sp-tile.selected {
		background: #4a3f24;
		border-color: #c8a96e;
		color: #fff;
	}
	.sp-tile:disabled { opacity: 0.4; cursor: not-allowed; }
	.sp-tile.custom { font-style: italic; color: #c8a96e; }
	.sp-tile.custom.selected { color: #fff; font-style: normal; }
	.sp-tile.blank {
		background: transparent;
		border-style: dashed;
		border-color: #2c2c2c;
		cursor: default;
	}

	.sp-custom-input {
		margin-top: 0.4rem;
		width: 100%;
		box-sizing: border-box;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #444;
		border-radius: 4px;
		padding: 0.4rem 0.5rem;
		font-size: 0.9rem;
		font-family: inherit;
	}
</style>
