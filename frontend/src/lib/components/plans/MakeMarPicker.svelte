<!-- MakeMarPicker.svelte
  Shared make/mar option picker used by every plan component. Renders each
  option as a tappable card (≥44px) rather than a native radio/checkbox:
  `single` plans pick exactly one (selecting deselects the rest); multi plans
  toggle independently. A pick is the card's bright gold border and a tick:
  the options wear no meaning colour (a choice is told apart by its words),
  though the RESULT above them does (Make green, Mar red, each beside its
  word). Options carrying a `level` show it as a signed number ("+1", "−2");
  the parent gates out-of-reach levels via `disabledKeys` and explains why via
  `lockReason`. Parent owns selected[].

  `readOnly` is a watcher's view: the same cards inside the shared greyed mirror
  (planPanel.css), described in the neutral words (`neutralDesc`), with no button.
  It shows the options on offer and which the margin puts out of reach, never a
  pick: the picker's own picks are local state that nobody else is sent.
-->
<script lang="ts">
	import './planPanel.css';
	import type { Snippet } from 'svelte';
	import type { PlanChoiceOption } from './shared';
	import EndsNote from './shared/EndsNote.svelte';
	import OptionCard from '$lib/components/shared/OptionCard.svelte';

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
		/** Shown under the submit button when applying the current picks ends the plan. */
		endsText?: string | null;
		/** A watcher's greyed, inert copy: neutral wording, no button. */
		readOnly?: boolean;
	}

	let {
		outcome, options, selected, busy, onToggle, onSubmit, header,
		single = false, disabledKeys = [], lockReason, endsText = null, readOnly = false,
	}: Props = $props();

	const nothingSelected = $derived(selected.length === 0);
	// Makes count up from the difficulty (+0, +1, …); mars count down (−1, …).
	const sign = $derived(outcome === 'make' ? '+' : '−');
</script>

{#snippet picker()}
	<div class="choices-section">
		<p class="choices-header">
			Result: <span class="outcome-{outcome}">{outcome === 'make' ? 'Make' : 'Mar'}</span>
		</p>

		{#if header}{@render header()}{/if}

		{#if options.length > 0}
			<!-- The player choosing is told what to do; a watcher, what is being chosen. -->
		<p class="choices-note">{readOnly
			? (single ? 'One of these options is chosen:' : 'The options to apply:')
			: (single ? 'Choose one option:' : 'Select options to apply:')}</p>
			<div class="option-list">
				{#each options as opt (opt.key)}
					{@const isLocked = disabledKeys.includes(opt.key)}
					<OptionCard
						title={opt.label}
						badge={opt.level != null ? `${sign}${opt.level}` : undefined}
						description={(readOnly ? opt.neutralDesc ?? opt.desc : opt.desc) || undefined}
						reason={isLocked ? lockReason?.(opt.key) : undefined}
						selected={selected.includes(opt.key)}
						disabled={isLocked}
						onclick={() => onToggle(opt.key)}
					/>
				{/each}
			</div>
		{/if}

		{#if !readOnly}
			<button class="action-btn primary" onclick={onSubmit} disabled={busy || (single && nothingSelected)}>
				{busy ? '…' : 'Apply choices'}
			</button>
			{#if endsText}<EndsNote text={endsText} />{/if}
		{/if}
	</div>
{/snippet}

<!-- A watcher's copy sits inside the shared mirror: a disabled fieldset, so every card is
     inert, and nothing in it can be committed. -->
{#if readOnly}
	<fieldset class="resolve-mirror-wrap resolve-mirror" disabled>
		{@render picker()}
	</fieldset>
{:else}
	{@render picker()}
{/if}
