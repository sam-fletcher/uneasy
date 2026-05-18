<!-- MakeWar/DeclarationScene.svelte
  Focus-player-only resolve UI: post the one-time declaration scene, then
  complete the plan. Non-focus players see a "waiting on…" note.
-->
<script lang="ts">
	import { postWarScene, completePlan, type Player } from '$lib/api';
	import { playerName } from '../shared';

	let { planID, preparerID, players, isFocusPlayer, warScenePosted, onPlansChanged, setError }: {
		planID: number;
		preparerID: number;
		players: Player[];
		isFocusPlayer: boolean;
		warScenePosted: boolean;
		onPlansChanged: () => void;
		setError: (msg: string) => void;
	} = $props();

	let sceneBusy = $state(false);
	let completeBusy = $state(false);

	async function onPostScene() {
		if (sceneBusy) return;
		sceneBusy = true; setError('');
		try {
			await postWarScene(planID);
			onPlansChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not mark scene posted.');
		} finally { sceneBusy = false; }
	}

	async function onComplete() {
		if (completeBusy) return;
		completeBusy = true; setError('');
		try {
			await completePlan(planID);
			onPlansChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not complete plan.');
		} finally { completeBusy = false; }
	}
</script>

{#if isFocusPlayer}
	<div class="choices-section">
		<p class="choices-header">Declaration scene</p>
		<p class="choices-note">
			This is the one-time scene where the war breaks open. Post the
			declaration in the scene thread above, then mark it posted here.
			(Battles between rows happen via cost-of-battle, narrated freely
			— no extra scene per row.)
		</p>
		{#if !warScenePosted}
			<button class="action-btn primary" onclick={onPostScene} disabled={sceneBusy}>
				{sceneBusy ? '…' : 'Mark declaration scene posted'}
			</button>
		{:else}
			<p class="choices-applied">Declaration scene marked posted.</p>
			<button class="action-btn primary" onclick={onComplete} disabled={completeBusy}>
				{completeBusy ? '…' : 'Complete plan'}
			</button>
			<p class="choices-note muted">
				(Completing the plan does NOT end the war — that requires peace
				or full surrender. The cost-of-battle picker above remains active
				each row until the war ends.)
			</p>
		{/if}
	</div>
{:else}
	<p class="choices-note muted">
		{playerName(players, preparerID)} is posting the war's
		declaration scene…
	</p>
{/if}
