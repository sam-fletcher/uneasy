<!-- MakeWar/WarStatus.svelte
  The two sides, and the way in for a player who is not in the war.

  Each side is headed by its leader ("otto’s side") and lists its members as
  plain lines with their shapes; nothing in a list is pressed, so it has no box.

  Below them a player outside the war chooses in two moves, like aid or
  interfere in a roll. A chip picks one of "Join otto", "Join Wren" or "Stay
  out", the pick's cost appears under the chips, and the main button, labelled
  with what it will do ("Join otto’s side"), commits it. Nothing is sent by the
  chip, so the cost is always read before the join call is made. The chips use
  no meaning colour and the cost is ordinary secondary text.

  "Stay out" is a note and sends nothing: the server doesn't know about it and
  the delay vote doesn't wait on it. It stays changeable, since the war can be
  joined while it lasts: after it the chips remain with "Stay out" picked, and
  picking a side starts a join as usual.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { joinWar, type Plan, type Player, type WarStateResponse } from '$lib/api';
	import Chip from '$lib/components/shared/Chip.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import { playerColor } from '$lib/playerColor';
	import { playerName } from '../shared';
	import { warStayOuts, stayOutOfWar } from '$lib/warStayOuts';
	import { warLeaders, sideLeader, joinChoices, type JoinChoiceID } from '$lib/warText';

	let { war, plan, players, currentPlayerID = null, amParticipant, begun, onChanged, setError }: {
		war: WarStateResponse;
		plan: Plan;
		players: Player[];
		currentPlayerID?: number | null;
		amParticipant: boolean;
		/** The war has a row: its delay vote is over, so joining now owes entry. */
		begun: boolean;
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	const planID = $derived(plan.id);
	let joinBusy = $state(false);

	// What the war ended on, in words. The raw value ("surrender") is an
	// internal key; players read "a side surrendered".
	const END_REASON_TEXT: Record<string, string> = {
		surrender: 'a side surrendered',
		peace: 'peace was agreed',
	};

	// A surrender's claims keep the row gated after the war has ended. The
	// claimant sees their own form (SurrenderClaims); everyone else sees whose
	// choice the table is waiting on.
	const othersClaims = $derived(
		war.status === 'ended'
			? war.open_claims.filter(c => c.claimant_id !== currentPlayerID)
			: [],
	);

	const leaders = $derived(warLeaders(plan, players));
	const sides = $derived(
		([1, 2] as const).map(side => ({
			side,
			label: `${sideLeader(side, leaders)}’s side`,
			members: war.participants
				.filter(p => p.side === side)
				.map(p => ({ part: p, player: players.find(pl => pl.id === p.player_id) ?? null })),
		})),
	);

	// Per-session: has this player said they are staying out of this war? It
	// opens the chips with "Stay out" already picked, and swaps the cost and the
	// main button for the note. Picking a side afterwards undoes it on screen.
	const stayedOut = $derived($warStayOuts.has(planID));
	const showChoices = $derived(!amParticipant && war.status === 'active');

	const choices = $derived(joinChoices(war, plan, players, begun));
	let pick = $state<JoinChoiceID | null>(null);
	const picked = $derived(pick ?? (stayedOut ? 'stay' : null));
	const chosen = $derived(choices.find(c => c.id === picked) ?? null);
	const settledOut = $derived(picked === 'stay' && stayedOut);

	async function commit() {
		if (joinBusy || chosen == null) return;
		if (chosen.side == null) {
			stayOutOfWar(planID);
			return;
		}
		joinBusy = true; setError('');
		try {
			await joinWar(planID, chosen.side);
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not join war.');
		} finally { joinBusy = false; }
	}
</script>

<div class="war-sides">
	<div class="sides-grid">
		{#each sides as s (s.side)}
			<div class="side" data-side={s.side}>
				<div class="side-label">{s.label}</div>
				{#if s.members.length === 0}
					<div class="member"><span class="member-note">No one yet</span></div>
				{:else}
					{#each s.members as m (m.part.player_id)}
						<div class="member">
							<span class="member-name" style:color={m.player ? playerColor(m.player) : undefined}>
								<PlayerShape player={m.player} size={12} />
								<span>{playerName(players, m.part.player_id)}</span>
							</span>
							{#if m.part.surrendered_at_row != null}
								<span class="member-note">surrendered, row {m.part.surrendered_at_row}</span>
							{:else if !m.part.entry_payment_complete}
								<span class="member-note">joining, owes entry</span>
							{/if}
						</div>
					{/each}
				{/if}
			</div>
		{/each}
	</div>

	{#if showChoices}
		<div class="join">
			<p class="join-lede">You’re not in this war.</p>
			<div class="join-chips" role="group" aria-label="Your choice">
				{#each choices as c (c.id)}
					<Chip selected={picked === c.id} disabled={joinBusy} onclick={() => (pick = c.id)}>
						{c.label}
					</Chip>
				{/each}
			</div>
			{#if chosen}
				{#if settledOut}
					<p class="join-cost">You’re staying out of this war. You can still join while it lasts.</p>
				{:else}
					<p class="join-cost">{chosen.cost}</p>
					<button type="button" class="action-btn primary join-main" onclick={commit} disabled={joinBusy}>
						{joinBusy ? '…' : chosen.button}
					</button>
				{/if}
			{:else}
				<button type="button" class="action-btn primary join-main" disabled>Choose an option</button>
			{/if}
		</div>
	{/if}

	{#if war.status === 'ended'}
		<div class="ended">
			<p class="ended-line">
				The war is over.
				{#if war.end_reason}({END_REASON_TEXT[war.end_reason] ?? war.end_reason}){/if}
				{#if war.ended_at_row != null} Ended on row {war.ended_at_row}.{/if}
			</p>
			{#each othersClaims as c (c.id)}
				<p class="ended-line">
					{playerName(players, c.claimant_id)} is choosing one of
					{playerName(players, c.surrendered_id)}'s assets to take.
				</p>
			{/each}
		</div>
	{/if}
</div>

<style>
	.war-sides {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	/* Two sides abreast, as in the mockup: each column is 150px at the narrowest
	   (a 344px column less the sheet's padding), enough for a shape and a name,
	   and a note wraps beneath its name. */
	.sides-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--space-4);
	}

	.side {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.side-label {
		padding-bottom: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}

	/* A member is a line, with a hairline above it. Nothing here is pressed. */
	.member {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-2);
		min-height: 2rem;
		padding: var(--space-1) 0;
		border-top: 1px solid var(--color-border);
		font-size: var(--text-small);
	}
	.member-name {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.member-note {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}

	/* The way in: set off from the sides by a rule, since it is a different job
	   (a choice to make, not a list to read). */
	.join {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding-top: var(--space-4);
		border-top: 1px solid var(--color-border);
	}
	.join-lede,
	.join-cost {
		margin: 0;
		font-size: var(--text-small);
		line-height: 1.5;
		color: var(--color-text-secondary);
	}
	.join-chips {
		display: flex;
		gap: var(--space-2);
	}
	/* Three chips share the row; a long name wraps inside its chip rather than
	   pushing the others off. */
	.join-chips :global(.chip) {
		flex: 1 1 0;
		min-width: 0;
		padding: 0 var(--space-1);
		text-align: center;
	}
	.join-main {
		align-self: stretch;
	}

	.ended {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.ended-line {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
</style>
