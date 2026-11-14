<!-- Duel/BoutsPhase.svelte
  Bout loop with a visual tracker: each side's stakes (with their hidden /
  revealed dice) and the dice they've won so far, plus a pending pool for
  tied dice that carry to the next non-tie winner. Below it sits the current
  actor's declare/respond picker. Backend mirrors the carry-over logic in
  game.AccumulateDuelDice.

  Mobile-first: the two sides and the set-aside tray stack in one column, in
  the order the dice flow. Nothing here is boxed except what a player presses (a
  stake they can pick) and the tray, the duel's one object that holds dice.
-->
<script lang="ts">
	import EndsNote from '../shared/EndsNote.svelte';
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import Chip from '$lib/components/shared/Chip.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';
	import { playerColorByID } from '$lib/playerColor';
	import { boutDeclare, boutRespond, type Asset, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
	import FormField from '../FormField.svelte';
	import DieTile from '../shared/DieTile.svelte';
	import { playerName, assetName } from '../shared';
	import { computeAccumulated, setAsideDice, type DuelRes } from './shared';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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
	// Tied dice waiting in the tray, each with its owner, so its edge can be their colour.
	const trayDice = $derived(setAsideDice(bouts, plan.preparer_id));

	// Bouts run until the shorter side exhausts its stakes, so the total is
	// the smaller of the two full stake counts (each bout — win or tie —
	// resolves exactly one stake per side). Mirrors game.duel's loop bound.
	const totalBouts = $derived(Math.min(preparerStakes.length, targetStakes.length));

	const latestBout = $derived(bouts.length === 0 ? null : bouts[bouts.length - 1]);
	const boutInProgress = $derived(latestBout != null && latestBout.resolved_at == null);
	// A tie on the last bout has no later winner to hand the set-aside dice to.
	const isFinalBout = $derived(duelRes.currentBout + (boutInProgress ? 0 : 1) >= totalBouts);

	const currentActorID = $derived(
		boutInProgress ? (latestBout?.responder_id ?? null)
			: duelRes.initiativeID,
	);
	const isMyTurn = $derived(
		amParticipant && currentPlayerID != null && currentActorID === currentPlayerID,
	);

	// ── The open declaration ─────────────────────────────────────────────────
	// A declared, unresolved bout is a public call — which stake was opened
	// with, and "high" or "low" — that the responder is choosing an answer to.
	// The declarer's die stays hidden (the server masks it); the call does not.
	const openCall = $derived(
		boutInProgress && latestBout != null
			? {
				declarerID: latestBout.declarer_id,
				responderID: latestBout.responder_id,
				stakeID: latestBout.declarer_stake_id,
				declaration: latestBout.declaration,
			}
			: null,
	);
	// The UI says higher/lower everywhere (the declarer's own picker, below); the
	// wire value and the action log say high/low, the rules' word.
	const callWord = (d: 'high' | 'low' | null) => (d === 'low' ? 'lower' : 'higher');
	const openCallAssetName = $derived.by(() => {
		if (openCall == null) return '';
		const s = [...preparerStakes, ...targetStakes].find(x => x.id === openCall.stakeID);
		return s == null ? '' : assetName(assets, s.asset_id);
	});

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
		called: 'high' | 'low' | null;  // this is the open bout's declared stake
	};
	function viewStake(s: DuelStake): StakeView {
		const name = assetName(assets, s.asset_id);
		const mine = currentPlayerID != null && s.player_id === currentPlayerID;
		const called = openCall != null && openCall.stakeID === s.id
			? (openCall.declaration ?? 'high') : null;
		if (s.is_resolved) {
			for (const b of bouts) {
				if (b.declarer_stake_id === s.id && b.declarer_die != null) {
					return { id: s.id, name, die: b.declarer_die, mine, hidden: false,
						resolved: true, setAside: b.is_match, won: s.is_winner, called: null };
				}
				if (b.responder_stake_id === s.id && b.responder_die != null) {
					return { id: s.id, name, die: b.responder_die, mine, hidden: false,
						resolved: true, setAside: b.is_match, won: s.is_winner, called: null };
				}
			}
			return { id: s.id, name, die: null, mine, hidden: false,
				resolved: true, setAside: false, won: s.is_winner, called: null };
		}
		// Unresolved: the owner sees their die; the opponent sees nothing.
		return { id: s.id, name, die: s.hidden_die, mine,
			hidden: s.hidden_die == null, resolved: false, setAside: false, won: null, called };
	}
	// What a die's tile says to a screen reader: whose it is and where it stands.
	function dieLabel(v: StakeView, ownerName: string): string {
		if (v.resolved) {
			const how = v.setAside ? 'tied, set aside' : v.won === true ? 'won' : 'lost';
			return `${ownerName}'s die under ${v.name}, ${how}`;
		}
		return v.hidden
			? `${ownerName}'s hidden die under ${v.name}`
			: `Your die under ${v.name}`;
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

{#snippet sideColumn(sidePlayerID: number | null, views: StakeView[], won: number[], isPreparer: boolean)}
	{@const mine = currentPlayerID != null && sidePlayerID === currentPlayerID}
	{@const role = mine ? '(you)' : amParticipant ? '(opponent)' : ''}
	{@const color = playerColorByID(sidePlayerID, players)}
	{@const ownerName = playerName(players, sidePlayerID)}
	<section class="duel-side" class:is-mine={mine}>
		<p class="side-name">
			<PlayerShape player={players.find(p => p.id === sidePlayerID)} size={12} />
			<span style:color={color}>{ownerName}</span>
			{#if role}<span class="side-role">{role}</span>{/if}
		</p>
		<div class="stake-list">
			{#each views as v (v.id)}
				{#if isMyTurn && mine && !v.resolved}
					<!-- A stake of mine I can play: a control, so it has the control's border;
					     picked is the bright gold border and a tick, like any option card. -->
					<button type="button" class="choice-card stake-line pick" class:active={pickedStakeID === v.id}
						aria-pressed={pickedStakeID === v.id} onclick={() => togglePick(v.id)}>
						<span class="stake-name">{v.name}</span>
						<span class="die"><DieTile face={v.die} {color} label={dieLabel(v, ownerName)} /></span>
						{#if pickedStakeID === v.id}
							<span class="choice-picked"><ToneMark tone="success" label="Picked" /></span>
						{/if}
					</button>
				{:else}
					<div class="stake-line" class:resolved={v.resolved}>
						<span class="stake-name">{v.name}</span>
						{#if v.called}
							<span class="status-word stake-tag">called {callWord(v.called)}</span>
						{/if}
						{#if v.hidden && !v.resolved}
							<!-- Rolled, but not yours to read. The tile alone looks like a die that
							     has not been rolled, so the word says which. -->
							<span class="status-word stake-tag">hidden</span>
						{/if}
						{#if v.resolved}
							{#if v.setAside}
								<span class="status-word stake-tag">tied</span>
							{:else if v.won === true}
								<span class="status-word success stake-tag">won</span>
							{:else}
								<span class="status-word danger stake-tag">lost</span>
							{/if}
						{/if}
						<span class="die"><DieTile face={v.die} {color} label={dieLabel(v, ownerName)} duplicate={v.setAside} /></span>
					</div>
				{/if}
			{/each}
		</div>
		<div class="won-row">
			<span class="won-label">
				{isPreparer ? 'Dice won for pool' : 'Dice won for interference'}
			</span>
			{#if won.length === 0}
				<span class="muted">none yet</span>
			{:else}
				{#each won as d}<DieTile face={d} {color} label={`${ownerName}'s won die`} />{/each}
			{/if}
		</div>
	</section>
{/snippet}

<div class="choices-section">
	<p class="choices-header">
		Bout {duelRes.currentBout + (boutInProgress ? 0 : 1)} of {totalBouts}
		· {playerName(players, currentActorID)}
		{#if boutInProgress}to respond{:else}to declare{/if}
	</p>

	<div class="duel-tracker">
		{@render sideColumn(plan.preparer_id, prepViews, accumulated.prep, true)}
		<!-- The tray: where tied dice wait for the next bout's winner. -->
		<div class="set-aside">
			<span class="won-label">Set aside</span>
			{#if accumulated.pending.length === 0}
				<span class="muted">no tied dice</span>
			{:else}
				<div class="tray-dice">
					{#each trayDice as d}
						<DieTile face={d.die} color={playerColorByID(d.ownerID, players)}
							label={`${playerName(players, d.ownerID)}'s tied die, set aside`} duplicate />
					{/each}
				</div>
				<span class="muted">
					{isFinalBout ? "→ this bout's winner — lost if it ties" : "→ next bout's winner"}
				</span>
			{/if}
		</div>
		{@render sideColumn(plan.target_player_id, targViews, accumulated.targ, false)}
	</div>

	{#if openCall}
		<!-- The public call: what the responder must read before choosing. Information,
		     so an info notice. -->
		<Notice tone="info">
			{playerName(players, openCall.declarerID)}
			opens with <em>{openCallAssetName}</em>, calling
			{callWord(openCall.declaration)}.
			{#if currentPlayerID === openCall.responderID}
				Pick one of your stakes to answer — you can see your own die, not theirs.
			{:else}
				{playerName(players, openCall.responderID)} must answer.
			{/if}
		</Notice>
	{/if}

	{#if isMyTurn && myUnresolvedStakes.length > 0}
		<div class="plan-form">
			{#if !boutInProgress}
				<FormField label="Will your die be higher or lower than your opponent's?">
					<div class="chip-row">
						<Chip selected={pickedDeclaration === 'high'} onclick={() => (pickedDeclaration = 'high')}>Higher</Chip>
						<Chip selected={pickedDeclaration === 'low'} onclick={() => (pickedDeclaration = 'low')}>Lower</Chip>
					</div>
				</FormField>
			{/if}
			{#if boutError}<ErrorText message={boutError} variant="panel" />{/if}
			<button class="action-btn primary"
				onclick={boutInProgress ? submitRespond : submitDeclare}
				disabled={boutBusy || pickedStakeID == null}>
				{boutBusy ? '…' : boutInProgress ? 'Respond' : 'Declare'}
			</button>
			<EndsNote show={boutInProgress && isFinalBout} text="This is the last bout: the bouts end when you respond, and the duel's dice roll follows." />
		</div>
	{:else if amParticipant}
		<p class="choices-note muted">
			Waiting for {playerName(players, currentActorID)} to
			{boutInProgress ? 'respond' : 'declare'}…
		</p>
	{/if}
</div>

<style>
	/* Always stacked: preparer on top, the set-aside tray, then the target. The
	   vertical order mirrors the dice flow: tied dice sit literally between the
	   two players, waiting for the next winner. The sides are sections (a name and
	   some space), not boxes. */
	.duel-tracker {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.duel-side {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.side-name {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
		font-size: var(--text-body);
	}
	.side-role {
		font-size: var(--text-small);
		color: var(--color-text-faint);
	}

	.stake-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* A stake that is not mine to play is a plain line: its name, what has been
	   said of it, and its die. */
	.stake-line {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-body);
		color: var(--color-text);
	}
	.stake-line.resolved .stake-name { color: var(--color-text-secondary); }
	/* A stake of mine I can pick is the shared option card (planPanel.css); it
	   only needs its die to sit at the end. */
	.stake-line.pick { gap: var(--space-3); }
	.die { display: inline-flex; flex: none; }

	.stake-name {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	/* Where a stake stands: a status word, so small capitals beside its colour. */
	.stake-tag { flex: none; }
	/* A stake can carry two ("called higher", "hidden"); a dot between them keeps
	   them from reading as one phrase. */
	.stake-tag + .stake-tag::before {
		content: '·';
		margin-right: var(--space-2);
	}

	/* The tray is the one box left, since it is an object that holds dice. It keeps
	   the antique gold as a dashed rim and takes no fill and no fade. */
	.set-aside {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding: var(--space-3);
		border: 1px dashed var(--color-border-warm-antique);
		border-radius: var(--radius-piece);
		font-size: var(--text-small);
	}
	.tray-dice {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.won-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin: 0;
		font-size: var(--text-small);
	}
	/* A small label over its dice, like "For" and "Against" on the roll. */
	.won-label {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	
</style>
