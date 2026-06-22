<!-- MakeMarPicker.svelte
  Shared make/mar checkbox picker used by every plan component.
  Parent owns the selected[] array and toggle/submit callbacks.
-->
<script lang="ts">
	import './planPanel.css';
	import type { Snippet } from 'svelte';
	import type { PlanChoiceOption } from './shared';

	interface Props {
		outcome: 'make' | 'mar';
		options: PlanChoiceOption[];
		selected: string[];
		busy: boolean;
		onToggle: (key: string) => void;
		onSubmit: () => void;
		/** Optional per-plan note rendered above the option list. */
		header?: Snippet;
		/**
		 * When true, render radio buttons (choose exactly one option) instead of
		 * checkboxes. Used by plans whose rules pick a single option.
		 */
		single?: boolean;
		/** Option keys to disable (e.g. beyond the dice-margin level cap). */
		disabledKeys?: string[];
	}

	let {
		outcome, options, selected, busy, onToggle, onSubmit, header,
		single = false, disabledKeys = [],
	}: Props = $props();

	const nothingSelected = $derived(selected.length === 0);
</script>

<div class="choices-section">
	<p class="choices-header">
		Result: <strong class="outcome-{outcome}">{outcome === 'make' ? '✓ Make' : '✗ Mar'}</strong>
	</p>

	{#if header}{@render header()}{/if}

	{#if options.length > 0}
		<p class="choices-note">{single ? 'Choose one option:' : 'Select options to apply:'}</p>
		{#each options as opt}
			<label class="choice-item" class:disabled={disabledKeys.includes(opt.key)}>
				<input type={single ? 'radio' : 'checkbox'}
					checked={selected.includes(opt.key)}
					disabled={disabledKeys.includes(opt.key)}
					onchange={() => onToggle(opt.key)}
				/>
				{opt.label}
			</label>
		{/each}
	{/if}

	<button class="action-btn primary" onclick={onSubmit} disabled={busy || (single && nothingSelected)}>
		{busy ? '…' : 'Apply choices'}
	</button>
</div>

<style>
	.choice-item.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
