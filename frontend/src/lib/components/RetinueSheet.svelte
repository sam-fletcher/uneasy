<!--
  THE sheet. A raised surface, rounded on its top corners, that rises over the
  screen with a scrim behind it and a soft shadow (the only shadow in the app).
  Dismissed via ESC, phone Back, backdrop tap, close button, or — when it is a
  bottom sheet — a swipe down ($lib/sheetSwipe). While it is open the page
  behind it can't be pulled to refresh (overscroll-behavior, below).

  Put a <h3 class="sheet-title"> first in the body (modalShell.css) and the
  sheet's content after it. Sheets never nest: opening one from inside another
  closes the first. Bright gold, a main button and the rest follow the same
  recipes as anywhere else; a control that stands on this surface and needs to
  read as raised again (an input) takes the raised-twice fill.

  WHERE IT OPENS. On the table page a sheet opens over the column that launched
  it and shades only that column, so the other panes stay readable and live: Tones over
  Chat, the war's panel over Play, and Help or a player's retinue over Play (or
  the left column when there are two). The column is found through context
  ($lib/sheetHost), measured when the sheet opens, and followed if the window
  resizes. On a phone there is one column, so the sheet covers the screen.
  Anywhere with no host (the profile page, the landing page) it covers the
  viewport: a bottom sheet on a phone and a centred dialog from two columns up.
-->
<script module lang="ts">
	// How many open sheets are holding the page's overscroll shut. A count, not
	// a flag, because a hand-off (one sheet closing as another opens in the same
	// click) can run the new sheet's setup before the old one's teardown.
	let overscrollLocks = 0;
	function lockPageOverscroll(): () => void {
		overscrollLocks++;
		document.documentElement.style.overscrollBehaviorY = 'none';
		return () => {
			overscrollLocks--;
			if (overscrollLocks <= 0) {
				overscrollLocks = 0;
				document.documentElement.style.removeProperty('overscroll-behavior-y');
			}
		};
	}
</script>

<script lang="ts">
	import '$lib/components/shared/modalShell.css';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { dismissOnBack } from '$lib/dismissOnBack.svelte';
	import { useSheetHost } from '$lib/sheetHost';
	import { swipeToDismiss } from '$lib/sheetSwipe';
	import { tableLayout } from '$lib/tableLayout.svelte';

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
	// app comes through here — Tones, the war's panel, a player's retinue,
	// Help and its feedback form. Unlike the chat panel and the Public Record
	// these are dialogs at every width, so they take part on desktop too.
	dismissOnBack(() => open, () => onClose());

	// Stop the browser's pull-to-refresh taking a downward drag while a sheet is
	// open. `.sheet-body { overscroll-behavior: contain }` keeps a scrolling body
	// from chaining to the page, but Chrome doesn't trap the overscroll of a body
	// too short to scroll, so the page itself is told not to overscroll either.
	// Runs on open and tears down on close or destroy.
	$effect(() => {
		if (!open) return;
		return lockPageOverscroll();
	});

	onMount(() => {
		window.addEventListener('keydown', onKeyDown);
		return () => window.removeEventListener('keydown', onKeyDown);
	});

	// The sheet is drawn at the end of <body>, not where it is declared, for two
	// reasons. A pane that isn't showing is `visibility: hidden`, and visibility is
	// inherited: the Tones sheet that the lobby's checklist opens while a phone is
	// showing Play is declared inside the Chat column, so without this it would
	// open invisibly. And its box is measured against the viewport, so it has to
	// be placed against the viewport: a size-query container (the play column is
	// one) can be the containing block of fixed descendants in some engines,
	// which would shift it by the column's own offset. Context is looked up where
	// the component is declared, so moving the node loses nothing.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	// ── Swipe down to dismiss ─────────────────────────────────────────────
	// Only a bottom sheet can be dragged away: every sheet on a phone, and a
	// hosted sheet (over a column) at any width. The unhosted centred dialog
	// from two columns up is a dialog, not a sheet, so it is left alone.
	// Matches the `.scope:not(.hosted)` breakpoint rule in the styles below.
	const draggable = () => tableLayout.mode === 'phone' || hostBox !== null;
	let sheetEl = $state<HTMLElement>();
	let scopeEl = $state<HTMLElement>();
	const swipeOptions = () => ({
		enabled: draggable,
		onClose,
		scrim: () => scopeEl?.querySelector<HTMLElement>('.backdrop') ?? null,
	});

	// ── Which column to cover ─────────────────────────────────────────────
	const sheetHost = useSheetHost();

	/** The hosting column's box in viewport coordinates, or null to cover the
	 *  whole viewport. Only set while the sheet is open. */
	let hostBox = $state<{ left: number; top: number; width: number; height: number } | null>(null);

	// $effect.pre, not $effect: it runs before the DOM updates, so the box is
	// known when the sheet first renders. A post-render effect would let the
	// first render use the unhosted (full-screen) layout and snap to the column
	// afterwards.
	$effect.pre(() => {
		if (!open) {
			hostBox = null;
			return;
		}
		// A phone has one column, which is the screen.
		const host = tableLayout.mode === 'phone' ? null : (sheetHost?.() ?? null);
		if (!host) {
			hostBox = null;
			return;
		}
		const measure = () => {
			const r = host.getBoundingClientRect();
			hostBox = { left: r.left, top: r.top, width: r.width, height: r.height };
		};
		measure();
		// The window can be resized (or turned) with a sheet open.
		const observer = new ResizeObserver(measure);
		observer.observe(host);
		window.addEventListener('resize', measure);
		return () => {
			observer.disconnect();
			window.removeEventListener('resize', measure);
		};
	});
