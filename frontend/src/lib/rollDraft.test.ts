import { beforeEach, describe, expect, it } from 'vitest';
import { rollDraft } from './rollDraft.svelte';

// What a player has picked on the open roll but not yet sent lives here, keyed
// by the roll, so it survives the roll panel moving between the plan's card and
// the page (a plan panel that was still loading when the roll opened), or a panel
// remounting its card mid-roll.

describe('rollDraft', () => {
	beforeEach(() => rollDraft.begin(-999)); // a roll no test uses, so each starts fresh

	it('keeps the picks when the same roll begins again, as a remount does', () => {
		rollDraft.begin(7);
		rollDraft.intent = 'interfere';
		rollDraft.assetIds = new Set([11, 12]);
		rollDraft.bankedIds = new Set([3]);

		rollDraft.begin(7); // the panel mounts again somewhere else
		expect(rollDraft.intent).toBe('interfere');
		expect([...rollDraft.assetIds]).toEqual([11, 12]);
		expect([...rollDraft.bankedIds]).toEqual([3]);
	});

	it('starts clean for a different roll', () => {
		rollDraft.begin(7);
		rollDraft.intent = 'aid';
		rollDraft.assetIds = new Set([11]);

		rollDraft.begin(8);
		expect(rollDraft.intent).toBeNull();
		expect(rollDraft.assetIds.size).toBe(0);
		expect(rollDraft.bankedIds.size).toBe(0);
	});

	it('clearPicks drops the dice picked once they are sent, and keeps the side', () => {
		rollDraft.begin(7);
		rollDraft.intent = 'aid';
		rollDraft.assetIds = new Set([11]);
		rollDraft.bankedIds = new Set([3]);

		rollDraft.clearPicks();
		expect(rollDraft.assetIds.size).toBe(0);
		expect(rollDraft.bankedIds.size).toBe(0);
		expect(rollDraft.intent).toBe('aid');
	});
});
