<!-- PushBlockedHelp.svelte
  What to do when the browser has notifications blocked for this origin.

  This is the one push state no code of ours can clear: there is no API to
  reset a permission and no way to prompt a second time (Chrome resolves the
  request 'denied' on the spot and shows its own blocked bubble). The switch
  is in browser settings, in a different place per browser — deriveBlockedHelp
  picks the branch, this renders it. Shown by the Profile push row and by the
  lobby soft-ask once an attempt has actually been refused.
-->
<script lang="ts">
	import { deriveBlockedHelp, isIOSDevice } from '$lib/push';

	const help = deriveBlockedHelp({
		ua: typeof navigator === 'undefined' ? '' : navigator.userAgent,
		isIOS: isIOSDevice(),
	});
</script>

<div class="push-blocked">
	<ol>
		{#each help.steps as step, i (i)}
			<li>{step}</li>
		{/each}
	</ol>
	{#if help.note}
		<p class="note">{help.note}</p>
	{/if}
</div>

<style>
	/* A thin warning rule down the left of the steps. It is kept as it was,
	   until the owner rules on it (docs/work/questions/push-blocked-rule.md). */
	.push-blocked {
		border-left: 2px solid var(--color-warning-border);
		padding: var(--space-1) 0 var(--space-1) var(--space-3);
		margin: var(--space-1) 0;
	}
	/* Kept as block flow rather than a flex column: a flex item's marker box is
	   inconsistent across engines, and the step numbers are the point here. */
	ol {
		margin: 0;
		padding-left: var(--space-4);
		list-style: decimal;
		color: var(--color-text);
		font-size: var(--text-small);
		line-height: 1.4;
	}
	li + li {
		margin-top: var(--space-1);
	}
	.note {
		margin: var(--space-2) 0 0;
		color: var(--color-text-secondary);
		font-size: var(--text-small);
		line-height: 1.4;
	}
</style>
