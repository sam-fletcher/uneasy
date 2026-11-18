// prepDraft.ts
// What a preparer's form tells the rest of the table while they type it.
//
// While the focus player fills in a plan's preparation form, everyone else sees a
// read-only mirror of it. The mirror is fed by a draft the form emits on every
// change (`ctx.emitPrepDraft`): PlanPanel sends it up the socket as
// `prepare_plan.draft`, and the server fans it out to the whole table. Nothing is
// stored; it is a live view.
//
// That makes a draft public the instant it is sent, so a draft must NEVER carry text
// that could become a secret. Showing a watcher the words as they are typed is the
// least important thing their read-only screen does: a nicety, and it must not cost
// anything else. Spread Rumors is the plan where it would: the rumor can be kept
// secret, and the box that says so can be ticked after the text is typed, by which
// time every watcher would already have read it. So its draft carries the target, the
// secret flag and the holder asset (that a secret exists is public) and never the
// rumor's text. The text reaches the table when the plan is prepared, as every plan's
// notes do. The other plans' drafts still carry their notes, because those are posted
// to the table in full the moment the plan is prepared.
//
// A form that gains a field able to hold a secret builds its draft here, where a test
// can pin what is left out. The server strips the same text again
// (hub.redactPreparePlanDraft), so a tab still running an older client after a deploy
// cannot leak what this file keeps back.

export interface SpreadRumorsFormState {
	filterOwnerID: number | null;
	targetAssetID: number | null;
	keepSecret: boolean;
	secretAssetID: number | null;
}

/**
 * The Spread Rumors preparation draft: which asset the rumor is about, whether it is kept
 * secret and which asset holds it. Never its text, ticked or not (no `notes` key at all).
 */
export function spreadRumorsPrepDraft(f: SpreadRumorsFormState): Record<string, unknown> {
	return {
		filter_owner_id: f.filterOwnerID,
		target_asset_id: f.targetAssetID,
		keep_secret: f.keepSecret,
		secret_asset_id: f.secretAssetID,
	};
}
