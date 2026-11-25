<!-- plans/shared/SetPiece.svelte
  A moment the game stages: two named pieces brought face to face (Round 6, "Inside a
  step"). Clandestinely Liaise's meeting and Propose Duel's pairing are the first two.

      “They meet where the river bends,       the scene, if there is one: quoted, body size
       after vespers.”
         alice's Lady Vey                     each player's name in their colour, then
             MEETS                            their peer in italic deep gold at 18px;
         bob's Ser Ott                        the connector a small faint uppercase label

  It is centred and it bends three rules on purpose: centring is otherwise for
  structural boundaries, deep gold for headings and 18px for the step heading. That is
  why it is rare: a line earns it only when the game itself brings two named pieces
  together. It carries no difficulty line, because every plan that rolls already shows
  the difficulty meter in the open step until the dice land.

  The parts are the shared inline-text components: <PlayerName> for the player,
  <AssetName setPiece> for the peer, <Quote> for the scene.
-->
<script module lang="ts">
	/** One side of the pairing: a player and the peer they bring. */
	export interface Side {
		playerID: number | null;
		/** The peer they bring. Null for a player with no peer at all: their name stands alone. */
		peer: string | null;
	}
</script>

<script lang="ts">
	import '../planPanel.css';
	import type { Player } from '$lib/api';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import Quote from '$lib/components/shared/Quote.svelte';

	interface Props {
		players: Player[];
		a: Side;
		b: Side;
		/** "meets", "faces": said as a small uppercase label between the two. */
		connector: string;
		/** The scene the preparer wrote, quoted above the pairing. */
		scene?: string | null;
		/** Names the region for assistive tech. */
		label?: string;
	}
	const { players, a, b, connector, scene = null, label = 'The pairing' }: Props = $props();
</script>

{#snippet side(s: Side)}
	<span class="set-piece-side"><PlayerName {players} id={s.playerID} />{#if s.peer != null}<span class="set-piece-poss">'s</span>{' '}<AssetName name={s.peer} setPiece />{/if}</span>
{/snippet}

<section class="set-piece" aria-label={label}>
	{#if scene}<p class="set-piece-scene"><Quote text={scene} /></p>{/if}
	<div class="set-piece-pair">
		{@render side(a)}
		<span class="set-piece-connector">{connector}</span>
		{@render side(b)}
	</div>
</section>

<style>
	.set-piece {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		gap: var(--space-4);
		padding: var(--space-5) 0;
	}
	.set-piece-scene {
		margin: 0;
		max-width: 320px;
		font-size: var(--text-body);
		line-height: 1.5;
		color: var(--color-text);
		overflow-wrap: anywhere;
	}
	.set-piece-pair {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-1);
		max-width: 100%;
	}
	.set-piece-side {
		font-size: var(--text-body);
		overflow-wrap: anywhere;
	}
	.set-piece-poss { color: var(--color-text-secondary); }
	.set-piece-connector {
		font-size: var(--text-label);
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--color-text-faint);
	}
</style>
