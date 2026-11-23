<!-- Rumors/StateRumor.svelte
  Pre-roll: the preparer states the rumor and how it is spreading; the server posts it
  for the table and only then opens the roll. Stating a secret rumor makes it public.

  OnResolve parks the plan with no roll. The preparer's narration is posted for the
  table — who read it to decide whether to help or interfere — and only then does the
  roll open. Mounted for as long as the step is open (the panel shows it while
  nothing says it is done).
-->
<script lang="ts">
	import { castSpreadRumorsRoll, getSecrets, type Plan } from '$lib/api';
	import { parseSpreadRumorsData } from '$lib/plans/resolutionData/spread_rumors';
	import PreRollStep from '../shared/PreRollStep.svelte';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	// Deliberately its own derived values and not the panel's scene: the effect below
	// must re-run when the plan or who is looking changes, and not when some other part
	// of the table does.
	const srData = $derived(parseSpreadRumorsData(plan));
	const isPreparer = $derived(currentPlayerID === plan.preparer_id);

	// The draft to start the "rumor" field from. A kept-secret rumor's draft lives in
	// a Secret, not the (blanked) public note; the preparer, its author, can read it
	// through the ordinary grant-checked secrets endpoint. Nobody else ever sees it.
	let secretDraft = $state<string | null>(null);
	$effect(() => {
		const holder = srData.secret_asset_id;
		const secretID = srData.secret_id;
		if (!isPreparer || !srData.is_secret || holder == null || secretID == null) return;
		let stale = false;
		getSecrets(holder)
			.then(r => { if (!stale) secretDraft = r.secrets.find(x => x.id === secretID)?.text ?? null; })
			.catch(() => { /* no draft to offer: the field simply starts blank */ });
		return () => { stale = true; };
	});
	const rumorDraft = $derived(plan.preparation_notes ?? secretDraft);

	let preRollBusy = $state(false);
	async function submitPreRoll(p: Plan, values: Record<string, string>) {
		if (preRollBusy) return;
		preRollBusy = true; error = '';
		try {
			const res = await castSpreadRumorsRoll(p.id, (values.rumor ?? '').trim(), (values.how ?? '').trim());
			if (res.roll) ctx.onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not open the roll.';
		} finally { preRollBusy = false; }
	}
</script>

<PreRollStep
	title="Spread the rumor"
	prompt="State the rumor you plan to spread and explain how you're spreading it."
	fields={[
		{
			key: 'rumor',
			label: 'The rumor',
			placeholder: 'What are people about to be told?',
			draft: rumorDraft,
			rows: 3,
			max: TEXT_LIMITS.NARRATIVE,
		},
		{
			key: 'how',
			label: 'How you are spreading it',
			placeholder: 'Letting it slip in taverns? Whispering it in ears around court?',
			rows: 3,
			max: TEXT_LIMITS.NARRATIVE,
		},
	]}
	canAct={isPreparer}
	busy={preRollBusy}
	onSubmit={(v) => submitPreRoll(plan, v)}
/>
