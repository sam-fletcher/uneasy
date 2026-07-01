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
	import '$lib/components/shared/actionButton.css';
	import { boutDeclare, boutRespond, type Asset, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
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

	// Bouts run until the shorter side exhausts its stakes, so the total is
	// the smaller of the two full stake counts (each bout — win or tie —
	// resolves exactly one stake per side). Mirrors game.duel's loop bound.
	const totalBouts = $derived(Math.min(preparerStakes.length, targetStakes.length));

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

	// ── Declare / respond selection ──────────────────────────────────────────
	// The viewer picks a stake by tapping a row in their own side panel
	// (see the sideColumn snippet); pickedStakeID holds the chosen stake.
	let pickedStakeID = $state<number | null>(null);

	function togglePick(stakeID: number) {
		pickedStakeID = pickedStakeID === stakeID ? null : stakeID;
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

{#snippet sideColumn(sidePlayerID: number | null, views: StakeView[], won: number[], isPreparer: boolean)}
	{@const mine = currentPlayerID != null && sidePlayerID === currentPlayerID}
	{@const role = mine ? '(you)' : amParticipant ? '(opponent)' : ''}
	<div class="duel-side" class:is-mine={mine}>
		<p class="side-name">
			{playerName(players, sidePlayerID)}
			{#if role}<span class="side-role">{role}</span>{/if}
		</p>
		<div class="stake-list">
			{#each views as v}
				{#if isMyTurn && mine && !v.resolved}
					<button type="button" class="stake-line pick" class:selected={pickedStakeID === v.id}
						aria-pressed={pickedStakeID === v.id} onclick={() => togglePick(v.id)}>
						<span class="check">{pickedStakeID === v.id ? '✓' : ''}</span>
						<span class="stake-name">{v.name}</span>
						{@render die(v.die ?? '?', 'mine')}
					</button>
				{:else}
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
							{@render die(v.die ?? '?', 'mine')}
						{/if}
					</div>
				{/if}
			{/each}
		</div>
		<div class="won-row">
			<span class="won-label">
				{isPreparer ? 'Dice won for pool' : 'Dice won for interference'}
			</span>
			{#if won.length === 0}
				<span class="muted" style="font-style:normal;">none yet</span>
			{:else}
				{#each won as d}{@render die(d, 'won')}{/each}
			{/if}
		</div>
		<!-- <p class="won-feeds muted">
			{isPreparer ? 'feeds the actor pool' : 'feeds interference'}
		</p> -->
	</div>
{/snippet}

<div class="choices-section">
	<p class="choices-header">
		Bout {duelRes.currentBout + (boutInProgress ? 0 : 1)} of {totalBouts}
		· {playerName(players, currentActorID)}
		{#if boutInProgress}to respond{:else}to declare{/if}
	</p>

	<div class="duel-tracker">
		{@render sideColumn(plan.preparer_id, prepViews, accumulated.prep, true)}
		<div class="set-aside" class:empty={accumulated.pending.length === 0}>
			<span class="won-label">Set aside</span>
			{#if accumulated.pending.length === 0}
				<span class="muted" style="font-style:normal;">no tied dice</span>
			{:else}
				{#each accumulated.pending as d}{@render die(d, 'aside')}{/each}
				<span class="muted" style="font-style:normal;">→ next bout's winner</span>
			{/if}
		</div>
		{@render sideColumn(plan.target_player_id, targViews, accumulated.targ, false)}
	</div>

	<!-- {#if latestBout && latestBout.resolved_at != null}
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
	{/if} -->

	{#if isMyTurn && myUnresolvedStakes.length > 0}
		<div class="plan-form" style="margin-top:0.5rem;">
			<!-- {#if boutInProgress}
				<p class="choices-note">
					Tap one of your stakes above to answer the challenge. Its hidden
					die is compared against the declarer's — you can see your own
					value, but not theirs.
				</p>
			{:else}
				<p class="choices-note">
					Tap one of your stakes above, then call <em>high</em> or <em>low</em>.
					Guess right and you win both dice, plus any set aside.
				</p>
			{/if} -->
			{#if !boutInProgress}
				<FormField label="Will your die be higher or lower than your opponent's?">
					<div class="chip-row">
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'high'}
							onclick={() => (pickedDeclaration = 'high')}
						>Higher</button>
						<button
							type="button"
							class="chip-btn"
							class:active={pickedDeclaration === 'low'}
							onclick={() => (pickedDeclaration = 'low')}
						>Lower</button>
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
	/* Always stacked: preparer on top, the set-aside pool, then the target.
	   The vertical order mirrors the dice flow — tied dice sit literally
	   between the two players, waiting for the next winner. */
	.duel-tracker {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.4rem;
		margin: 0.5rem 0;
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
	.duel-side.is-mine { border-color: var(--color-accent); }

	.side-name {
		margin: 0;
		font-size: 0.85rem;
		color: var(--color-text);
	}
	.side-role {
		margin-left: 0.3rem;
		font-size: 0.72rem;
		font-weight: 400;
		color: var(--color-text-faint);
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

	/* Selectable variant: my own un-played stakes during my turn. A tap on
	   the row is the bout choice, replacing the old separate card picker. */
	.stake-line.pick {
		width: 100%;
		min-height: 2.75rem;       /* ≥44px tap target */
		padding: 0.35rem 0.45rem;
		background: transparent;
		border: 1px solid var(--color-border, #4a3a20);
		border-radius: 5px;
		color: var(--color-text);
		cursor: pointer;
		transition: border-color 0.12s, background 0.12s;
	}
	.stake-line.pick:hover { border-color: var(--color-accent); }
	.stake-line.pick.selected {
		border-color: var(--color-accent);
		background: color-mix(in srgb, var(--color-accent) 14%, transparent);
	}
	.check {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1rem;
		height: 1rem;
		flex-shrink: 0;
		border: 1px solid var(--color-border, #5a4a2a);
		border-radius: 3px;
		font-size: 0.7rem;
		line-height: 1;
		color: var(--color-accent);
	}
	.stake-line.pick.selected .check { border-color: var(--color-accent); }

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
		font-weight: 600;
		background: #2a2418;
		border: 1px solid #5a4a2a;
		color: #e0d4b0;
		flex-shrink: 0;
	}
	.die-mine { border-color: var(--color-accent); color: var(--color-text); }
	.die-hidden { color: var(--color-text-faint); font-weight: 400; }
	.die-aside { border-style: dashed; opacity: 0.85; }

	.won-row, .set-aside {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin: 0;
		font-size: 0.82rem;
	}
	/* Tied-dice pool, seated between the two stacked player panels. */
	.set-aside {
		padding: 0.4rem 0.6rem;
		background: #1e1a10;
		border: 1px dashed #5a4a2a;
		border-radius: 5px;
	}
	.set-aside.empty { opacity: 0.7; }

	.won-label {
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-accent);
	}

</style>