</script>

{#if open}
	<!-- The scope is the area the sheet may cover: a column, or the viewport.
	     It passes touches through; its two children take them. -->
	<div
		use:portal
		bind:this={scopeEl}
		class="scope"
		class:hosted={hostBox !== null}
		style:left={hostBox ? `${hostBox.left}px` : undefined}
		style:top={hostBox ? `${hostBox.top}px` : undefined}
		style:width={hostBox ? `${hostBox.width}px` : undefined}
		style:height={hostBox ? `${hostBox.height}px` : undefined}
	>
		<div class="modal-backdrop backdrop" onclick={onClose} role="presentation"></div>
		<div
				class="sheet"
				role="dialog"
				aria-modal="true"
				bind:this={sheetEl}
				use:swipeToDismiss={swipeOptions()}
			>
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
	</div>
{/if}

<style>
	.scope {
		position: fixed;
		inset: 0;
		z-index: 90;
		pointer-events: none;
	}
	/* A hosted scope has its box set inline, so the viewport inset is dropped. */
	.scope.hosted { inset: auto; }

	/* modalShell.css gives the scrim `position: fixed; inset: 0`; inside the
	   scope it covers the scope instead. */
	.backdrop {
		position: absolute;
		pointer-events: auto;
		animation: fade-in 150ms ease-out;
	}

	.sheet {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		max-height: 90%;
		pointer-events: auto;
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
		/* A scroll that reaches the end stops here instead of carrying on to the
		   page, where on a phone it would pull-to-refresh the whole app. */
		overscroll-behavior: contain;
		min-height: 0;
	}

	/* With no column to cover, from two columns up a sheet is a centred dialog
	   rather than a bottom sheet stretched across a wide window. Like every
	   content column it caps at 440 (docs/STYLE_GUIDE.md "Layout widths"). */
	@media (min-width: 744px) {
		.scope:not(.hosted) .sheet {
			left: 50%;
			right: auto;
			bottom: 50%;
			transform: translate(-50%, 50%);
			width: min(440px, 92vw);
			max-height: 80%;
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
	   animation is enough on every layout: the bottom sheet has no transform
	   of its own to fall back to, and the centred dialog already holds its
	   `translate(-50%, 50%)` statically, so neither lands off-position.

	   After the breakpoint block on purpose — same specificity, so source
	   order is what lets this override `pop-in`. */
	@media (prefers-reduced-motion: reduce) {
		.backdrop,
		.sheet { animation: none; }
	}
</style>
