import { describe, it, expect } from 'vitest';
import type { Asset, ClosingReady, ExtraPeer, PrologueClaim, PrologueSheet } from '$lib/api';
import {
	MAIN_CHARACTER_PLACEHOLDER,
	findMainCharacter,
	isMcNamed,
	needsExtraPeer,
	findExtraPeer,
	unclaimedTitles,
	readyBlockedReason,
	isReady,
	notReadyPlayerIDs,
	myAtRiskCount,
	blankAssets,
	retinueTallies,
} from './closing';

/** One intact marginalia at the given position, for readability in the
 *  blank/at-risk cases below. */
function note(assetID: number, position = 1) {
	return {
		id: position,
		asset_id: assetID,
		position,
		text: 'x',
		is_torn: false,
		torn_at: null,
		torn_by_id: null,
		title: null,
	};
}

function asset(overrides: Partial<Asset> = {}): Asset {
	return {
		id: 1,
		game_id: 1,
		owner_id: 1,
		creator_id: 1,
		asset_type: 'peer',
		name: 'Some Name',
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: false,
		created_at: '2026-01-01T00:00:00Z',
		destroyed_at: null,
		linked_card_suit: null,
		linked_card_value: null,
		marginalia: [],
		secret_count: 0,
		...overrides,
	} as Asset;
}

describe('findMainCharacter', () => {
	it('returns null with no viewer', () => {
		expect(findMainCharacter([asset({ is_main_character: true })], null)).toBeNull();
	});

	it('finds the owned main-character asset', () => {
		const mc = asset({ id: 5, owner_id: 2, is_main_character: true });
		const other = asset({ id: 6, owner_id: 2, is_main_character: false });
		expect(findMainCharacter([other, mc], 2)).toBe(mc);
	});

	it('ignores another player\'s main character', () => {
		const mc = asset({ id: 5, owner_id: 3, is_main_character: true });
		expect(findMainCharacter([mc], 2)).toBeNull();
	});
});

describe('isMcNamed', () => {
	it('is false with no main character', () => {
		expect(isMcNamed(null)).toBe(false);
	});

	it('is false for the placeholder name', () => {
		expect(isMcNamed(asset({ name: MAIN_CHARACTER_PLACEHOLDER }))).toBe(false);
	});

	it('is false for a blank/whitespace name', () => {
		expect(isMcNamed(asset({ name: '   ' }))).toBe(false);
	});

	it('is true for a real name', () => {
		expect(isMcNamed(asset({ name: 'Lady Wren' }))).toBe(true);
	});
});

describe('needsExtraPeer', () => {
	it('is true at and below 3 players', () => {
		expect(needsExtraPeer(2)).toBe(true);
		expect(needsExtraPeer(3)).toBe(true);
	});

	it('is false above 3 players', () => {
		expect(needsExtraPeer(4)).toBe(false);
		expect(needsExtraPeer(5)).toBe(false);
	});
});

describe('findExtraPeer', () => {
	const peers: ExtraPeer[] = [{ player_id: 1, title_name: 'Spymaster', asset_id: 10 }];

	it('returns null with no viewer', () => {
		expect(findExtraPeer(peers, null)).toBeNull();
	});

	it('finds the viewer\'s own extra peer', () => {
		expect(findExtraPeer(peers, 1)).toEqual(peers[0]);
	});

	it('returns null when the viewer has none', () => {
		expect(findExtraPeer(peers, 2)).toBeNull();
	});
});

