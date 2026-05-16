<!-- BattleCostForm.svelte
  Shared cost-of-battle picker used in two places:
   - cost mode: per-row cost-of-battle (break / leverage / propose peace),
     with optional surrender-after-payment toggle.
   - entry mode: late-joiner war entry payment (break / leverage only).
-->
<script lang="ts">
	import type { Player, Asset } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import AssetCardSelectable from '../../AssetCardSelectable.svelte';
	import PlayerChips from '../PlayerChips.svelte';

	export type BattleSubmission =
		| { kind: 'peace'; terms: string }
		| { kind: 'battle'; opponent_id: number; choice: 'break_asset'; marginalia_id: number; surrender: boolean }
		| { kind: 'battle'; opponent_id: number; choice: 'leverage_two'; asset_id_1: number; asset_id_2: number; surrender: boolean };

	interface Props {
		mode: 'cost' | 'entry';
		formKey: string | number;        // disambiguates radio names when multiple forms coexist
		opponents: number[];
		players: Player[];
		/** Assets owned by the current player with at least one intact
		 *  marginalium. Each intact marginalium becomes a tap target via
		 *  AssetCardSelectable's marginalia-pick mode. */
		marginaliaAssets: Asset[];
		/** Current player's unleveraged assets — candidates for leverage_two. */
		unleveraged: Asset[];
		allowPeace: boolean;
		allowSurrender: boolean;
		submitLabel?: string;            // defaults inferred from mode/kind
		onSubmit: (s: BattleSubmission) => Promise<void>;
	}

	let {
		mode, formKey, opponents, players, marginaliaAssets, unleveraged,
		allowPeace, allowSurrender, submitLabel, onSubmit,
	}: Props = $props();

	type Kind = 'break_asset' | 'leverage_two' | 'propose_peace';
	let opponentID = $state<number | null>(null);
	let kind = $state<Kind>('break_asset');
	let marginaliaID = $state<number | null>(null);
	let leverageIDs = $state<number[]>([]);
	let surrender = $state(false);
	let peaceTerms = $state('');
	let busy = $state(false);

	const submittable = $derived.by(() => {
		if (kind === 'propose_peace') return peaceTerms.trim().length > 0;
		if (opponentID == null) return false;
		if (kind === 'break_asset') return marginaliaID != null;
		return leverageIDs.length === 2;
	});

	function reset() {
		marginaliaID = null;
		leverageIDs = [];
		surrender = false;
		peaceTerms = '';
	}

	function setKind(k: Kind) {
		kind = k;
		reset();
	}

	function toggleLeverage(assetID: number) {
		if (leverageIDs.includes(assetID)) {
			leverageIDs = leverageIDs.filter(id => id !== assetID);
		} else if (leverageIDs.length < 2) {
			leverageIDs = [...leverageIDs, assetID];
		}
		// At the cap, taps on un-selected cards are no-ops — the user
		// must un-select one to pick another. Avoids silent rotation.
	}

	async function submit() {
		if (!submittable || busy) return;
		busy = true;
		try {
			let s: BattleSubmission;
			if (kind === 'propose_peace') {
				s = { kind: 'peace', terms: peaceTerms.trim() };
			} else if (kind === 'break_asset') {
				s = {
					kind: 'battle', opponent_id: opponentID!, choice: 'break_asset',
					marginalia_id: marginaliaID!, surrender: allowSurrender && surrender,
				};
			} else {
				s = {
					kind: 'battle', opponent_id: opponentID!, choice: 'leverage_two',
					asset_id_1: leverageIDs[0], asset_id_2: leverageIDs[1],
					surrender: allowSurrender && surrender,
				};
			}
			await onSubmit(s);
			reset();
		} finally {
			busy = false;
		}
	}

	const defaultLabel = $derived(
		kind === 'propose_peace' ? 'Propose peace'
			: mode === 'entry' ? 'Pay entry against this opponent'
			: 'Pay cost',
	);

	const opponentPlayers = $derived(
		opponents.map(id => players.find(p => p.id === id)).filter((p): p is Player => p != null),
	);
</script>

{#if opponents.length > 0}
	<div class="form-label">
		<span class="form-label-text">Opponent:</span>
		<PlayerChips
			players={opponentPlayers}
			isActive={(p) => opponentID === p.id}
			onSelect={(p) => (opponentID = opponentID === p.id ? null : p.id)}
		/>
	</div>
{/if}

<div class="form-label">
	<span class="form-label-text">How will you pay?</span>
	<div class="chip-row">
		<button type="button" class="chip-btn"
			class:active={kind === 'break_asset'}
			onclick={() => setKind('break_asset')}>Break an asset</button>
		<button type="button" class="chip-btn"
			class:active={kind === 'leverage_two'}
			onclick={() => setKind('leverage_two')}>Leverage two</button>
		{#if allowPeace}
			<button type="button" class="chip-btn"
				class:active={kind === 'propose_peace'}
				onclick={() => setKind('propose_peace')}>Propose peace</button>
		{/if}
	</div>
	<p class="choices-note muted" style="margin:0.25rem 0 0;">
		{#if kind === 'break_asset'}
			Tear one of your marginalia.
		{:else if kind === 'leverage_two'}
			Leverage two of your un-leveraged assets.
		{:else}
			Open a vote on terms. If it doesn't pass unanimously you'll still
			need to pay using one of the options above.
		{/if}
	</p>
</div>

{#if kind === 'break_asset'}
	<div class="form-label">
		<span class="form-label-text">Marginalium to tear:</span>
		{#if marginaliaAssets.length === 0}
			<p class="choices-note muted">You have no intact marginalia.</p>
		{:else}
			<div class="peer-cards">
				{#each marginaliaAssets as a (a.id)}
					<AssetCardSelectable
						asset={a}
						ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
						marginaliaSelectable
						selectedMarginaliaID={marginaliaID}
						onMarginaliaToggle={(mID) => (marginaliaID = marginaliaID === mID ? null : mID)}
					/>
				{/each}
			</div>
		{/if}
	</div>
{:else if kind === 'leverage_two'}
	<div class="form-label">
		<span class="form-label-text">Pick two assets to leverage:</span>
		{#if unleveraged.length < 2}
			<p class="choices-note muted">You don't have two un-leveraged assets available.</p>
		{:else}
			<div class="peer-cards">
				{#each unleveraged as a (a.id)}
					<AssetCardSelectable
						asset={a}
						ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
						selectable
						selected={leverageIDs.includes(a.id)}
						disabled={!leverageIDs.includes(a.id) && leverageIDs.length >= 2}
						onToggle={() => toggleLeverage(a.id)}
					/>
				{/each}
			</div>
		{/if}
	</div>
{:else}
	<label class="form-label">
		Peace terms:
		<textarea rows={3} bind:value={peaceTerms} class="form-textarea"
			placeholder="Describe the terms you propose…"></textarea>
	</label>
{/if}

{#if allowSurrender}
	<label class="form-label" style="display:flex;align-items:center;gap:0.5rem;"
		title={kind === 'propose_peace'
			? "Doesn't apply when proposing peace."
			: 'After this payment is recorded you will be marked surrendered. ' +
			  'Each opposing non-surrendered opponent will get to claim one of your assets.'}>
		<input type="checkbox" bind:checked={surrender}
			disabled={kind === 'propose_peace'} />
		<span class:muted={kind === 'propose_peace'}>
			Surrender after this payment
		</span>
	</label>
{/if}

<button class="action-btn primary" onclick={submit}
	disabled={busy || !submittable}>
	{busy ? '…' : (submitLabel ?? defaultLabel)}
</button>
