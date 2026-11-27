<!-- Duel/DuelTermsFields.svelte
  The two terms a duel is set on: its type (arms or wits) and its place. Propose
  Duel's preparation names both (book p.60), and so does a Host Festivity
  challenge, which skips the delay but not the preparation (RULINGS, Host
  Festivity, 2026-11-26). One component, so both forms word and limit them alike.
  The parent checks the place is non-empty before it sends.
-->
<script lang="ts">
	import Chip from '$lib/components/shared/Chip.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import FormField from '../FormField.svelte';

	let { duelType = $bindable<'arms' | 'wits'>('arms'), place = $bindable('') }: {
		duelType?: 'arms' | 'wits';
		place?: string;
	} = $props();
</script>

<FormField label="Duel of">
	<div class="chip-row">
		<Chip selected={duelType === 'arms'} onclick={() => (duelType = 'arms')}>Arms</Chip>
		<Chip selected={duelType === 'wits'} onclick={() => (duelType = 'wits')}>Wits / Trial</Chip>
	</div>
</FormField>
<label class="form-label">
	Location
	<textarea rows={2} bind:value={place} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
		placeholder="Where will the duel take place?" required></textarea>
</label>
