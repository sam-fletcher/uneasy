<!-- PhaseBadge.svelte — the game-phase badge, shared by the in-table header
  and the profile page's table cards so the two can't drift.

  Two-line, square-ish: the label's spaces render as line breaks ("MAIN
  EVENT" stacks) so it stays as narrow as a button and fits phone status
  rows. Violet = procedural information (ADR-009 role assignment). -->
<script lang="ts" module>
	const PHASE_LABELS: Record<string, string> = {
		lobby: 'Lobby',
		tone_setting: 'Tone Setting',
		prologue: 'Prologue',
		main_event: 'Main Event',
		shake_up: 'Shake-Up',
		ended: 'Game Over',
	};
</script>

<script lang="ts">
	let { phase }: { phase: string } = $props();
	const label = $derived(PHASE_LABELS[phase] ?? phase);
</script>

<span class="phase-badge">{#each label.split(' ') as word, i}{#if i}{' '}{/if}<span>{word}</span>{/each}</span>

<style>
	.phase-badge {
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.1em;
		min-height: 32px;
		/* Bottom padding is trimmed because capitals have no descenders — this
		   keeps the space above, between, and below the two words even. */
		padding: 0.3em 0.55em 0.2em;
		background: var(--color-chip-violet-bg);
		border: 1px solid var(--color-chip-violet-border);
		color: var(--color-chip-violet-text);
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-family: var(--font-serif);
		font-size: 0.76rem;
		line-height: 1;
		white-space: nowrap;
	}
</style>
