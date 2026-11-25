<!--
  AssetName.svelte — an asset's (or a peer's) name in text. Italic, in the
  colour of the text around it, never the owner's. Italic means an asset and
  nothing else, so this is the only place an `<em>` for a name is written; a
  test fails a panel that writes one by hand (inlineText.test.ts).

  Not for an asset's own card or sheet title (the card is the thing, not a
  mention of it: that stays upright), nor inside an <input> or <option>, which
  phone browsers don't reliably style.

      <AssetName name={a.name} />
      <AssetName {assets} id={assetID} />     looked up; "?" if it isn't there
      <AssetName name={p.name} setPiece />    the set piece's peers: deep gold, 18px

  `setPiece` is the one place an asset takes a colour of its own: a set piece
  (plans/shared/SetPiece.svelte) puts the two peers it stages in italic deep gold,
  at the heading size. Everywhere else an asset takes the colour around it.
-->
<script lang="ts">
	import '$lib/components/shared/inlineText.css';
	import type { Asset } from '$lib/api';

	interface Props {
		name?: string | null;
		assets?: Pick<Asset, 'id' | 'name'>[];
		id?: number | null;
		/** The deep-gold, heading-size peers of a set piece. */
		setPiece?: boolean;
	}

	const { name, assets, id, setPiece = false }: Props = $props();

	const text = $derived(
		name ?? (id != null && assets ? assets.find((a) => a.id === id)?.name : undefined) ?? '?',
	);
</script>

<em class={["asset-name", setPiece && "set-piece-peer"]}>{text}</em>
