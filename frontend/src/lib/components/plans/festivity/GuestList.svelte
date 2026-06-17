<!-- Festivity/GuestList.svelte
  Renders the esteem-ordered guest list with each guest's current status.
  Every player attends as a guest by default (added in the handler's
  OnResolve), so there is no opt-in. Owns its esteem-rank derivation.
-->
<script lang="ts">
	import { type Plan, type Player, type Ranking } from '$lib/api';
	import { playerName } from '../shared';
	import type { FestRes } from './options';

	let { plan, fest, players, rankings }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		rankings: Ranking[];
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

	function guestStatus(pid: number): string {
		const k = String(pid);
		const oc = fest.outcomes[k];
		if (oc === 'make') return `make → ${fest.guestMakes[k] ?? '?'}`;
		if (oc === 'mar') return `mar → ${fest.guestMars[k] ?? '?'}`;
		if (oc === 'opt_out') return 'opted out';
		if (oc === 'host') return 'free make (host)';
		if (fest.guestRollIDs[k]) return 'rolling';
		return 'waiting';
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
				<li class:muted={String(gid) in fest.outcomes}>
					<strong>{playerName(players, gid)}</strong>
					{#if isHost}<em> (host)</em>{/if}
					— {guestStatus(gid)}
				</li>
			{/each}
		</ul>
	{/if}
</div>
