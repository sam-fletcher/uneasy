<!-- plans/shared/PreRollStep.svelte
  The pre-roll narration step Spread Propaganda and Spread Rumors share: the
  preparer says what they are about to do (the rulebook's pre-roll), the server
  posts it for the table, and only then is the roll opened.

  Why it is a step at all (DESIGN_PRINCIPLES.md 1–2): the other players read this
  to decide whether to help or interfere, so it must be on the record first; and
  the preparation note was a SKETCH written rows ago, so this is where the player
  commits to what actually goes out.

  How the draft is offered back (principle 2's open tension, decided here — see the
  "worked case" there): a field whose prompt asks for the SAME thing the draft says
  starts pre-filled with it, marked as a draft; a field that asks for something the
  draft never held starts blank. Pre-filling spares a retype; the blank field and
  the "change whatever has changed" note keep the moment from being a bare "Next".

  Non-actors get a one-line wait; the caller shows the draft (or the secret's
  placeholder) itself, since only it knows whether the draft is public.
-->
<script module lang="ts">
	export interface PreRollField {
		key: string;
		label: string;
		placeholder: string;
		/** The prep-time sketch this field starts from, when the prompt asks for the
		 *  same thing. May arrive late (a kept secret is fetched). */
		draft?: string | null;
		rows?: number;
		max: number;
	}
</script>

<script lang="ts">
	import '../planPanel.css';

	interface Props {
		title: string;
		/** The rulebook's pre-roll wording. */
		prompt: string;
		fields: PreRollField[];
		/** Whether the viewer performs the step. */
		canAct: boolean;
		busy?: boolean;
		/** Who performs it, for the waiting line. */
		actorName: string;
		/** e.g. "is stating the rumor" — completes "{actorName} …". */
		waitingVerb: string;
		submitLabel?: string;
		onSubmit: (values: Record<string, string>) => void;
	}
	let {
		title, prompt, fields, canAct, busy = false, actorName, waitingVerb,
		submitLabel = 'Post & open the roll', onSubmit,
	}: Props = $props();

	let values = $state<Record<string, string>>({});
	let touched = $state<Record<string, boolean>>({});

	// Start from the draft until the player has typed in the field.
	$effect(() => {
		for (const f of fields) {
			if (f.draft && !touched[f.key] && values[f.key] !== f.draft) values[f.key] = f.draft;
		}
	});

	const complete = $derived(fields.every(f => (values[f.key] ?? '').trim().length > 0));
</script>

{#if canAct}
	<div class="pre-roll plan-form">
		<p class="choices-header">{title}</p>
		<p class="choices-note">{prompt}</p>
		{#each fields as f (f.key)}
			<label class="form-label">
				<span class="form-label-text">{f.label}</span>
				<textarea
					rows={f.rows ?? 3}
					class="form-textarea"
					maxlength={f.max}
					placeholder={f.placeholder}
					value={values[f.key] ?? ''}
					oninput={(e) => { touched[f.key] = true; values[f.key] = e.currentTarget.value; }}
				></textarea>
			</label>
			{#if f.draft && !touched[f.key]}
				<p class="choices-note muted">
					Starts from your preparation note, which was only a sketch — change whatever
					the last few rows have changed.
				</p>
			{/if}
		{/each}
		<p class="choices-note muted">
			The table reads this before anyone decides whether to help or interfere.
		</p>
		<button class="action-btn primary" onclick={() => onSubmit({ ...values })} disabled={busy || !complete}>
			{busy ? '…' : submitLabel}
		</button>
	</div>
{:else}
	<p class="ft-prompt muted">{actorName} {waitingVerb}…</p>
{/if}
