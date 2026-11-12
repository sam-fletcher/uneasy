<!--
  THE sheet. A raised surface, rounded on its top corners, that rises over the
  screen with a scrim behind it and a soft shadow (the only shadow in the app).
  Slides up on mobile, centered on larger screens. Dismissed via ESC, phone
  Back, backdrop tap, or close button.

  Put a <h3 class="sheet-title"> first in the body (modalShell.css) and the
  sheet's content after it. Sheets never nest: opening one from inside another
  closes the first. Bright gold, a main button and the rest follow the same
  recipes as anywhere else; a control that stands on this surface and needs to
  read as raised again (an input) takes the raised-twice fill.
-->
<script lang="ts">
	import '$lib/components/shared/modalShell.css';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { dismissOnBack } from '$lib/dismissOnBack.svelte';

	let {
		open,
		onClose,
		children,
	}: {
		open: boolean;
		onClose: () => void;
		children: Snippet;
	} = $props();

	function onKeyDown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') onClose();
	}

	// Back is the phone spelling of the Escape above, and every sheet in the
	// app comes through here — Tones, Laws, Rumors, feedback, a player's
	// retinue, the war drawer, and both of HelpButton's panels. Unlike the
	// chat panel and the Public Record this one is a dialog at every width,
	// so it takes part on desktop too.
	dismissOnBack(() => open, () => onClose());

	onMount(() => {
		window.addEventListener('keydown', onKeyDown);
		return () => window.removeEventListener('keydown', onKeyDown);
	});
</script>

{#if open}
	<div class="modal-backdrop backdrop" onclick={onClose} role="presentation"></div>
	<div class="sheet" role="dialog" aria-modal="true">
		<div class="sheet-header">
			<button class="modal-close close" onclick={onClose} aria-label="Close">
				<svg
					viewBox="0 0 24 24"
					width="20"
					height="20"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M6 6l12 12M18 6L6 18" />
				</svg>
			</button>
		</div>
		<div class="sheet-body">
			{@render children()}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		z-index: 90;
		animation: fade-in 150ms ease-out;
	}

	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 91;
		max-height: 90dvh;
		background: var(--color-surface);
		border-radius: var(--radius-sheet) var(--radius-sheet) 0 0;
		box-shadow: 0 -8px 32px rgba(0, 0, 0, 0.5);
		display: flex;
		flex-direction: column;
		animation: slide-up 200ms ease-out;
	}

	/* The close button floats over the body's top-right corner, level with the
	   title (which is 44px tall for the same reason), so the header itself
	   takes no height. */
	.sheet-header {
		position: relative;
		flex-shrink: 0;
	}

	.close {
		position: absolute;
		top: var(--space-2);
		right: var(--space-2);
		z-index: 1;
	}

	.sheet-body {
		padding: var(--space-2) var(--space-4) var(--space-5);
		overflow-y: auto;
		min-height: 0;
	}

	/* From the chat dock up, trade the bottom-sheet idiom for a centered
	   dialog. Like every content column it caps at 440 (docs/STYLE_GUIDE.md
	   "Layout widths"). */
	@media (min-width: 790px) {
		.sheet {
			left: 50%;
			right: auto;
			bottom: 50%;
			transform: translate(-50%, 50%);
			width: min(440px, 92vw);
			max-height: 80dvh;
			border-radius: var(--radius-sheet);
			box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
			animation: pop-in 180ms ease-out;
		}
	}

	@keyframes fade-in {
		from { opacity: 0; }
		to { opacity: 1; }
	}
	@keyframes slide-up {
		from { transform: translateY(100%); }
		to { transform: translateY(0); }
	}
	@keyframes pop-in {
		from { transform: translate(-50%, 50%) scale(0.96); opacity: 0; }
		to { transform: translate(-50%, 50%) scale(1); opacity: 1; }
	}

	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): all three of
	   these are entrance motion, and the sheet arriving is the feedback — it
	   opens either way, it just doesn't travel to get there. Killing the
	   animation is enough on both layouts: the mobile sheet has no transform
	   of its own to fall back to, and the centered dialog already holds its
	   `translate(-50%, 50%)` statically, so neither lands off-position.

	   After the 790 block on purpose — same specificity, so source order is
	   what lets this override `pop-in`. */
	@media (prefers-reduced-motion: reduce) {
		.backdrop,
		.sheet { animation: none; }
	}
</style>
