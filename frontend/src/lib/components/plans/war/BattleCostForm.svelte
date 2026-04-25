<!-- BattleCostForm.svelte
  Shared cost-of-battle picker used in two places:
   - cost mode: per-row cost-of-battle (break / leverage / propose peace),
     with optional surrender-after-payment toggle.
   - entry mode: late-joiner war entry payment (break / leverage only).
-->
<script lang="ts">
	import type { Player, Asset } from '$lib/api';
	import { playerName } from '../shared';

	export type BattleSubmission =
		| { kind: 'peace'; terms: string }
		| { kind: 'battle'; opponent_id: number; choice: 'break_asset'; marginalia_id: number; surrender: boolean }
		| { kind: 'battle'; opponent_id: number; choice: 'leverage_two'; asset_id_1: number; asset_id_2: number; surrender: boolean };

	interface MarginaliumOption { id: number; label: string; }

	interface Props {
		mode: 'cost' | 'entry';
		formKey: string | number;        // disambiguates radio names when multiple forms coexist
		opponents: number[];
		players: Player[];
		marginalia: MarginaliumOption[];
		unleveraged: Asset[];
		allowPeace: boolean;
		allowSurrender: boolean;
		submitLabel?: string;            // defaults inferred from mode/kind
		onSubmit: (s: BattleSubmission) => Promise<void>;
	}

	let {
		mode, formKey, opponents, players, marginalia, unleveraged,
		allowPeace, allowSurrender, submitLabel, onSubmit,
	}: Props = $props();

	type Kind = 'break_asset' | 'leverage_two' | 'propose_peace';
	let opponentID = $state<number | null>(null);
	let kind = $state<Kind>('break_asset');
	let marginaliaID = $state<number | null>(null);
	let asset1 = $state<number | null>(null);
	let asset2 = $state<number | null>(null);
	let surrender = $state(false);
	let peaceTerms = $state('');
	let busy = $state(false);

	const submittable = $derived.by(() => {
		if (kind === 'propose_peace') return peaceTerms.trim().length > 0;
		if (opponentID == null) return false;
		if (kind === 'break_asset') return marginaliaID != null;
		return asset1 != null && asset2 != null && asset1 !== asset2;
	});

	function reset() {
		marginaliaID = null;
		asset1 = null;
		asset2 = null;
		surrender = false;
		peaceTerms = '';
	}

	function setKind(k: Kind) {
		kind = k;
		reset();
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
					asset_id_1: asset1!, asset_id_2: asset2!,
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
</script>

<label class="form-label">
	Opponent:
	<select bind:value={opponentID} class="form-textarea" style="height:auto;">
		<option value={null}>— pick {mode === 'entry' ? 'an' : 'the'} opponent —</option>
		{#each opponents as id}
			<option value={id}>{playerName(players, id)}</option>
		{/each}
	</select>
</label>

<div class="choice-list">
	<label class="choice-item">
		<input type="radio" name="bcf-{mode}-{formKey}" value="break_asset"
			checked={kind === 'break_asset'}
			onchange={() => setKind('break_asset')} />
		<strong>Break an asset</strong> — tear one of your marginalia
	</label>
	<label class="choice-item">
		<input type="radio" name="bcf-{mode}-{formKey}" value="leverage_two"
			checked={kind === 'leverage_two'}
			onchange={() => setKind('leverage_two')} />
		<strong>Leverage two</strong> — leverage two of your un-leveraged assets
	</label>
	{#if allowPeace}
		<label class="choice-item">
			<input type="radio" name="bcf-{mode}-{formKey}" value="propose_peace"
				checked={kind === 'propose_peace'}
				onchange={() => setKind('propose_peace')} />
			<strong>Propose peace</strong> — open a vote on terms; if it doesn't
			pass unanimously you'll still need to pay using one of the options above
		</label>
	{/if}
</div>

{#if kind === 'break_asset'}
	<label class="form-label">
		Marginalia to tear:
		<select bind:value={marginaliaID} class="form-textarea" style="height:auto;">
			<option value={null}>— pick a marginalium —</option>
			{#each marginalia as m}
				<option value={m.id}>{m.label}</option>
			{/each}
		</select>
	</label>
{:else if kind === 'leverage_two'}
	<label class="form-label">
		First asset to leverage:
		<select bind:value={asset1} class="form-textarea" style="height:auto;">
			<option value={null}>— pick an asset —</option>
			{#each unleveraged as a}
				<option value={a.id} disabled={a.id === asset2}>{a.name}</option>
			{/each}
		</select>
	</label>
	<label class="form-label">
		Second asset to leverage:
		<select bind:value={asset2} class="form-textarea" style="height:auto;">
			<option value={null}>— pick an asset —</option>
			{#each unleveraged as a}
				<option value={a.id} disabled={a.id === asset1}>{a.name}</option>
			{/each}
		</select>
	</label>
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
