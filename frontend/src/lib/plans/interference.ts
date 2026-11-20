// Who interfered with a roll, and how much. Spread Propaganda's mar option (d)
// goes to the top interferer, so its picker names them and shows the tally.
import type { DiceRollDie, Ranking } from '$lib/api';

export interface Interferer {
	playerID: number;
	dice: number;
	/** Their rank on the esteem track (1 is the highest), or null when they have none. */
	esteemRank: number | null;
}

/**
 * Everyone who interfered, most dice first. A tie goes to the better esteem rank
 * (rank 1 is the highest; a player with no rank sorts last), then to the lower
 * player id, so the first entry is the one the server picks.
 *
 * Every interference die counts, a cancelled one too: the book's test is who
 * leveraged the most assets, not whose dice survived.
 *
 * Backend mirror: ListInterferenceDiceByRoll + pickBestEsteemRanked
 * (handler/plan_spread_propaganda.go).
 */
export function interferers(dice: DiceRollDie[], rankings: Ranking[]): Interferer[] {
	const counts = new Map<number, number>();
	for (const d of dice) {
		if (d.is_interference) counts.set(d.player_id, (counts.get(d.player_id) ?? 0) + 1);
	}
	const rankOf = (id: number) =>
		rankings.find(r => r.category === 'esteem' && r.player_id === id)?.rank ?? null;
	return [...counts]
		.map(([playerID, n]) => ({ playerID, dice: n, esteemRank: rankOf(playerID) }))
		.sort((a, b) =>
			b.dice - a.dice
			|| (a.esteemRank ?? Infinity) - (b.esteemRank ?? Infinity)
			|| a.playerID - b.playerID);
}

const diceWord = (n: number) => `${n} ${n === 1 ? 'die' : 'dice'}`;

function joinNames(names: string[]): string {
	if (names.length <= 2) return names.join(' and ');
	return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
}

/**
 * The line under the counter-propaganda option: who the top interferer is, with
 * their dice, and the rest of the tally. Empty when nobody interfered. `list` is
 * interferers()'s output.
 *
 * A tie at the top is settled by esteem rank, and says so only when that is what
 * settled it. Between players with no esteem rank the server falls back to the
 * lower player id, which is not something to dress up as a ranking, so the line
 * says only that the player goes first on the tie-break.
 */
export function interferenceLine(list: Interferer[], nameOf: (playerID: number) => string): string {
	const [top, ...rest] = list;
	if (!top) return '';
	const tied = rest.filter(i => i.dice === top.dice);
	const behind = rest.filter(i => i.dice < top.dice);
	let line: string;
	if (tied.length > 0) {
		const byRank = top.esteemRank != null
			&& tied.every(i => i.esteemRank == null || top.esteemRank! < i.esteemRank);
		line = `Top interferer: ${nameOf(top.playerID)}, who ties ${joinNames(tied.map(i => nameOf(i.playerID)))}`
			+ ` at ${diceWord(top.dice)} ${byRank ? 'but ranks higher on esteem' : 'and goes first on the tie-break'}.`;
	} else {
		line = `Top interferer: ${nameOf(top.playerID)}, with ${diceWord(top.dice)}.`;
	}
	if (behind.length > 0) {
		line += ` Also interfered: ${behind.map(i => `${nameOf(i.playerID)}, with ${diceWord(i.dice)}`).join('; ')}.`;
	}
	return line;
}

/**
 * The line the picker shows under the counter-propaganda option, or '' where it
 * would say nothing true: before the dice have loaded, and on a plan that is itself
 * counter-propaganda, where the option is shut ("not available on
 * counter-propaganda") and naming who it would have gone to is a promise.
 */
export function counterPropLine(opts: {
	isCounter: boolean;
	dice: DiceRollDie[] | null;
	rankings: Ranking[];
	nameOf: (playerID: number) => string;
}): string {
	if (opts.isCounter || opts.dice == null) return '';
	return interferenceLine(interferers(opts.dice, opts.rankings), opts.nameOf);
}
