<!-- CardPicker.svelte
  Wraps the recurring pattern:

      <FormField label="...">
          {#if items.length === 0}
              <p class="choices-note muted">EMPTY</p>
          {:else}
              <div class="peer-cards">
                  {#each items as a (a.id)}
                      <AssetCardSelectable
                          asset={a}
                          ownerColor={playerColor(players.find(p => p.id === a.owner_id))}
                          selectable / marginaliaSelectable
                          selected / selectedMarginaliaID
                          onToggle / onMarginaliaToggle
                      />
                  {/each}
              </div>
          {/if}
      </FormField>

  Three modes, picked by which selection prop is passed:

    SINGLE (default)     selected: number | null
                         onSelect: (id: number | null) => void
                         — tapping the selected card calls onSelect(null).

    MULTI (set `multi`)  selected: number[]
                         onSelect: (ids: number[]) => void
                         — toggles membership. Optional `max` caps the set;
                           taps on un-selected cards beyond the cap are no-ops.

    MARGINALIA           marginaliaMode + selectedMarginaliaID + onSelectMarginalia
                         (mID: number | null, asset: Asset | null) => void
                         — items should be assets with intact marginalia
                           (use shared.ts:assetsWithIntactMarginalia).

  `ownerLabel` is either a static string (rare; usually omitted) or a per-item
  function returning the subtitle to render under the asset name — used for
  "hidden d3", "already leveraged", "Owned by Alice", etc.

  Modes are mutually exclusive at call sites. The component does not enforce
  that statically; in practice each consumer uses one mode at a time.
-->
<script lang="ts">
	import type { Asset, Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { useSecretCounts } from '$lib/secretCountsContext';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import FormField from './FormField.svelte';

	interface Props {
		label: string;
		items: Asset[];
		players: Player[];
		/** Shown when items is empty. Default: "No eligible assets." */
		emptyMessage?: string;
		/** Per-card subtitle. String or function of the asset. */
		ownerLabel?: string | ((a: Asset) => string | undefined);

		// SINGLE-select (default).
		selected?: number | null;
		onSelect?: (id: number | null) => void;

		// MULTI-select — pass `multi`, use selectedMulti + onSelectMulti instead.
		multi?: boolean;
		max?: number;
		selectedMulti?: number[];
		onSelectMulti?: (ids: number[]) => void;

		// MARGINALIA mode
		marginaliaMode?: boolean;
		selectedMarginaliaID?: number | null;
		onSelectMarginalia?: (mID: number | null, asset: Asset | null) => void;

		/** When true, render selection state but block all interaction
		 *  (used by non-focus prep viewers mirroring the focus player). */
		readOnly?: boolean;
	}

	let {
		label,
		items,
		players,
		emptyMessage = 'No eligible assets.',
		ownerLabel,
		selected,
		onSelect,
		multi = false,
		max,
		selectedMulti,
		onSelectMulti,
		marginaliaMode = false,
		selectedMarginaliaID,
		onSelectMarginalia,
		readOnly = false,
	}: Props = $props();

	function resolveOwnerLabel(a: Asset): string | undefined {
		return typeof ownerLabel === 'function' ? ownerLabel(a) : ownerLabel;
	}

	// This is the asset-card seam for plan pickers: read the per-viewer known
	// counts here so the leaf card can show the secret eyes without every plan
	// panel threading the data. Undefined outside a provider → no eyes.
	const secretCounts = useSecretCounts();

	function ownerColorFor(a: Asset): string {
		return playerColor(players.find(p => p.id === a.owner_id));
	}

	function isPickedSingle(a: Asset): boolean {
		return selected === a.id;
	}

	function isPickedMulti(a: Asset): boolean {
		return selectedMulti?.includes(a.id) ?? false;
	}

	function disabledMulti(a: Asset): boolean {
		// Cap-reached: un-picked cards become disabled until the user
		// deselects something. Avoids silent rotation.
		return (
			multi &&
			max != null &&
			(selectedMulti?.length ?? 0) >= max &&
			!(selectedMulti?.includes(a.id) ?? false)
		);
	}

	function handleToggle(a: Asset) {
		if (multi) {
			const cur = selectedMulti ?? [];
			const next = cur.includes(a.id)
				? cur.filter(id => id !== a.id)
				: (max == null || cur.length < max ? [...cur, a.id] : cur);
			if (next !== cur) onSelectMulti?.(next);
			return;
		}
		// single-select with tap-again-deselect
		onSelect?.(selected === a.id ? null : a.id);
	}

	function handleMarginaliaToggle(mID: number, asset: Asset) {
		if (selectedMarginaliaID === mID) {
			onSelectMarginalia?.(null, null);
		} else {
			onSelectMarginalia?.(mID, asset);
		}
	}
</script>

<FormField {label}>
	{#if items.length === 0}
		<p class="choices-note muted">{emptyMessage}</p>
	{:else}
		<div class="peer-cards">
			{#each items as a (a.id)}
				{#if marginaliaMode}
					<AssetCardSelectable
						asset={a}
						ownerColor={ownerColorFor(a)}
						ownerLabel={resolveOwnerLabel(a)}
						knownSecretCount={secretCounts?.known(a.id)}
						marginaliaSelectable
						selectedMarginaliaID={selectedMarginaliaID ?? null}
						onMarginaliaToggle={handleMarginaliaToggle}
					/>
				{:else}
					<AssetCardSelectable
						asset={a}
						ownerColor={ownerColorFor(a)}
						ownerLabel={resolveOwnerLabel(a)}
						knownSecretCount={secretCounts?.known(a.id)}
						selectable
						selected={multi ? isPickedMulti(a) : isPickedSingle(a)}
						disabled={readOnly || disabledMulti(a)}
						onToggle={() => handleToggle(a)}
					/>
				{/if}
			{/each}
		</div>
	{/if}
</FormField>
