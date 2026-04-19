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
	}

	let { outcome, options, selected, busy, onToggle, onSubmit, header }: Props = $props();
</script>

<div class="choices-section">
	<p class="choices-header">
		Result: <strong class="outcome-{outcome}">{outcome === 'make' ? '✓ Make' : '✗ Mar'}</strong>
	</p>

	{#if header}{@render header()}{/if}

	{#if options.length > 0}
		<p class="choices-note">Select options to apply:</p>
		{#each options as opt}
			<label class="choice-item">
				<input type="checkbox"
					checked={selected.includes(opt.key)}
					onchange={() => onToggle(opt.key)}
				/>
				{opt.label}
			</label>
		{/each}
	{/if}

	<button class="action-btn primary" onclick={onSubmit} disabled={busy}>
		{busy ? '…' : 'Apply choices'}
	</button>
</div>
