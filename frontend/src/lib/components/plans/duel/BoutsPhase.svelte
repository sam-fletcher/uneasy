<!-- Duel/BoutsPhase.svelte
  Bout loop with a visual tracker: each side's stakes (with their hidden /
  revealed dice) and the dice they've won so far, plus a pending pool for
  tied dice that carry to the next non-tie winner. Below it sits the current
  actor's declare/respond picker. Backend mirrors the carry-over logic in
  game.AccumulateDuelDice.

  Mobile-first: the two side panels stack on a narrow viewport and sit
  side-by-side from 560px up.
-->
<script lang="ts">
	import { boutDeclare, boutRespond, type Asset, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import FormField from '../FormField.svelte';
	import { playerName, assetName } from '../shared';
	import { computeAccumulated, type DuelRes } from './shared';

	let { plan, duelRes, players, assets, currentPlayerID, amParticipant, preparerStakes, targetStakes, bouts, myUnresolvedStakes, onPlansChanged, onRefresh }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		amParticipant: boolean;
		preparerStakes: DuelStake[];
		targetStakes: DuelStake[];
		bouts: DuelBout[];
		myUnresolvedStakes: DuelStake[];
		onPlansChanged: () => void;
		onRefresh: () => Promise<void> | void;
	} = $props();

	const accumulated = $derived(computeAccumulated(bouts, plan.preparer_id));

	const latestBout = $derived(bouts.length === 0 ? null : bouts[bouts.length - 1]);
	const boutInProgress = $derived(latestBout != null && latestBout.resolved_at == null);

	const currentActorID = $derived(
		boutInProgress ? (latestBout?.responder_id ?? null)
			: duelRes.initiativeID,
	);
	const isMyTurn = $derived(
		amParticipant && currentPlayerID != null && currentActorID === currentPlayerID,
	);

	// ── Stake → display view (die value, resolution status) ──────────────────
	type StakeView = {
		id: number;
		name: string;
		die: number | null;   // revealed value, or my own hidden value
		mine: boolean;
		hidden: boolean;       // true when value is concealed from the viewer
		resolved: boolean;
		setAside: boolean;     // tied bout — die carries forward
		won: boolean | null;
	};
	function viewStake(s: DuelStake): StakeView {
		const name = assetName(assets, s.asset_id);
		const mine = currentPlayerID != null && s.player_id === currentPlayerID;
		if (s.is_resolved) {
			for (const b of bouts) {
				if (b.declarer_stake_id === s.id && b.declarer_die != null) {
					return { id: s.id, name, die: b.declarer_die, mine, hidden: false,
						resolved: true, setAside: b.is_match, won: s.is_winner };
				}
				if (b.responder_stake_id === s.id && b.responder_die != null) {
					return { id: s.id, name, die: b.responder_die, mine, hidden: false,
						resolved: true, setAside: b.is_match, won: s.is_winner };
				}
			}
			return { id: s.id, name, die: null, mine, hidden: false,
				resolved: true, setAside: false, won: s.is_winner };
		}
		// Unresolved: the owner sees their die; the opponent sees nothing.
		return { id: s.id, name, die: s.hidden_die, mine,
			hidden: s.hidden_die == null, resolved: false, setAside: false, won: null };
	}
	const prepViews = $derived(preparerStakes.map(viewStake));
	const targViews = $derived(targetStakes.map(viewStake));

	// ── Declare / respond picker ─────────────────────────────────────────────
	let pickedStakeID = $state<number | null>(null);

	const boutStakeAssets = $derived(
		myUnresolvedStakes
			.map(s => assets.find(a => a.id === s.asset_id))
			.filter((a): a is NonNullable<typeof a> => a != null),
	);
	const pickedStakeAssetID = $derived(
		myUnresolvedStakes.find(s => s.id === pickedStakeID)?.asset_id ?? null,
	);
	function pickBoutStakeByAssetID(assetID: number | null) {
		const s = assetID == null ? null : myUnresolvedStakes.find(x => x.asset_id === assetID);
		pickedStakeID = s?.id ?? null;
	}
	function boutStakeLabel(a: { id: number }): string {
		const s = myUnresolvedStakes.find(x => x.asset_id === a.id);
		return s?.hidden_die != null ? `your die: ${s.hidden_die}` : 'hidden';
	}

	let pickedDeclaration = $state<'high' | 'low'>('high');
	let boutBusy = $state(false);
	let boutError = $state('');

	async function submitDeclare() {
		if (boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutDeclare(plan.id, pickedStakeID, pickedDeclaration);
			pickedStakeID = null;
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not declare bout.';
		} finally { boutBusy = false; }
	}
	async function submitRespond() {
		if (boutBusy || pickedStakeID == null) return;
		boutBusy = true; boutError = '';
		try {
			await boutRespond(plan.id, pickedStakeID);
			pickedStakeID = null;
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			boutError = e instanceof Error ? e.message : 'Could not respond.';
		} finally { boutBusy = false; }
	}
