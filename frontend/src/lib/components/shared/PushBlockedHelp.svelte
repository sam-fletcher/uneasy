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
	.push-blocked {
		border-left: 2px solid var(--color-warning-border);
		padding: 0.1rem 0 0.1rem 0.75rem;
		margin: 0.15rem 0;
	}
	/* Kept as block flow rather than a flex column: a flex item's marker box is
	   inconsistent across engines, and the step numbers are the point here. */
	ol {
		margin: 0;
		padding-left: 1.1rem;
		list-style: decimal;
		color: var(--color-text);
		font-size: 0.85rem;
		line-height: 1.4;
	}
	li + li {
		margin-top: 0.3rem;
	}
	.note {
		margin: 0.5rem 0 0;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		line-height: 1.4;
	}
</style>