describe('unclaimedTitles', () => {
	const titlesSheet: PrologueSheet = {
		type: 'titles',
		display_name: 'Titles',
		choice_asset_type: 'Artifact',
		choices: [
			{ name: 'Monarch', description: '', cards: [{ suit: 'H', value: 'A' }, { suit: 'S', value: 'K' }] },
			{ name: 'Spymaster', description: '', cards: [{ suit: 'H', value: 'Q' }, { suit: 'D', value: 'J' }] },
			{ name: 'Heretic', description: '', cards: [{ suit: 'H', value: '9' }, { suit: 'C', value: '8' }] },
		],
	};

	it('returns nothing when there is no titles sheet', () => {
		expect(unclaimedTitles(undefined, [], [])).toEqual([]);
	});

	it('excludes titles claimed through ordinary turn-taking', () => {
		const claims: PrologueClaim[] = [
			{ sheet_type: 'titles', choice_name: 'Monarch', player_id: 1, turn_number: 1 },
		];
		const names = unclaimedTitles(titlesSheet, claims, []).map((c) => c.name);
		expect(names).toEqual(['Spymaster', 'Heretic']);
	});

	it('excludes titles already taken by another extra peer', () => {
		const peers: ExtraPeer[] = [{ player_id: 2, title_name: 'Spymaster', asset_id: 11 }];
		const names = unclaimedTitles(titlesSheet, [], peers).map((c) => c.name);
		expect(names).toEqual(['Monarch', 'Heretic']);
	});

	it('ignores claims from other sheets', () => {
		const claims: PrologueClaim[] = [
			{ sheet_type: 'hailing_from', choice_name: 'Monarch', player_id: 1, turn_number: 1 },
		];
		const names = unclaimedTitles(titlesSheet, claims, []).map((c) => c.name);
		expect(names).toEqual(['Monarch', 'Spymaster', 'Heretic']);
	});
});

describe('readyBlockedReason', () => {
	it('blocks on an unnamed main character first', () => {
		expect(readyBlockedReason(false, 4, false)).toBe('Name your main character first.');
	});

	it('blocks on a missing extra peer in small games once named', () => {
		expect(readyBlockedReason(true, 3, false)).toBe('Create your extra peer first.');
	});

	it('does not require an extra peer in 4+ player games', () => {
		expect(readyBlockedReason(true, 4, false)).toBeNull();
	});

	it('is null once every hard condition is met', () => {
		expect(readyBlockedReason(true, 2, true)).toBeNull();
	});

	it('blocks on blank assets once the earlier conditions pass', () => {
		expect(readyBlockedReason(true, 4, false, 2)).toBe(
			'Give every asset at least one marginalia first.'
		);
	});

	it('reports the main-character reason ahead of blank assets, matching the server order', () => {
		expect(readyBlockedReason(false, 4, false, 3)).toBe('Name your main character first.');
	});

	it('reports the extra-peer reason ahead of blank assets, matching the server order', () => {
		expect(readyBlockedReason(true, 3, false, 3)).toBe('Create your extra peer first.');
	});

	it('defaults the blank count to zero so callers can omit it', () => {
		expect(readyBlockedReason(true, 4, false)).toBeNull();
	});
});

describe('blankAssets', () => {
	it('is empty with no viewer', () => {
		expect(blankAssets([asset({ owner_id: 1 })], null)).toEqual([]);
	});

	it("returns only the viewer's own note-less assets", () => {
		const mineBlank = asset({ id: 1, owner_id: 1, marginalia: [] });
		const mineNoted = asset({ id: 2, owner_id: 1, marginalia: [note(2)] });
		const theirsBlank = asset({ id: 3, owner_id: 2, marginalia: [] });
		expect(blankAssets([mineBlank, mineNoted, theirsBlank], 1).map((a) => a.id)).toEqual([1]);
	});

	it('excludes destroyed assets', () => {
		const destroyed = asset({ owner_id: 1, is_destroyed: true, marginalia: [] });
		expect(blankAssets([destroyed], 1)).toEqual([]);
	});

	it('counts an asset whose only note is torn as noted, not blank', () => {
		// A torn note is still a row, so the asset is breakable-then-destroyable
		// by the normal path — it is at risk, not invulnerable. (In practice the
		// last tear destroys it outright; this guards the helper's semantics.)
		const torn = asset({ id: 7, owner_id: 1, marginalia: [{ ...note(7), is_torn: true }] });
		expect(blankAssets([torn], 1)).toEqual([]);
	});

	it('treats a marginalia-less WS payload as blank rather than throwing', () => {
		const partial = asset({ id: 9, owner_id: 1 });
		delete (partial as Partial<Asset>).marginalia;
		expect(blankAssets([partial], 1).map((a) => a.id)).toEqual([9]);
	});
});

describe('isReady / notReadyPlayerIDs', () => {
	const closingReady: ClosingReady[] = [
		{ player_id: 1, ready: true },
		{ player_id: 2, ready: false },
	];

	it('isReady reflects the ready flag', () => {
		expect(isReady(closingReady, 1)).toBe(true);
		expect(isReady(closingReady, 2)).toBe(false);
	});

	it('isReady is false for a player with no row yet', () => {
		expect(isReady(closingReady, 3)).toBe(false);
	});

	it('isReady is false with no viewer', () => {
		expect(isReady(closingReady, null)).toBe(false);
	});

	it('notReadyPlayerIDs lists everyone not (yet) ready', () => {
		const players = [{ id: 1 }, { id: 2 }, { id: 3 }];
		expect(notReadyPlayerIDs(players, closingReady)).toEqual([2, 3]);
	});

	it('notReadyPlayerIDs is empty once everyone is ready', () => {
		const players = [{ id: 1 }];
		expect(notReadyPlayerIDs(players, closingReady)).toEqual([]);
	});
});

