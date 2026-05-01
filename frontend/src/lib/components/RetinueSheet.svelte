<!--
  Bottom-sheet wrapper for RetinueView. Slides up on mobile, centered on
  larger screens. Dismissed via ESC, backdrop tap, or close button.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';

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

	onMount(() => {
		window.addEventListener('keydown', onKeyDown);
		return () => window.removeEventListener('keydown', onKeyDown);
	});
</script>

{#if open}
	<div class="backdrop" onclick={onClose} role="presentation"></div>
	<div class="sheet" role="dialog" aria-modal="true">
		<div class="sheet-header">
			<span class="grabber" aria-hidden="true"></span>
			<button class="close" onclick={onClose} aria-label="Close">×</button>
		</div>
		<div class="sheet-body">
			{@render children()}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.55);
		z-index: 90;
		animation: fade-in 150ms ease-out;
	}

	.sheet {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 91;
		max-height: 85dvh;
		background: #1e1e1c;
		border-top: 1px solid #3a3a3a;
		border-radius: 14px 14px 0 0;
		display: flex;
		flex-direction: column;
		animation: slide-up 200ms ease-out;
	}

	.sheet-header {
		position: relative;
		padding: 0.5rem 0.75rem 0.25rem;
		flex-shrink: 0;
	}

	.grabber {
		display: block;
		width: 40px;
		height: 4px;
		margin: 0 auto;
		background: #555;
		border-radius: 2px;
	}

	.close {
		position: absolute;
		top: 0.25rem;
		right: 0.25rem;
		width: 44px;
		height: 44px;
		background: none;
		color: #aaa;
		font-size: 1.6rem;
		line-height: 1;
		padding: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 6px;
	}
	.close:hover { color: #e8e4d9; background: #2a2a2a; }
	.close:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

	.sheet-body {
		padding: 0.5rem 1rem 1.25rem;
		overflow-y: auto;
		min-height: 0;
	}

	@media (min-width: 700px) {
		.sheet {
			left: 50%;
			right: auto;
			bottom: 50%;
			transform: translate(-50%, 50%);
			width: min(560px, 92vw);
			max-height: 80dvh;
			border-radius: 14px;
			border: 1px solid #3a3a3a;
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
</style>
