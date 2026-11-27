import { beforeEach, describe, expect, it } from 'vitest';
import { hostMakeDraft, hostMarDraft } from './festivityDraft.svelte';

// What the host has picked and typed in the Make picker and the pending-Mar card
// lives here, keyed by the plan, so it survives both cards being unmounted while
// a duel is pending (a guest's challenge, or the nested duel it becomes).

describe('hostMakeDraft', () => {
	beforeEach(() => hostMakeDraft.begin(-999)); // a plan no test uses, so each starts fresh

	it('keeps the picks when the same plan begins again, as a remount does', () => {
		hostMakeDraft.begin(7);
		hostMakeDraft.picked = 'spread_rumor';
		hostMakeDraft.rumor = 'half a rumo';
		hostMakeDraft.peerName = 'Lord Pell';

		hostMakeDraft.begin(7); // the card mounts again after the challenge is answered
		expect(hostMakeDraft.picked).toBe('spread_rumor');
		expect(hostMakeDraft.rumor).toBe('half a rumo');
		expect(hostMakeDraft.peerName).toBe('Lord Pell');
	});

	it('starts clean for a different plan', () => {
		hostMakeDraft.begin(7);
		hostMakeDraft.picked = 'challenge_duel';
		hostMakeDraft.duelTargetID = 3;

		hostMakeDraft.begin(8);
		expect(hostMakeDraft.picked).toBeNull();
		expect(hostMakeDraft.duelTargetID).toBeNull();
	});

	it('clear drops every pick once the make is sent', () => {
		hostMakeDraft.begin(7);
		hostMakeDraft.picked = 'introduce_peer';
		hostMakeDraft.peerName = 'Lord Pell';
		hostMakeDraft.peerMarginalia = 'Owes everyone';
		hostMakeDraft.assetID = 4;

		hostMakeDraft.clear();
		expect(hostMakeDraft.picked).toBeNull();
		expect(hostMakeDraft.peerName).toBe('');
		expect(hostMakeDraft.peerMarginalia).toBe('');
		expect(hostMakeDraft.assetID).toBeNull();
	});

	it('keeps a duel\'s terms until it is sent, and clear resets them', () => {
		hostMakeDraft.begin(7);
		hostMakeDraft.picked = 'challenge_duel';
		hostMakeDraft.duelTargetID = 3;
		hostMakeDraft.duelType = 'wits';
		hostMakeDraft.duelPlace = 'The sunken garden.';

		hostMakeDraft.begin(7); // a remount, before the make is sent
		expect(hostMakeDraft.duelType).toBe('wits');
		expect(hostMakeDraft.duelPlace).toBe('The sunken garden.');

		hostMakeDraft.clear();
		expect(hostMakeDraft.duelType).toBe('arms');
		expect(hostMakeDraft.duelPlace).toBe('');
	});
});

describe('hostMarDraft', () => {
	beforeEach(() => hostMarDraft.begin(-999, null));

	it('keeps the picks when the same insistence is answered again after a remount', () => {
		hostMarDraft.begin(7, 21);
		hostMarDraft.picked = 'rumor_about_you';
		hostMarDraft.rumor = 'It is said the host watered the wine.';

		hostMarDraft.begin(7, 21);
		expect(hostMarDraft.picked).toBe('rumor_about_you');
		expect(hostMarDraft.rumor).toBe('It is said the host watered the wine.');
	});

	it('starts clean for the next guest\'s insistence, and for a different plan', () => {
		hostMarDraft.begin(7, 21);
		hostMarDraft.picked = 'break_self';
		hostMarDraft.marginaliaID = 5;

		hostMarDraft.begin(7, 22);
		expect(hostMarDraft.picked).toBeNull();
		expect(hostMarDraft.marginaliaID).toBeNull();

		hostMarDraft.picked = 'accept_duels';
		hostMarDraft.begin(8, 22);
		expect(hostMarDraft.picked).toBeNull();
	});
});