describe('myAtRiskCount', () => {
	it('is zero with no viewer', () => {
		expect(myAtRiskCount([asset({ owner_id: 1 })], null)).toBe(0);
	});

	it('counts only the viewer\'s own needlessly-at-risk assets', () => {
		const mine = asset({ id: 1, owner_id: 1, marginalia: [] });
		const alsoMine = asset({
			id: 2,
			owner_id: 1,
			marginalia: [{ id: 1, asset_id: 2, position: 1, text: 'x', is_torn: false, torn_at: null, torn_by_id: null, title: null }],
		});
		const theirs = asset({ id: 3, owner_id: 2, marginalia: [] });
		const mineButSafe = asset({
			id: 4,
			owner_id: 1,
			marginalia: [1, 2, 3, 4].map((position) => ({
				id: position, asset_id: 4, position, text: 'x', is_torn: false, torn_at: null, torn_by_id: null, title: null,
			})),
		});
		expect(myAtRiskCount([mine, alsoMine, theirs, mineButSafe], 1)).toBe(2);
	});

	it('excludes destroyed assets', () => {
		const destroyed = asset({ owner_id: 1, is_destroyed: true, marginalia: [] });
		expect(myAtRiskCount([destroyed], 1)).toBe(0);
	});
});

describe('retinueTallies', () => {
	const players = [{ id: 1 }, { id: 2 }];

	it('tallies each owner\'s live assets by type', () => {
		const assets = [
			asset({ id: 1, owner_id: 1, creator_id: 1, asset_type: 'peer', is_main_character: true }),
			asset({ id: 2, owner_id: 1, creator_id: 1, asset_type: 'artifact' }),
			asset({ id: 3, owner_id: 1, creator_id: 1, asset_type: 'holding' }),
			asset({ id: 4, owner_id: 2, creator_id: 2, asset_type: 'resource' }),
		];
		const [one, two] = retinueTallies(players, assets);
		expect(one.counts).toEqual({ peer: 1, artifact: 1, resource: 0, holding: 1 });
		expect(one.total).toBe(3);
		expect(two.counts).toEqual({ peer: 0, artifact: 0, resource: 1, holding: 0 });
		expect(two.total).toBe(1);
	});

	it('counts assets taken from others via owner_id !== creator_id', () => {
		const assets = [
			// Player 1 made their own peer, and took an artifact player 2 created.
			asset({ id: 1, owner_id: 1, creator_id: 1, asset_type: 'peer' }),
			asset({ id: 2, owner_id: 1, creator_id: 2, asset_type: 'artifact' }),
			// Player 2 kept only a self-made holding.
			asset({ id: 3, owner_id: 2, creator_id: 2, asset_type: 'holding' }),
		];
		const [one, two] = retinueTallies(players, assets);
		expect(one.takenFromOthers).toBe(1);
		expect(one.total).toBe(2);
		expect(two.takenFromOthers).toBe(0);
	});

	it('excludes destroyed assets from every tally', () => {
		const assets = [
			asset({ id: 1, owner_id: 1, creator_id: 1, asset_type: 'peer' }),
			asset({ id: 2, owner_id: 1, creator_id: 2, asset_type: 'artifact', is_destroyed: true }),
		];
		const [one] = retinueTallies(players, assets);
		expect(one.total).toBe(1);
		expect(one.counts.artifact).toBe(0);
		expect(one.takenFromOthers).toBe(0);
	});

	it('returns a zeroed tally for a player with no assets', () => {
		const [one, two] = retinueTallies(players, [
			asset({ id: 1, owner_id: 1, creator_id: 1, asset_type: 'peer' }),
		]);
		expect(one.total).toBe(1);
		expect(two).toEqual({
			playerID: 2,
			counts: { peer: 0, artifact: 0, resource: 0, holding: 0 },
			total: 0,
			takenFromOthers: 0,
		});
	});
});
