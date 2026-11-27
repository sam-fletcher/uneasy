// festivityDraft.svelte.ts
// What the Host Festivity host (or a Make Demands stand-in driving the host's
// steps) has picked and typed in the Make picker and the pending-Mar card but not
// yet sent: the option, the rumor's words, the new peer's name, the asset or
// duel target chosen. Nothing is sent until the button is pressed, so until then
// the picks live only here.
//
// They are kept OUT of HostChoosing and HostPendingMar on purpose. Both are
// unmounted whenever a duel is pending: a guest's make can be a challenge, which
// takes the controls off the page until the target answers, and an accepted
// challenge becomes a nested Propose Duel that takes the whole festivity panel
// off the page until it ends. A component's own state does not survive that; this
// does, because it is keyed by the plan. (The same shape as lib/rollDraft.svelte.ts,
// and the "state moved into a child an {#if} can unmount" entry in the decisions
// file's recurring bug shapes.)
//
// One draft of each at a time is enough: a tab shows one table, and a game has
// at most one resolving Host Festivity. A different plan starts fresh, and a
// successful send clears it.

class HostMakeDraft {
	planId = $state(-1);
	picked = $state<string | null>(null);
	rumor = $state('');
	peerName = $state('');
	peerMarginalia = $state('');
	assetID = $state<number | null>(null);
	duelTargetID = $state<number | null>(null);
	/** The duel's terms, named by a challenger as Propose Duel's preparation does. */
	duelType = $state<'arms' | 'wits'>('arms');
	duelPlace = $state('');

	/** Start (or keep) the draft for this plan. The same plan keeps what was picked. */
	begin(planId: number): void {
		if (this.planId === planId) return;
		this.planId = planId;
		this.clear();
	}

	/** The make has been sent: nothing left to keep. */
	clear(): void {
		this.picked = null;
		this.rumor = '';
		this.peerName = '';
		this.peerMarginalia = '';
		this.assetID = null;
		this.duelTargetID = null;
		this.duelType = 'arms';
		this.duelPlace = '';
	}
}

class HostMarDraft {
	planId = $state(-1);
	/** The guest whose insistence the draft answers: the oldest still unanswered. */
	insisterID = $state<number | null>(null);
	picked = $state<string | null>(null);
	rumor = $state('');
	marginaliaID = $state<number | null>(null);
	assetID = $state<number | null>(null);

	/** Start (or keep) the draft for this plan and this insistence. Answering the
	 *  next guest's insistence is a fresh question, so it starts clean. */
	begin(planId: number, insisterID: number | null): void {
		if (this.planId === planId && this.insisterID === insisterID) return;
		this.planId = planId;
		this.insisterID = insisterID;
		this.clear();
	}

	/** The mar has been settled: nothing left to keep. */
	clear(): void {
		this.picked = null;
		this.rumor = '';
		this.marginaliaID = null;
		this.assetID = null;
	}
}

export const hostMakeDraft = new HostMakeDraft();
export const hostMarDraft = new HostMarDraft();
