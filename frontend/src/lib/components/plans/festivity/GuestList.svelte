<!-- Festivity/GuestList.svelte — "The Talk of the Event"
  A per-guest scorecard / record of the social occasion: who made a good
  impression (Make), who slipped up (Mar), who got the better of the host
  (invoked their IOU), and how the host is working the room (extra Makes).

  This is a compact summary, not a re-narration — the blow-by-blow already
  lives in the chat action log. One verdict line per player; the host is never
  shown as "waiting" (they don't roll). Order is stable (roster order, host
  last) so the record doesn't reshuffle as the evening unfolds.
-->
<script lang="ts">
	import { type Plan, type Player } from '$lib/api';
	import { playerName } from '../shared';
	import { MAKE_PHRASE, MAR_PHRASE, earnedHostMakes, type FestRes } from './options';

	let { plan, fest, players, currentPlayerID }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		currentPlayerID: number | null;
	} = $props();

	// Stable order: everyone in roster order, the host last (they frame the event).
	const orderedGuests = $derived.by<number[]>(() => {
		const hostID = plan.preparer_id;
		const others = fest.guests.filter(id => id !== hostID);
		return fest.guests.includes(hostID) ? [...others, hostID] : others;
	});

	type Tone = 'make' | 'mar' | 'opt' | 'host' | 'pending';
	type Entry = { tone: Tone; headline: string; detail?: string };

	function entryFor(pid: number): Entry {
		const k = String(pid);

		// The host never rolls — show how they're working the room instead.
		if (pid === plan.preparer_id) {
			const earned = earnedHostMakes(fest, plan.preparer_id);
			const taken = fest.hostMakesTaken.length;
			const marsTaken = fest.hostMarInsists.length;
			const bits = [`${taken} of ${earned} ${earned === 1 ? 'Make' : 'Makes'} taken`];
			if (marsTaken > 0) bits.push(`${marsTaken} ${marsTaken === 1 ? 'Mar' : 'Mars'} from guests`);
			return { tone: 'host', headline: 'Working the room', detail: bits.join(' · ') };
		}

		const oc = fest.outcomes[k];
		if (oc === 'make') {
			const phrase = MAKE_PHRASE[fest.guestMakes[k]] ?? 'made their play';
			// A make-roller holds an IOU until they spend it on a host Mar.
			const note = fest.guestIOUs.includes(pid)
				? 'still holds something over the host'
				: 'got the better of the host';
			return { tone: 'make', headline: 'Made a good impression', detail: `${phrase} · ${note}` };
		}
		if (oc === 'mar') {
			return { tone: 'mar', headline: 'Slipped up', detail: MAR_PHRASE[fest.guestMars[k]] ?? 'had a rough moment' };
		}
		if (oc === 'opt_out') return { tone: 'opt', headline: 'Kept to themselves' };
		if (fest.guestRollIDs[k]) return { tone: 'pending', headline: 'Making their play…' };
		return { tone: 'pending', headline: 'Yet to make their mark' };
	}
</script>

<div class="choices-section">
	<p class="choices-header">The Talk of the Event</p>
	{#if orderedGuests.length === 0}
		<p class="choices-note muted">No guests yet.</p>
	{:else}
		<ul class="fest-scorecard">
			{#each orderedGuests as gid (gid)}
				{@const e = entryFor(gid)}
				<li>
					<div class="who">
						{playerName(players, gid)}{#if gid === currentPlayerID}<em>&nbsp;(you)</em>{/if}{#if gid === plan.preparer_id}<em>&nbsp;(host)</em>{/if}
					</div>
					<div class="say" class:pending={e.tone === 'pending'}>
						<span class="verdict"
							class:outcome-make={e.tone === 'make'}
							class:outcome-mar={e.tone === 'mar'}
						>{e.headline}</span>{#if e.detail}<span class="detail">&nbsp;—&nbsp;{e.detail}</span>{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.fest-scorecard {
		list-style: none;
		margin: 0.25rem 0 0;
		padding: 0;
	}
	.fest-scorecard li {
		padding: 0.4rem 0;
		border-bottom: 1px solid var(--color-border-warm);
	}
	.fest-scorecard li:last-child {
		border-bottom: none;
	}
	.who {
		overflow-wrap: anywhere;
	}
	.who em {
		color: var(--color-text-faint);
		font-style: normal;
		font-size: 0.85em;
	}
	.say {
		font-size: 0.9rem;
		margin-top: 0.1rem;
	}
	.verdict:not(.outcome-make):not(.outcome-mar) {
		color: var(--color-text-muted);
	}
	.say.pending .verdict {
		font-style: italic;
	}
	.detail {
		color: var(--color-text-faint);
	}
</style>
