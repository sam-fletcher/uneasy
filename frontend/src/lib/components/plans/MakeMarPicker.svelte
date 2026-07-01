<!-- MakeMarPicker.svelte
  Shared make/mar option picker used by every plan component. Renders each
  option as a tappable card (≥44px) rather than a native radio/checkbox:
  `single` plans pick exactly one (selecting deselects the rest); multi plans
  toggle independently and show a check indicator. Options carrying a `level`
  render a signed badge ("+1", "−2"); the parent gates out-of-reach levels via
  `disabledKeys` and explains why via `lockReason`. Parent owns selected[].
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
		 * When true, pick exactly one option. Otherwise options toggle
		 * independently (multi-select) and each card shows a check indicator.
		 */
		single?: boolean;
		/** Option keys to disable (e.g. beyond the dice-margin level cap). */
		disabledKeys?: string[];
		/** Short reason a disabled option is locked, e.g. "needs −3". */
		lockReason?: (key: string) => string | undefined;
	}

	let {
		outcome, options, selected, busy, onToggle, onSubmit, header,
		single = false, disabledKeys = [], lockReason,
	}: Props = $props();

	const nothingSelected = $derived(selected.length === 0);
	// Makes count up from the difficulty (+0, +1, …); mars count down (−1, …).
	const sign = $derived(outcome === 'make' ? '+' : '−');
</script>

<div class="choices-section">
	<p class="choices-header">
		Result: <span class="outcome-{outcome}">{outcome === 'make' ? '✓ Make' : '✗ Mar'}</span>
	</p>

	{#if header}{@render header()}{/if}

	{#if options.length > 0}
		<p class="choices-note">{single ? 'Choose one option:' : 'Select options to apply:'}</p>
		<div class="choice-list">
			{#each options as opt (opt.key)}
				{@const isSelected = selected.includes(opt.key)}
				{@const isLocked = disabledKeys.includes(opt.key)}
				{@const reason = isLocked ? lockReason?.(opt.key) : undefined}
				<button type="button" class="choice-card" class:active={isSelected} class:locked={isLocked}
					disabled={isLocked} aria-pressed={isSelected} onclick={() => onToggle(opt.key)}>
					<span class="choice-badge">
						{#if opt.level != null}{sign}{opt.level}
						{:else if isSelected}✓
						{/if}
					</span>
					<span class="choice-text">
						<span class="choice-title">{opt.label}{#if isSelected}<span class="choice-tick"> ✓</span>{/if}</span>
						{#if opt.desc}<span class="choice-desc">{opt.desc}</span>{/if}
					</span>
					{#if reason}<span class="choice-lock">{reason}</span>{/if}
				</button>
			{/each}
		</div>
	{/if}

	<button class="action-btn primary" onclick={onSubmit} disabled={busy || (single && nothingSelected)}>
		{busy ? '…' : 'Apply choices'}
	</button>
</div>