</script>

{#snippet die(value: number | string, kind: 'mine' | 'hidden' | 'aside' | 'won' | 'lost')}
	<span class="die" class:die-mine={kind === 'mine'} class:die-aside={kind === 'aside'}
		class:die-hidden={kind === 'hidden'}>{value}</span>
{/snippet}

{#snippet sideColumn(name: string, views: StakeView[], won: number[], isPreparer: boolean)}
	<div class="duel-side">
		<p class="side-name">{name}</p>
		<div class="stake-list">
			{#each views as v}
				<div class="stake-line" class:resolved={v.resolved}>
					<span class="stake-name">{v.name}</span>
					{#if v.resolved}
						{#if v.setAside}
							{@render die(v.die ?? '·', 'aside')}<span class="stake-tag">tied</span>
						{:else if v.won === true}
							{@render die(v.die ?? '·', 'won')}<span class="stake-tag win">won</span>
						{:else}
							{@render die(v.die ?? '·', 'lost')}<span class="stake-tag loss">lost</span>
						{/if}
					{:else if v.hidden}
						{@render die('?', 'hidden')}
					{:else}
						{@render die(v.die ?? '?', 'mine')}<span class="stake-tag">yours</span>
					{/if}
				</div>
			{/each}
		</div>
		<div class="won-row">
			<span class="won-label">Dice won</span>
			{#if won.length === 0}
				<span class="muted" style="font-style:normal;">none yet</span>
			{:else}
				{#each won as d}{@render die(d, 'won')}{/each}
			{/if}
		</div>
		<p class="won-feeds muted">
			{isPreparer ? 'feeds the actor pool' : 'feeds interference'}
		</p>
	</div>
{/snippet}

<div class="choices-section">
	<p class="choices-header">
		Bout {duelRes.currentBout + (boutInProgress ? 0 : 1)}
		· <strong>{playerName(players, currentActorID)}</strong>
		{#if boutInProgress}to respond{:else}to declare{/if}
	</p>

	<div class="duel-tracker">
		{@render sideColumn(playerName(players, plan.preparer_id), prepViews, accumulated.prep, true)}
		{@render sideColumn(playerName(players, plan.target_player_id), targViews, accumulated.targ, false)}
	</div>

	{#if accumulated.pending.length > 0}
		<p class="pending-row">
			<span class="won-label">Tied dice carried over</span>
			{#each accumulated.pending as d}{@render die(d, 'aside')}{/each}
			<span class="muted" style="font-style:normal;">go to the next bout's winner</span>
		</p>
	{/if}

	{#if latestBout && latestBout.resolved_at != null}
		<p class="choices-note last-bout">
			Last bout:
			{playerName(players, latestBout.declarer_id)}
			declared <strong>{latestBout.declaration}</strong>
			({latestBout.declarer_die}) vs
			{playerName(players, latestBout.responder_id)} ({latestBout.responder_die})
			{#if latestBout.is_match}
				→ tie, dice set aside
			{:else if latestBout.winner_id != null}
				→ <strong>{playerName(players, latestBout.winner_id)}</strong> wins both dice
			{/if}
		</p>
	{/if}

	{#if isMyTurn && myUnresolvedStakes.length > 0}
		<div class="plan-form" style="margin-top:0.5rem;">
			{#if boutInProgress}
				<p class="choices-note">
					Answer the challenge: pick one of your stakes. Its hidden die is
					compared against the declarer's — you can see your own value, but
					not theirs.
				</p>
			{:else}
				<p class="choices-note">
					You have initiative. Pick one of your stakes and call <strong>high</strong>
					or <strong>low</strong>: you're betting whether <em>your</em> hidden
					die will end up the higher or lower of the two revealed. Guess right
					and you win both dice; a tie sets them aside to carry forward.
				</p>
			{/if}
			<CardPicker
				label="Pick a stake"
				items={boutStakeAssets}
				{players}
				ownerLabel={boutStakeLabel}
				selected={pickedStakeAssetID}
				onSelect={pickBoutStakeByAssetID}
			/>
			{#if !boutInProgress}
				<FormField label="Call your die">
					<div class="chip-row">
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'high'}
							onclick={() => (pickedDeclaration = 'high')}
						>High</button>
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'low'}
							onclick={() => (pickedDeclaration = 'low')}
						>Low</button>
					</div>
				</FormField>
			{/if}
			{#if boutError}<p class="res-error">{boutError}</p>{/if}
			<button class="action-btn primary"
				onclick={boutInProgress ? submitRespond : submitDeclare}
				disabled={boutBusy || pickedStakeID == null}>
				{boutBusy ? '…' : boutInProgress ? 'Respond' : 'Declare'}
			</button>
		</div>
	{:else if amParticipant}
		<p class="choices-note muted">
			Waiting for {playerName(players, currentActorID)} to
			{boutInProgress ? 'respond' : 'declare'}…
		</p>
	{/if}
</div>

<style>
	.duel-tracker {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.6rem;
		margin: 0.5rem 0;
	}
	@media (min-width: 560px) {
		.duel-tracker { grid-template-columns: 1fr 1fr; }
	}

	.duel-side {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.5rem 0.6rem;
		background: var(--color-surface-2, #1d1d1d);
		border: 1px solid var(--color-border, #4a3a20);
		border-radius: 6px;
	}

	.side-name {
		margin: 0;
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.stake-list {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.stake-line {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.82rem;
		color: var(--color-text-muted);
	}
	.stake-line.resolved { opacity: 0.6; }

	.stake-name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.stake-tag {
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-faint);
	}
	.stake-tag.win { color: var(--color-success); }
	.stake-tag.loss { color: var(--color-danger); }

	.die {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.4rem;
		height: 1.4rem;
		padding: 0 0.2rem;
		border-radius: 4px;
		font-size: 0.8rem;
		font-weight: 700;
		background: #2a2418;
		border: 1px solid #5a4a2a;
		color: #e0d4b0;
		flex-shrink: 0;
	}
	.die-mine { border-color: var(--color-accent); color: var(--color-text); }
	.die-hidden { color: var(--color-text-faint); font-weight: 400; }
	.die-aside { border-style: dashed; opacity: 0.85; }

	.won-row, .pending-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin: 0;
		font-size: 0.82rem;
	}
	.pending-row {
		padding: 0.4rem 0.5rem;
		background: #1e1a10;
		border: 1px dashed #5a4a2a;
		border-radius: 5px;
	}

	.won-label {
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-accent);
	}

	.won-feeds { margin: 0; font-size: 0.72rem; }

	.last-bout { margin-top: 0.3rem; }
</style>
