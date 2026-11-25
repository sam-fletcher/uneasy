<!--
  Quote.svelte — a player's own words in the middle of the app's: a rumor, a
  decree, a scene's text, a preparation note, a question. It owns the curly
  marks and the look (upright, never italic) and it never parses its text for
  names or assets: the app can't safely tell a name in free text from an
  ordinary word, so the words go in as written.

      <Quote text={notes} />

  It is an inline span: put it inside the paragraph or line that lays it out
  (the caller's own class), so layout stays with the caller.

  Chat is the other place players' words appear; it keeps its own byline and
  mask-cast and doesn't go through this.
-->
<script lang="ts">
	import '$lib/components/shared/inlineText.css';

	interface Props {
		text: string;
	}

	const { text }: Props = $props();
</script>

<span class="quote">“{text}”</span>
