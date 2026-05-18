<!-- Festivity/GuestList.svelte
  Renders the esteem-ordered guest list with each guest's current status,
  plus a "Join as guest" button for non-guest non-host players during the
  socializing phase. Owns its esteem-rank derivation.
-->
<script lang="ts">
	import { joinFestivity, type Plan, type Player, type Ranking } from '$lib/api';
	import { playerName } from '../shared';
	import type { FestRes } from './options';

	let { plan, fest, players, rankings, currentPlayerID, amHost, iAmGuest, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		amHost: boolean;
		iAmGuest: boolean;
		onPlansChanged: () => void;
	} = $props();

	function esteemRank(playerID: number | null): number {
		if (playerID == null) return 999;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? 999;
	}

	const orderedGuests = $derived.by<number[]>(() => {
		const hostID = plan.preparer_id;
		const others = fest.guests.filter(id => id !== hostID);
		others.sort((a, b) => esteemRank(b) - esteemRank(a));
		return fest.guests.includes(hostID) ? [...others, hostID] : others;
	});

	const currentTurnID = $derived.by<number | null>(() => {
		for (const id of orderedGuests) {
			if (!(String(id) in fest.outcomes)) return id;
		}
		return null;
	});

	function guestStatus(pid: number): string {
		const k = String(pid);
		const oc = fest.outcomes[k];
		if (oc === 'make') return `make → ${fest.guestMakes[k] ?? '?'}`;
		if (oc === 'mar') return `mar → ${fest.guestMars[k] ?? '?'}`;
		if (oc === 'opt_out') return 'opted out';
		if (fest.guestRollIDs[k]) return 'rolling';
		return 'waiting';
	}

	let actionBusy = $state(false);
	let actionError = $state('');

	async function onJoin() {
		if (actionBusy) return;
		actionBusy = true; actionError = '';
		try { await joinFestivity(plan.id); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not join.'; }
		finally { actionBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">Guests</p>
	{#if orderedGuests.length === 0}
		<p class="choices-note muted">No guests yet.</p>
	{:else}
		<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
			{#each orderedGuests as gid (gid)}
				{@const isHost = gid === plan.preparer_id}
				{@const isTurn = gid === currentTurnID}
				<li class:muted={!isTurn && (String(gid) in fest.outcomes)}>
					<strong>{playerName(players, gid)}</strong>
					{#if isHost}<em> (host)</em>{/if}
					— {guestStatus(gid)}
					{#if isTurn && fest.phase === 'socializing'}
						<span class="muted"> · their turn</span>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	{#if fest.phase === 'socializing' && !iAmGuest && currentPlayerID != null && !amHost}
		{#if actionError}<p class="res-error">{actionError}</p>{/if}
		<button class="action-btn"
			onclick={onJoin}
			disabled={actionBusy}>
			{actionBusy ? '…' : 'Join as guest'}
		</button>
	{/if}
</div>
