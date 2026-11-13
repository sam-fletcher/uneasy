<!-- LobbyView.svelte
  Lobby phase: the two questions a player arrives with — *what am I supposed
  to do* and *who else is here* — answered in that order.

  Rebuilt from adr/LOBBY_AND_CHECKLIST_PLAN.md D5 after a playtester joined a
  table and could not tell what was expected of them. The answer is *nothing*,
  so the page now says so out loud (the verdict), shows the table, and
  offers three optional things to do while waiting. Order matters: the old
  page led with the Join Code — a heading-weight invite tool aimed at someone
  who by definition had already used the code — and buried the primer below
  the fold.

  The facilitator gets the same top slot, filled with the one thing that IS
  theirs to do: start the game. There is never both a verdict and a start
  block.

  Nothing here is framed. Each section is set apart by its heading and the gap
  around it, the seats are plain lines and the checklist rows are the house
  tappable row. Only what can be pressed has an outline: the main button, and the
  invite chair, a button that copies the join code.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { startPrologue } from '$lib/api';
	import type { Game, Player, PresenceMember } from '$lib/api';
	import { getPushState, enablePush, onPushPermissionChange, type PushState } from '$lib/push';
	import PushBlockedHelp from '$lib/components/shared/PushBlockedHelp.svelte';
	import ChecklistRow from '$lib/components/shared/ChecklistRow.svelte';
	import TableRoster from '$lib/components/shared/TableRoster.svelte';
	import HelpContent from '../HelpContent.svelte';
	import HelpGlyph from '../HelpGlyph.svelte';
	import FlagGlyph from '../FlagGlyph.svelte';
	import BellGlyph from '../BellGlyph.svelte';
	import { onMount } from 'svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let {
		gameID,
		game,
		players,
		members,
		currentPlayerID,
		waitingPlayerIDs,
		isFacilitator,
		vapidPublicKey,
		tonesSetCount,
		onOpenTones,
		onFeedback,
	}: {
		gameID: string;
		game: Game;
		players: Player[];
		/** Live presence, for the roster's online rings. */
		members: PresenceMember[];
		currentPlayerID: number | null;
		/** The same set that rings the header chips. */
		waitingPlayerIDs: Set<number>;
		isFacilitator: boolean;
		vapidPublicKey: string;
		/** Tone topics the table has actually taken a position on. */
		tonesSetCount: number;
		onOpenTones: () => void;
		onFeedback: () => void;
	} = $props();

	let error = $state('');

	// ── The table ─────────────────────────────────────────────────────────────
	const MAX_PLAYERS = 5;
	const MIN_PLAYERS = 2;
	const seatsLeft = $derived(Math.max(0, MAX_PLAYERS - players.length));
	const capacityLine = $derived(
		seatsLeft === 0
			? `${players.length} seated · table full`
			: `${players.length} seated · room for ${seatsLeft} more`
	);
	const facilitatorName = $derived(
		players.find((p) => p.is_facilitator)?.display_name ?? 'the facilitator'
	);

	// ── Join-code copy feedback ───────────────────────────────────────────────
	let joinCodeCopied = $state(false);
	let joinCodeCopyTimer: ReturnType<typeof setTimeout> | null = null;
	async function copyJoinCode() {
		try {
			await navigator.clipboard.writeText(`https://uneasy.up.railway.app/ Table code: ${game.join_code}`);
			joinCodeCopied = true;
			if (joinCodeCopyTimer) clearTimeout(joinCodeCopyTimer);
			joinCodeCopyTimer = setTimeout(() => (joinCodeCopied = false), 1500);
		} catch {
			// Clipboard can reject (permissions / insecure context); leave the
			// label unchanged so the user can still read & copy manually.
		}
	}

	// ── Phase advancement ─────────────────────────────────────────────────────
	let advancing = $state(false);
	const canStart = $derived(players.length >= MIN_PLAYERS);
	async function advancePhase() {
		if (advancing || !canStart) return;
		advancing = true;
		error = '';
		try {
			await startPrologue(gameID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally {
			advancing = false;
		}
	}

	// ── Notifications row ─────────────────────────────────────────────────────
	// Was a dismissible soft-ask card with its dismissal written to
	// localStorage *permanently* — so the one task that makes asynchronous play
	// work could be invisible for good after a single "Not now" (D5, finding
	// 5). A 44px collapsed row carrying a status chip cannot nag, so there is
	// nothing left to dismiss and no key to remember.
	let pushState = $state<PushState>('unsupported');
	let pushBusy = $state(false);
	let pushError = $state('');
	// Whether *this* visit asked the browser. All it gates now is the "nothing
	// changed" annotation, which describes the last click and nothing else.
	let pushAttempted = $state(false);

	const pushChip = $derived.by<{ text: string; tone: 'on' | 'off' } | undefined>(() => {
		switch (pushState) {
			case 'on': return { text: 'on', tone: 'on' };
			case 'denied': return { text: 'blocked', tone: 'off' };
			case 'ios-needs-install': return { text: 'add to home', tone: 'off' };
			case 'off': return { text: 'off', tone: 'off' };
			default: return undefined; // 'unsupported' — the row doesn't render
		}
	});

	async function enablePushFromLobby() {
		pushBusy = true;
		pushError = '';
		try {
			pushState = await enablePush(vapidPublicKey);
		} catch (e) {
			pushError = e instanceof Error ? e.message : 'Could not turn on notifications.';
			pushState = await getPushState().catch(() => pushState);
		} finally {
			pushAttempted = true;
			pushBusy = false;
		}
	}

	function refreshPushState() {
		// Not mid-request: the permission prompt blurs the page, so the focus
		// handler below fires *during* enablePush. That read would see
		// granted-but-not-yet-subscribed ('off') and could land after
		// enablePush's own 'on', flipping the chip back to the ask.
		if (pushBusy) return;
		void getPushState()
			.then((s) => {
				// The "nothing changed" annotation describes the last click. A
				// state change since then can only have come from browser
				// settings, which makes it stale — drop it so a player who has
				// just un-blocked us gets the clean ask back.
				if (s !== pushState) pushAttempted = false;
				pushState = s;
			})
			.catch(() => {});
	}

	onMount(() => {
		refreshPushState();
		// Following the recovery steps happens in browser settings, not here, so
		// re-read on the way back — otherwise the row keeps insisting they're
		// blocked after they've allowed us. The permission event covers the
		// same-tab case; the focus/visibility pair covers Safari, which rejects
		// permissions.query for notifications. This reads the browser, not the
		// server, so it costs nothing per fire.
		const unsubscribePermission = onPushPermissionChange(refreshPushState);
		const onVisibility = () => {
			if (document.visibilityState === 'visible') refreshPushState();
		};
		document.addEventListener('visibilitychange', onVisibility);
		window.addEventListener('focus', refreshPushState);
		return () => {
			unsubscribePermission();
			document.removeEventListener('visibilitychange', onVisibility);
			window.removeEventListener('focus', refreshPushState);
		};
	});
</script>

<!-- The bell is the house mark the profile's table rows use, drawn once in
     BellGlyph. Colour comes from the row. -->
{#snippet bellGlyph()}
	<BellGlyph size={16} />
{/snippet}
<!-- The flag is a component, not a path drawn here, because the prologue's
     closing stage offers the same item and the two must not drift. -->
{#snippet flagGlyph()}
	<FlagGlyph />
{/snippet}

<div class="phase-view lobby">
	{#if error}
		<ErrorText message={error} />
	{/if}

	<!-- ── Verdict, or the facilitator's start block (D4) ─────────────────── -->
	<section class="verdict">
		{#if isFacilitator}
			<h2>Is everyone here?</h2>
			<p class="verdict-text">
				The table is waiting on you. 
				Once you start the Prologue, no one else can join, so make sure everyone is here.
			</p>
			<!-- Disabled-with-a-reason rather than hidden, mirroring the closing
			     stage's Ready button: a control that vanishes teaches nothing. -->
			<button class="action-btn primary" onclick={advancePhase} disabled={!canStart || advancing}>
				{advancing ? '…' : 'Start Prologue'}
			</button>
			{#if !canStart}
				<p class="muted-text">One more player and you can begin.</p>
			{/if}
		{:else}
			<h2>You're seated. Nothing to do yet.</h2>
			<p class="verdict-text">
				{facilitatorName} will start the game once everyone has arrived. 
				No one can join after that, so chase up anyone missing.
			</p>
		{/if}
	</section>

	<!-- ── The table ───────────────────────────────────────────────────────── -->
	<section class="lobby-table">
		<h3 class="section-heading">
			The Table <span class="capacity">{capacityLine}</span>
		</h3>
		<TableRoster {players} {members} {currentPlayerID} {waitingPlayerIDs}>
			{#snippet inviteSeat()}
				<!-- The join code is an empty chair, not a heading: it is addressed
				     to whoever ISN'T here yet. Gone once the table is full.
				     The whole chair is the copy control, so the label and the code
				     are one object rather than a stray word beside a button. It is
				     a control, so it is the one thing in the list with an outline,
				     and the dashed edge is the house "not filled in yet" line. -->
				{#if seatsLeft > 0}
					<button
						type="button"
						class="invite-chair"
						class:copied={joinCodeCopied}
						onclick={copyJoinCode}
						aria-label={`Invite a friend — copy the join code, ${game.join_code.split('').join(' ')}`}
					>
						<span class="invite-text">Invite a friend</span>
						<span class="code-badge">
							{game.join_code}
							<span class="copy-hint" aria-live="polite">{joinCodeCopied ? 'Copied!' : 'copy'}</span>
						</span>
					</button>
				{/if}
			{/snippet}
		</TableRoster>
	</section>

	<!-- ── While you wait ──────────────────────────────────────────────────── -->
	<section class="while-you-wait">
		<h3 class="section-heading">While you wait</h3>

		<!-- Shut on arrival (owner, 2026-08-16), reversing the plan's
		     defaultOpen: the primer's body is a whole tabbed sub-panel, and
		     open it pushed the two rows below it a screen and a half down —
		     burying the notifications row, which is the one item here that
		     makes asynchronous play work. Its gold title is what marks it as
		     the invitation now. No persistence either way. -->
		<ChecklistRow
			title="Read the two-minute primer"
			subtitle="New here? A skim is enough for now."
			glyph="help"
			tone="primary"
			action="expand"
			id="lobby-primer-body"
		>
			<p class="primer-cue">
				You can reopen this any time from the <span class="help-cue" role="img" aria-label="help button"
					><HelpGlyph size="1.15em" /></span
				> in the top-right corner.
			</p>
			<HelpContent {onFeedback} />
		</ChecklistRow>

		{#if pushChip}
			<ChecklistRow
				title="Turn on notifications"
				subtitle="Turns might be days apart sometimes."
				glyph={bellGlyph}
				action="expand"
				id="lobby-push-body"
				state={pushChip}
			>
				{#if pushState === 'on'}
					<p class="muted-text">
						We'll tell you when the table is waiting on you. Change this any time
						in your Profile.
					</p>
				{:else if pushState === 'ios-needs-install'}
					<p class="muted-text">
						iPhone and iPad only deliver notifications to installed apps: tap the
						Share icon, then "Add to Home Screen". Open Uneasy from there and turn
						them on.
					</p>
				{:else if pushState === 'denied'}
					<div role="status">
						<p class="muted-text">
							Your browser has notifications blocked for this site. That's a browser
							setting, so we can't ask again from here — but you can undo it:
						</p>
						<PushBlockedHelp />
					</div>
				{:else}
					<p class="muted-text">
						Turn them on for this device and we'll tell you when it's your turn, so
						you don't have to keep checking back. You can change this any time in
						your Profile.
					</p>
					{#if pushError}
						<ErrorText message={pushError} />
					{:else if pushAttempted}
						<p class="muted-text" role="status">
							Nothing changed — the browser prompt was closed without an answer. You can try again.
						</p>
					{/if}
					<div class="row-actions">
						<button class="action-btn secondary" onclick={enablePushFromLobby} disabled={pushBusy}>
							{pushBusy ? '…' : 'Enable notifications'}
						</button>
					</div>
				{/if}
			</ChecklistRow>
		{/if}

		<!-- Tones live in a sheet, so this is an arrow, not a caret (D1). R2:
		     an invitation, not a deadline — the lock is the closing stage's
		     line to deliver, not the lobby's. -->
		<ChecklistRow
			title="Adjust the tone of the game"
			subtitle="Themes to include or avoid."
			glyph={flagGlyph}
			action="navigate"
			onSelect={onOpenTones}
			state={{ text: tonesSetCount > 0 ? `${tonesSetCount} set` : 'none yet', tone: 'neutral' }}
		/>
	</section>
</div>

<style>
	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: var(--space-4) var(--space-3);
		gap: var(--space-5);
		overflow-y: auto;
		min-height: 0;
	}

	/* ── Verdict ──────────────────────────────────────────────────────────── */
	/* The step's heading, its words and, for the facilitator, its one main
	   button, standing on the page where a framed card used to hold them. The
	   facilitator's copy is the one thing here someone has to act on, and what
	   says so is that it comes first and carries the only filled button on the
	   screen. */
	.verdict {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		align-items: flex-start;
	}
	.verdict h2 {
		margin: 0;
		color: var(--color-accent);
		font-size: var(--text-heading);
		line-height: 1.3;
	}
	.verdict-text {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.45;
	}
	.verdict .action-btn { margin-top: var(--space-1); }

	/* ── Sections ─────────────────────────────────────────────────────────── */
	.lobby-table,
	.while-you-wait {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.section-heading {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin: 0;
		color: var(--color-accent);
		font-size: var(--text-heading);
	}
	.capacity {
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}

	/* ── Invite chair ─────────────────────────────────────────────────────── */
	/* A dashed seat: unmistakably empty, and a button, so it is a control (the
	   neutral control border and the control corner) and stays 44px. The dash is
	   the house "not filled in yet" mark (StandingStrip's dummy slots,
	   DifficultyMeter's next segment). Set a gap below the last seat's rule so
	   the box doesn't hug it. */
	.invite-chair {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: var(--size-tap);
		margin-top: var(--space-2);
		padding: var(--space-1) var(--space-3);
		background: none;
		border: 1px dashed var(--color-border-control);
		border-radius: var(--radius-control);
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.invite-chair:hover { border-color: var(--color-border-control-hover); }
	.invite-chair:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.invite-text {
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}
	/* Pushed to the trailing edge, where an occupied seat's `trailing` content
	   would sit. The code is plain text: a fill behind it would be a badge on
	   something that can't be pressed on its own, and the whole chair is the
	   thing that is pressed. */
	.code-badge {
		margin-left: auto;
		display: inline-flex;
		gap: var(--space-2);
		align-items: center;
		font-family: var(--font-mono);
		font-size: var(--text-body);
		letter-spacing: 0.1em;
		color: var(--color-text);
	}
	.copy-hint {
		font-family: var(--font-serif);
		font-size: var(--text-label);
		letter-spacing: normal;
		color: var(--color-text-secondary);
	}
	/* The result of pressing it, so it takes the success colour, beside its word. */
	.invite-chair.copied .copy-hint { color: var(--color-success); }

	/* ── Row bodies ───────────────────────────────────────────────────────── */
	.row-actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
	.primer-cue {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}
	/* Mid-sentence stand-in for the header's help control: same glyph, same
	   gold, sized a touch over 1em so the circle reads at caption size and
	   nudged onto the text baseline (a geometric mark centres, a letterform
	   sits). Inline-flex keeps it from stretching the line box. */
	.help-cue {
		display: inline-flex;
		align-items: center;
		color: var(--color-accent);
		vertical-align: -0.2em;
	}
</style>
