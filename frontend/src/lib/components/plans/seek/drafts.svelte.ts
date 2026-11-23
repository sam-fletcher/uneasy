// drafts.svelte.ts
// What the viewer has picked or typed on Seek Answers' resolve steps but not yet sent.
//
// It is made by the panel, not by any one step, because the steps come and go under it.
// The picker is replaced by the sub-steps once the choices commit; each sub-step leaves
// when nothing more is owed on it; and a player who is asked a question (a Make Demands
// winner can ask themselves) is shown the answer form in place of the whole sub-step area.
// A draft owned by a step is gone when its step leaves the screen, so a half-typed truth
// would not survive being asked a question. Held here, drafts survive as they did when all
// the steps were one component.
//
// `forfeitBusy` is one flag for the three "Skip — no valid targets" buttons, so that
// while one skip is in flight the others are disabled, as before.

export class SeekDrafts {
	/** How many times each option is picked (repeatable; the total is the dice result). */
	counts = $state<Record<string, number>>({
		break_resource: 0, declare_truth: 0, ask_question: 0, reveal_secret: 0,
	});
	total = $derived(Object.values(this.counts).reduce((a, b) => a + b, 0));

	// Make-list "break a resource": the marginalium chosen, and the asset it is on.
	brAssetID = $state<number | null>(null);
	brMargID = $state<number | null>(null);
	// Mar penalty selection (own resources).
	maAssetID = $state<number | null>(null);
	maMargID = $state<number | null>(null);
	// "Reveal an asset's secrets": the asset chosen.
	rsAssetID = $state<number | null>(null);

	// "Declare something true": the text.
	dtText = $state('');
	// "Ask a player a question": who is picked, and the question typed.
	aqPickedID = $state<number | null>(null);
	aqQuestion = $state('');
	// The questioned player's answer, typed.
	answerText = $state('');

	forfeitBusy = $state(false);
}
