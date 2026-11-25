<!-- plans/shared/QuotedText.svelte
  The player-written text an open step is about, centred (Round 6, "Inside a step"):
  the decree in debate, the rumor, the law being written.

          ALICE PROPOSES              a small deep-gold label; names the text and
                                      the player, whose name is in their colour
     “No private armies may be         16px main text, in its own quotation marks,
      raised within the city walls.”   at most 320px wide

  Centring is for what the step is ABOUT. A finished step keeps it to one line (the
  FinishedStep line), and text being TYPED stays left-aligned in its field: this is
  only for text that is already written and is being read, so it never wraps a
  form. A player's own words go in as written through <Quote>, never parsed.

  The label is the caller's snippet, so a name inside it is a <PlayerName> like
  anywhere: `{#snippet label()}<PlayerName {players} id={…} /> proposes{/snippet}`.
-->
<script lang="ts">
	import '../planPanel.css';
	import type { Snippet } from 'svelte';
	import Quote from '$lib/components/shared/Quote.svelte';

	interface Props {
		/** What the text is, and whose: "alice proposes", "The rumor". */
		label: Snippet;
		text: string;
		/** A line about the text, under it, in secondary words ("How it's spreading: …"). */
		note?: string | null;
		/** A second player-written part joined to the first by the app's own word: a law and its
		 *  addendum ("— and …"). The connector is the app's, so it stays outside both quotes. */
		then?: { connector: string; text: string } | null;
	}
	const { label, text, note, then = null }: Props = $props();
</script>

<div class="quoted-text">
	<p class="choices-header">{@render label()}</p>
	<p class="quoted-body"><Quote {text} />{#if then}{' '}— {then.connector}{' '}<Quote text={then.text} />{/if}</p>
	{#if note}<p class="quoted-note">{note}</p>{/if}
</div>

<style>
	.quoted-text {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		gap: var(--space-2);
		padding: var(--space-2) 0;
	}
	.quoted-note {
		margin: 0;
		max-width: 320px;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		overflow-wrap: anywhere;
	}
	.quoted-body {
		margin: 0;
		max-width: 320px;
		font-size: var(--text-body);
		line-height: 1.5;
		color: var(--color-text);
		overflow-wrap: anywhere;
	}
</style>
