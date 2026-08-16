<!-- LobbyView.svelte
  Lobby phase: the two questions a player arrives with — *what am I supposed
  to do* and *who else is here* — answered in that order.

  Rebuilt from adr/LOBBY_AND_CHECKLIST_PLAN.md D5 after a playtester joined a
  table and could not tell what was expected of them. The answer is *nothing*,
  so the page now says so out loud (the verdict panel), shows the table, and
  offers three optional things to do while waiting. Order matters: the old
  page led with the Join Code — a heading-weight invite tool aimed at someone
  who by definition had already used the code — and buried the primer below
  the fold.

  The facilitator gets the same top slot, filled with the one thing that IS
  theirs to do: start the game. There is never both a verdict and a start
  block.
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

<!-- House icon idiom (AssetTypeIcon/CrownGlyph/HelpGlyph): 24×24 viewBox,
     stroke=currentColor, width 2, round caps. Colour comes from the row. -->
{#snippet bellGlyph()}
	<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
		<path d="M13.7 21a2 2 0 0 1-3.4 0" />
	</svg>
{/snippet}
{#snippet flagGlyph()}
	<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z" />
		<line x1="4" y1="22" x2="4" y2="15" />
	</svg>
{/snippet}

<div class="phase-view lobby">
	{#if error}
		<ErrorText message={error} />
	{/if}

	<!-- ── Verdict, or the facilitator's start block (D4) ─────────────────── -->
	<section class="verdict" class:yours={isFacilitator}>
		{#if isFacilitator}
			<h2>Is everyone here?</h2>
			<p class="muted-text">
				The table is waiting on you. 
				Once you start the Prologue, no one else can join, so make sure everyone is here.
			</p>
			<!-- Disabled-with-a-reason rather than hidden, mirroring the closing
			     stage's Ready button: a control that vanishes teaches nothing. -->
			<button class="action-btn primary" onclick={advancePhase} disabled={!canStart || advancing}>
				{advancing ? '…' : 'Start Prologue'}
			</button>
			{#if !canStart}
				<p class="muted-text small">One more player and you can begin.</p>
			{/if}
		{:else}
			<h2>You're seated. Nothing to do yet.</h2>
			<p class="muted-text">
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
				     are one object rather than a stray word beside a button — and
				     the row lines up with the seats above it: dashed dot where
				     their colour dot sits, code pill where `trailing` would. -->
				{#if seatsLeft > 0}
					<button
						type="button"
						class="invite-chair"
						class:copied={joinCodeCopied}
						onclick={copyJoinCode}
						aria-label={`Invite a friend — copy the join code, ${game.join_code.split('').join(' ')}`}
					>
						<span class="invite-dot" aria-hidden="true"></span>
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
		     makes asynchronous play work. The gold frame is what marks it as
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
					<p class="muted-text small">
						We'll tell you when the table is waiting on you. Change this any time
						in your Profile.
					</p>
				{:else if pushState === 'ios-needs-install'}
					<p class="muted-text small">
						iPhone and iPad only deliver notifications to installed apps: tap the
						Share icon, then "Add to Home Screen". Open Uneasy from there and turn
						them on.
					</p>
				{:else if pushState === 'denied'}
					<div role="status">
						<p class="muted-text small">
							Your browser has notifications blocked for this site. That's a browser
							setting, so we can't ask again from here — but you can undo it:
						</p>
						<PushBlockedHelp />
					</div>
				{:else}
					<p class="muted-text small">
						Turn them on for this device and we'll tell you when it's your turn, so
						you don't have to keep checking back. You can change this any time in
						your Profile.
					</p>
					{#if pushError}
						<ErrorText message={pushError} />
					{:else if pushAttempted}
						<p class="muted-text small" role="status">
							Nothing changed — the browser prompt was closed without an answer. You can try again.
						</p>
					{/if}
					<div class="row-actions">
						<button class="action-btn primary" onclick={enablePushFromLobby} disabled={pushBusy}>
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
		padding: 1rem 0.75rem;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	/* ── Verdict ──────────────────────────────────────────────────────────── */
	.verdict {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		align-items: flex-start;
		padding: 0.85rem 0.8rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 12px;
	}
	/* The facilitator's copy is the one thing on this screen someone has to
	   act on, so it gets the warm frame. Gold as the frame and the label,
	   never as a fill (chat-bar ruling, 2026-07-25). */
	.verdict.yours { border-color: var(--color-border-warm-antique); }
	.verdict h2 {
		margin: 0;
		color: var(--color-accent);
		font-size: 1.05rem;
		line-height: 1.3;
	}
	.verdict .muted-text { line-height: 1.45; }
	.verdict .action-btn { margin-top: 0.15rem; }

	/* ── Sections ─────────────────────────────────────────────────────────── */
	.lobby-table,
	.while-you-wait {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.section-heading {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin: 0;
		color: var(--color-accent);
		font-size: 1rem;
	}
	.capacity {
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	/* ── Invite chair ─────────────────────────────────────────────────────── */
	/* A dashed seat: same height, padding and rhythm as the occupied rows above
	   it, but unmistakably empty. The dash is the house "not filled in yet"
	   mark (StandingStrip's dummy slots, DifficultyMeter's next segment). */
	.invite-chair {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		min-height: 44px;
		padding: 0.35rem 0.6rem;
		background: none;
		border: 1px dashed var(--color-border-strong);
		border-radius: 999px;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.invite-chair:hover { border-color: var(--color-accent-dim); }
	.invite-chair:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	/* Sits in the seat dot's slot, same 8px, hollow — the seat has no one in
	   it yet, so it has no colour yet either. */
	.invite-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		border: 1px dashed var(--color-border-strong);
		flex-shrink: 0;
	}
	.invite-text {
		color: var(--color-text-muted);
		font-size: 0.9rem;
	}
	/* Pushed to the trailing edge, where an occupied seat's `trailing` content
	   would sit. The gap between label and code is then the width of the row,
	   which reads as layout rather than as an odd space. */
	.code-badge {
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 0.9rem;
		background: var(--color-border);
		color: var(--color-text);
		padding: 0.25rem 0.6rem;
		border-radius: 4px;
		letter-spacing: 0.1em;
		display: inline-flex;
		gap: 0.4rem;
		align-items: center;
	}
	.copy-hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}
	.invite-chair.copied .copy-hint { color: var(--color-accent); }

	/* ── Row bodies ───────────────────────────────────────────────────────── */
	.row-actions { display: flex; gap: 0.6rem; flex-wrap: wrap; }
	.primer-cue {
		margin: 0;
		color: var(--color-text-muted);
		font-size: 0.8rem;
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
