<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/modalShell.css';
	import '$lib/components/shared/statusText.css';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import {
		getMe, listMyTables, updateMe, logout,
		createTable, joinTable,
		type Account, type MyTable,
	} from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import PhaseBadge from '$lib/components/shared/PhaseBadge.svelte';
	import LogMark from '$lib/components/LogMark.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import CharCounter from '$lib/components/CharCounter.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import FeedbackForm from '$lib/components/FeedbackForm.svelte';
	import { getPushState, enablePush, disablePush, onPushPermissionChange, type PushState } from '$lib/push';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import PushBlockedHelp from '$lib/components/shared/PushBlockedHelp.svelte';

	let me = $state<Account | null>(null);
	let tables = $state<MyTable[]>([]);
	let loading = $state(true);
	let error = $state('');

	let editingUsername = $state(false);
	let usernameDraft = $state('');
	let editingEmail = $state(false);
	let emailDraft = $state('');
	let editingPassword = $state(false);
	let passwordDraft = $state('');

	let joinCode = $state('');
	let busy = $state(false);
	let notice = $state('');
	let feedbackOpen = $state(false);

	// ── Table cards ──────────────────────────────────────────────────────────
	// Live games first; within each group keep the server's most-recently-
	// joined-first order (Array.prototype.sort is stable).
	const sortedTables = $derived(
		[...tables].sort((a, b) => Number(a.phase === 'ended') - Number(b.phase === 'ended'))
	);

	function isYourMove(t: MyTable): boolean {
		return t.phase !== 'ended' && t.waiting_on_player_ids.includes(t.player_id);
	}

	// ── Notifications ────────────────────────────────────────────────────────
	// The cadence <select> works in strings ('off' | '1' | '3' | '8' | '24' |
	// '72'); notify_cadence_hours itself is number | null.
	let cadenceDraft = $state('24');
	let cadenceSaving = $state(false);
	let pushState = $state<PushState>('off');
	let pushBusy = $state(false);

	// Reject if a fetch hangs (e.g. a wedged dev server) so the page can show a
	// retry button instead of a permanent "Loading…".
	function withTimeout<T>(p: Promise<T>, ms = 10000): Promise<T> {
		return Promise.race([
			p,
			new Promise<T>((_, reject) =>
				setTimeout(() => reject(new Error('Timed out loading your profile.')), ms)
			),
		]);
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const acct = await withTimeout(getMe());
			if (!acct) { goto('/'); return; }
			me = acct;
			usernameDraft = acct.username;
			emailDraft = acct.email ?? '';
			cadenceDraft = acct.notify_cadence_hours == null ? 'off' : String(acct.notify_cadence_hours);
			const res = await withTimeout(listMyTables());
			tables = res.tables;
			getPushState().then((s) => { pushState = s; });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load profile.';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	// ── Refresh on return ────────────────────────────────────────────────────
	// The table cards (roster, phase, waiting-on, online) come from one fetch
	// in onMount and have no WebSocket behind them — an account-level socket
	// doesn't exist, and by design: hub presence means "has a table open", so
	// a profile page deliberately counts as offline. Without this, a player
	// who joins your table while you sit here never appears until a reload.
	//
	// Refetch when the player comes back to the page instead of polling. That
	// distinction is a cost constraint, not just taste: Neon free bills awake
	// time against a 100 CU-hr cap and suspends after ~5 idle minutes, so a
	// poll would hold the DB awake for as long as one tab stays open — the
	// same failure the notifications ticker hit in 2026-07 (see
	// adr/PUBLIC_LAUNCH_PLAN.md "Post-deploy finding"). A visibility/focus
	// refetch can only fire when a human is already using the app.
	//
	// visibilitychange covers tab switches; focus covers app switches that
	// leave the tab "visible". They overlap, which the debounce absorbs.
	// ListMyTables is the priciest read we have (N+1: GetPlayersByGame plus a
	// full ComputeWaitState per table), so the floor matters more here than
	// the staleness does.
	const REFRESH_MIN_INTERVAL_MS = 10_000;
	let lastRefreshAt = 0;
	let refreshing = false;

	async function refreshTables() {
		if (refreshing || loading || !me) return;
		const now = Date.now();
		if (now - lastRefreshAt < REFRESH_MIN_INTERVAL_MS) return;
		lastRefreshAt = now;
		refreshing = true;
		try {
			const res = await withTimeout(listMyTables());
			tables = res.tables;
		} catch {
			// Leave the cards showing what they already had — a failed
			// background refresh shouldn't replace a working page with an
			// error. The next return to the page tries again.
		} finally {
			refreshing = false;
		}
	}

	// Push state is read from the browser, not the server, so it's free of the
	// debounce and the DB-cost reasoning above — re-read it on every return.
	// Fixing a block happens in browser settings, i.e. necessarily away from
	// this page; without a re-read the row keeps insisting they're blocked
	// after they've allowed us. permissions.query's change event covers the
	// same-tab case (Chrome's settings bubble doesn't blur the page), and
	// Safari, which rejects that query, is covered by the visibility path.
	function refreshPushState() {
		// Not mid-toggle: the permission prompt blurs the page, so this fires
		// *during* enablePush. That read would see granted-but-not-yet-
		// subscribed ('off') and could land after togglePush's own 'on'.
		if (pushBusy) return;
		void getPushState().then((s) => { pushState = s; }).catch(() => {});
	}

	onMount(() => {
		// `load()` is the initial fetch; count it so returning to a
		// just-opened page doesn't immediately refetch.
		lastRefreshAt = Date.now();
		const onVisibility = () => {
			if (document.visibilityState === 'visible') {
				void refreshTables();
				refreshPushState();
			}
		};
		const onFocus = () => {
			void refreshTables();
			refreshPushState();
		};
		document.addEventListener('visibilitychange', onVisibility);
		window.addEventListener('focus', onFocus);
		const unsubscribePermission = onPushPermissionChange(refreshPushState);
		return () => {
			document.removeEventListener('visibilitychange', onVisibility);
			window.removeEventListener('focus', onFocus);
			unsubscribePermission();
		};
	});

	async function saveUsername() {
		error = ''; notice = '';
		try {
			me = { ...me, ...await updateMe({ username: usernameDraft.trim() }) };
			editingUsername = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update player name.';
		}
	}
	async function saveEmail() {
		error = ''; notice = '';
		try {
			me = { ...me, ...await updateMe({ email: emailDraft.trim() || null }) };
			editingEmail = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update email.';
		}
	}
	async function savePassword() {
		if (!passwordDraft) return;
		error = ''; notice = '';
		try {
			await updateMe({ password: passwordDraft });
			passwordDraft = '';
			editingPassword = false;
			notice = 'Password updated.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update password.';
		}
	}
	async function saveCadence() {
		error = ''; notice = '';
		cadenceSaving = true;
		try {
			const hours = cadenceDraft === 'off' ? null : Number(cadenceDraft);
			me = { ...me, ...await updateMe({ notify_cadence_hours: hours }) };
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update reminder cadence.';
		} finally {
			cadenceSaving = false;
		}
	}

	async function togglePush() {
		if (!me) return;
		error = '';
		pushBusy = true;
		try {
			pushState = pushState === 'on'
				? await disablePush()
				: await enablePush(me.vapid_public_key);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update push notifications.';
			// disablePush unsubscribes the browser *before* telling the server,
			// so a failed DELETE leaves the toggle claiming "on" when this
			// device is already off. Re-read the real state so the control
			// isn't lying while the error explains what didn't happen. (The
			// orphaned server row self-heals: the sender prunes on 404/410 —
			// handler/push_notifications.go:247.)
			pushState = await getPushState().catch(() => pushState);
		} finally {
			pushBusy = false;
		}
	}

	async function doLogout() {
		// logout() reports failure now (it used to ignore res.ok entirely, so a
		// failed logout looked exactly like a successful one and navigated away
		// with the session cookie still live). Stay put and say so instead.
		error = '';
		try {
			await logout();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not log out.';
			return;
		}
		goto('/');
	}
	async function doCreate() {
		busy = true; error = '';
		try {
			const { game } = await createTable();
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create table.';
			busy = false;
		}
	}
	async function doJoin() {
		if (!joinCode.trim()) return;
		busy = true; error = '';
		try {
			const { game } = await joinTable(joinCode.trim().toUpperCase());
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not join table.';
			busy = false;
		}
	}
</script>

{#if loading}
	<p class="muted-text">Loading…</p>
{:else if !me}
	<div class="load-error">
		<ErrorText message={error || 'Could not load your profile.'} />
		<button class="action-btn primary" onclick={load}>Retry</button>
	</div>
{:else}
	<div class="profile">
		<p class="wordmark">Uneasy Lies <span class="the">the</span> Head</p>

		{#if error}<ErrorText message={error} />{/if}
		{#if notice}<p class="status">{notice}</p>{/if}

		{#if tables.length > 0}
			<section class="card tables-card">
				<h2>Your tables</h2>
				<ul class="table-list">
					{#each sortedTables as t (t.game_id)}
						{@const ended = t.phase === 'ended'}
						<li>
							<a class="table-card" class:your-move={isYourMove(t)} class:ended href={`/table/${t.game_id}`}>
								<span class="table-id">
									<span class="table-code">Table <span class="code-value">{t.join_code}</span></span>
									<span class="status-row">
										<PhaseBadge phase={t.phase} />
										{#if !ended && t.unread_count > 0}
											<span
												class="unread-chip"
												aria-label={`${t.unread_count} unread chat ${t.unread_count === 1 ? 'message' : 'messages'}`}
											>
												<span class="unread-chip-mark" aria-hidden="true"><LogMark family="chat" /></span>
												<b>{t.unread_count > 99 ? '99+' : t.unread_count}</b>
											</span>
										{/if}
									</span>
								</span>
								<span class="pills">
									{#each t.players as p (p.id)}
										{@const online = !ended && (p.online || p.id === t.player_id)}
										{@const waited = !ended && t.waiting_on_player_ids.includes(p.id)}
										<span
											class="pill"
											class:waited
											style:--pill-color={playerColor(p)}
											aria-label={`${p.display_name}${p.id === t.player_id ? ' (you)' : ''}${online ? ', online' : ''}${waited ? ', game is waiting on them' : ''}`}
										>
											<span class="pill-dot" aria-hidden="true"></span>
											<span class="pill-name">{p.display_name}</span>
										</span>
									{/each}
								</span>
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		<section class="card">
			<h2>Join a table</h2>
			<p class="hint">Have a code from your host? Enter it to take a seat.</p>
			<div class="join">
				<input aria-label="Join code" placeholder="Join code" bind:value={joinCode} maxlength={6} style="text-transform:uppercase;letter-spacing:0.15em" />
				<button class="action-btn primary" onclick={doJoin} disabled={busy || !joinCode.trim()}>Join</button>
			</div>
			<div class="create-row">
				<span class="hint">Hosting a game?</span>
				<button class="action-btn primary" onclick={doCreate} disabled={busy}>Create a new table</button>
			</div>
		</section>

				<section class="card">
			<h2>Notifications</h2>
			<p class="hint">
				If a table is waiting on you longer than your chosen time, we'll send a reminder.
			</p>
			<div class="row">
				<span class="label">Remind me</span>
				<select aria-label="Reminder cadence" bind:value={cadenceDraft} onchange={saveCadence} disabled={cadenceSaving}>
					<option value="1">Every hour</option>
					<option value="3">Every 3 hours</option>
					<option value="8">Every 8 hours</option>
					<option value="24">Once a day</option>
					<option value="72">Every 3 days</option>
					<option value="off">Off</option>
				</select>
			</div>
			<div class="row push-row">
				<span class="label">This device</span>
				{#if me.vapid_public_key === '' && pushState !== 'unsupported' && pushState !== 'ios-needs-install'}
					<span class="muted-text small">Push isn't configured on this server yet.</span>
				{:else if pushState === 'unsupported'}
					<span class="muted-text small">Push notifications aren't supported in this browser.</span>
				{:else if pushState === 'ios-needs-install'}
					<span class="muted-text small">Add Uneasy to your Home Screen (Share → Add to Home Screen) to enable push on iPhone/iPad.</span>
				{:else if pushState === 'denied'}
					<div class="push-blocked-row">
						<span class="muted-text small">
							Blocked by your browser. We can't ask again from here — this switch only
							exists in your browser's own settings:
						</span>
						<PushBlockedHelp />
					</div>
				{:else}
					<span>Push notifications: {pushState === 'on' ? 'On' : 'Off'}</span>
					<button class="action-btn primary" onclick={togglePush} disabled={pushBusy}>
						{pushBusy ? '…' : pushState === 'on' ? 'Turn off' : 'Turn on'}
					</button>
				{/if}
			</div>
			<p class="hint push-hint">
				The cadence applies to your whole account; push must be turned
				on separately on each device/browser you want reminders on.
			</p>
		</section>

		<section class="card">
			<h2>Account</h2>
			<div class="row">
				<span class="label">Player name</span>
				{#if editingUsername}
					<input aria-label="Player name" bind:value={usernameDraft} maxlength={TEXT_LIMITS.USERNAME} />
					<CharCounter value={usernameDraft} max={TEXT_LIMITS.USERNAME} />
					<button class="action-btn primary small" onclick={saveUsername}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingUsername = false; usernameDraft = me?.username ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.username}</span>
					<button class="action-btn primary small" aria-label="Edit player name" onclick={() => { editingUsername = true; }}>Edit</button>
				{/if}
			</div>
			<!-- TODO: Hook up backend email handling -->
			<!-- <div class="row">
				<span class="label">Email</span>
				{#if editingEmail}
					<input type="email" aria-label="Email" bind:value={emailDraft} />
					<button class="action-btn primary small" onclick={saveEmail}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingEmail = false; emailDraft = me?.email ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.email ?? 'Not set. For notifications and password recovery.'}</span>
					<button class="action-btn secondary small" aria-label="Edit email" onclick={() => { editingEmail = true; }}>Edit</button>
				{/if}
			</div> -->
			<div class="row">
				<span class="label">Password</span>
				{#if editingPassword}
					<input type="password" aria-label="New password" bind:value={passwordDraft} placeholder="Enter a new password" />
					<button class="action-btn primary small" onclick={savePassword} disabled={!passwordDraft}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingPassword = false; passwordDraft = ''; }}>Cancel</button>
				{:else}
					<span class="masked">••••••••</span>
					<button class="action-btn primary small" aria-label="Edit password" onclick={() => { editingPassword = true; }}>Edit</button>
				{/if}
			</div>
		</section>

		<div class="footer-actions">
			<button class="action-btn primary feedback-btn" onclick={() => feedbackOpen = true}>Send feedback</button>
			<button class="action-btn secondary" onclick={doLogout}>Log out</button>
		</div>
	</div>

	<RetinueSheet open={feedbackOpen} onClose={() => feedbackOpen = false}>
		<div class="feedback-sheet">
			<h3 class="sheet-title">Send feedback</h3>
			<FeedbackForm />
		</div>
	</RetinueSheet>
{/if}

<style>
	/* A content column like any other: capped at 440 and centered
	   (docs/STYLE_GUIDE.md "Layout widths"). From the chat dock up — the
	   system's "two columns fit" boundary — the page widens to two column
	   widths so the tables grid can go 2-up, while the form sections stay
	   phone-width (forms gain nothing from width).
	   The horizontal gutter is ours, not main's (main.flush in +layout.svelte):
	   inside the cap, so the column measures a true 440 / 888 at those
	   viewports. 0.75rem matches the phase views' inner gutter, which keeps
	   the content box identical to a table column's at every width. */
	.profile { display:flex; flex-direction:column; gap:1.25rem; max-width:440px; margin: 0 auto; padding:1rem 0.75rem 0; }
	@media (min-width: 790px) {
		/* 440 + 8 gutter + 440, sizing the PAGE, not the tiles. An outer-box
		   cap like .phase-column's 440, so our gutter and .card's padding both
		   eat into it — the tiles land at 406 here. They're fluid (the grid's
		   floor is 330), not capped columns in their own right, so that's
		   within range; the number's job is to stop the page growing past two
		   phones. */
		.profile { max-width: 888px; }
		.profile > :not(.tables-card) { width: 100%; max-width: 440px; margin-inline: auto; }
	}
	.wordmark {
		text-align: center;
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		font-size: clamp(1.15rem, 4.5vw, 1.6rem);
		color: var(--color-accent);
		margin-top: -0.8rem;
		margin-bottom: -0.4rem;
	}
	.wordmark .the { font-size: 0.6em; letter-spacing: 0.1em; }
	h2 { color:var(--color-accent); font-size:1.2rem; margin-bottom:0.75rem; }
	.hint { color:var(--color-text-muted); font-size:0.85rem; margin-bottom:0.6rem; }
	.card { background:var(--color-surface); border:1px solid var(--color-border); border-radius:12px; padding:1.25rem; }
	.row { display:flex; align-items:center; flex-wrap:wrap; gap:0.5rem; padding:0.5rem 0; border-bottom:1px solid var(--color-border); }
	.row:last-child { border-bottom:none; }
	.label { width:5rem; color:var(--color-text-muted); font-size:0.85rem; }
	.masked { letter-spacing:0.15em; color:var(--color-text-muted); }
	.row input, .row select { flex:1; min-width:0; min-height:44px; }
	.row select {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		padding: 0 0.6rem;
		font: inherit;
	}
	.row span:not(.label) { flex:1; min-width:0; }
	.push-row { border-bottom: none; }
	/* The blocked case is a paragraph plus a numbered list, not a value that
	   can sit inline next to its label like the other rows' controls. */
	.push-blocked-row { flex: 1 1 100%; display: flex; flex-direction: column; gap: 0.4rem; }
	.push-hint { margin-top: -0.3rem; }
	/* Renders in place of .profile, so it carries the same gutter — main.flush
	   no longer supplies one. */
	.load-error { display:flex; flex-direction:column; align-items:center; gap:1rem; max-width:440px; margin:0 auto; padding:2rem 0.75rem 0; }
	.status { color:var(--color-accent); font-size:0.9rem; }
	/* The field label sits on its own line so the value and its buttons get
	   the full width instead of cramping — the form sections stay phone-width
	   (≤440) at every viewport, so this is unconditional. */
	.label { width:100%; }
	ul { list-style:none; }

	/* ── Table cards ────────────────────────────────────────────────────────
	   One card per game, echoing the in-table header: table id + shared
	   PhaseBadge stacked on the left, the roster as a pill grid on the
	   right. State channels: waited pill = gold BORDER (the app's one
	   waiting-on treatment, shared with the header chips and TableRoster),
	   your-move = warm card fill, ended = whole card muted with the other
	   treatments suppressed. Presence is no longer drawn here — see
	   `.pill` below. */
	/* Tiles: ~330px is the card content's intrinsic minimum (sized at the
	   phone floor), so the auto-fill grid is 1-up while the section is a
	   single 440 column and 2-up (~420 each, inside the phone band) once
	   the page widens at the dock. min(330px, 100%) keeps the minimum from
	   forcing overflow on phones. */
	.table-list {
		display:grid;
		grid-template-columns:repeat(auto-fill, minmax(min(330px, 100%), 1fr));
		gap:0.6rem;
	}
	.table-card {
		display:flex;
		align-items:center;
		flex-wrap:wrap;
		gap:0.6rem 1.25rem;
		padding:0.7rem 0.75rem;
		border:1px solid var(--color-border);
		border-radius:var(--radius-md);
		color:var(--color-text);
		text-decoration:none;
	}
	.table-card:hover { border-color:var(--color-accent-dim); }
	.table-card:focus-visible { outline:2px solid var(--color-accent); outline-offset:1px; }
	/* The game is blocked on you: warm emphasis fill, same semantics as the
	   in-table selected/active gold surfaces. */
	.table-card.your-move {
		background:var(--color-surface-active);
		border-color:var(--color-accent-dim);
	}
	.table-card.ended { opacity:0.55; }
	.table-card.ended:hover { opacity:0.8; }
	/* Unread ("12 new") sits in the status cluster beside the phase badge,
	   since it is card-level status like the phase is.
	   It uses the QUIET gold chip trio, not the solid-accent badge the in-game
	   chat bar wears: on this card gold already carries turn semantics twice
	   (.your-move's surface fill, .waited's pill), so a third solid-gold
	   object would collapse two independent signals — "you owe a move" and
	   "the table has been talking" — into one indistinguishable glow. The
	   trio is the documented register for a quiet badge, and it differs from
	   .your-move by channel as well as brightness: that is a surface fill,
	   this is an object.
	   The chat mark carries the noun instead of the word "new", which never
	   said new *what* while sitting beside a phase badge. It also repays the
	   mark the mobile chat bar teaches, so recognising it in one place works
	   in the other. Bold on the number, per the style guide's standalone-
	   numeric-counter ruling. The mark is aria-hidden and the chip carries an
	   aria-label, since dropping the word would otherwise leave a screen
	   reader announcing a bare number. */
	.unread-chip {
		display:inline-flex;
		align-items:center;
		gap:0.3rem;
		padding:0.15rem 0.5rem;
		border-radius:999px;
		background:var(--color-chip-gold-bg);
		border:1px solid var(--color-chip-gold-border);
		color:var(--color-chip-gold-text);
		font-size:0.75rem;
		white-space:nowrap;
	}
	/* 14px against 12px text — the usual optical bump that keeps a line icon
	   from reading smaller than the type beside it. This is the smallest any
	   house mark renders anywhere, which is exactly why `chat` holds the
	   simplest shape in the set (see LogMark's note on the rumor swap). */
	.unread-chip-mark { width:14px; height:14px; flex-shrink:0; }

	.table-id {
		display:flex;
		flex-direction:column;
		/* Badge centres under the (wider) table name. */
		align-items:center;
		gap:0.5rem;
		flex-shrink:0;
	}
	/* Phase badge and unread chip share a row under the table code — both are
	   card-level status, so they read as one cluster. Wraps rather than
	   overflowing if a long phase label meets a 3-digit count. */
	.status-row {
		display:flex;
		align-items:center;
		justify-content:center;
		flex-wrap:wrap;
		gap:0.4rem;
	}
	.table-code { font-size:0.85rem; color:var(--color-text-muted); }
	.code-value { color:var(--color-text); letter-spacing:0.12em; }

	/* Roster pills fill the space right of the table id (the card's column
	   gap keeps a healthy minimum distance) and centre in it both ways,
	   wrapping to as many rows as the names need. */
	.pills {
		flex:1;
		display:flex;
		flex-wrap:wrap;
		justify-content:center;
		align-items:center;
		align-content:center;
		gap:0.4rem;
	}
	.pill {
		display:inline-flex;
		align-items:center;
		gap:0.35rem;
		padding:0.28rem 0.6rem;
		font-size:0.8rem;
		background:var(--color-surface-2);
		border:1px solid var(--color-border);
		border-radius:999px;
	}
	/* Waiting-on = a gold BORDER, the app's single treatment for it (header
	   chips, TableRoster, here). It was a gold tint; the fill was the thing
	   that read as muddy brown, and a green presence ring wrapped around that
	   brown was the worst object on this page (owner, 2026-08-16).

	   It paints your OWN pill too, deliberately. `.your-move` already fills
	   the whole card when the table is blocked on you, so the pill is saying
	   it twice — but a channel that changes meaning depending on whose row it
	   is isn't a channel (owner's ruling). Gold border = "waiting on this
	   player", every row, every surface; the card fill is emphasis stacked on
	   top, not a different statement.

	   Presence is no longer drawn. It was a green ring, and it never said
	   anything about you in the first place: `online` is computed as
	   `p.online || p.id === t.player_id`, so your own pill was hard-coded lit.
	   The word still reaches a screen reader through the pill's aria-label. */
	.pill.waited { border-color:var(--color-accent); }
	.pill-dot {
		width:8px; height:8px;
		border-radius:50%;
		background:var(--pill-color, var(--color-text-muted));
		flex-shrink:0;
	}
	/* These pills WRAP (see .pills) rather than scrolling, so there's no
	   strip to share and no reason for the header's tighter budget — a long
	   name costs a row, not a clipped name. 26ch is the same pathological
	   wide-glyph backstop the header chips use. */
	.pill-name {
		max-width:26ch;
		overflow:hidden;
		white-space:nowrap;
		text-overflow:ellipsis;
	}
	.join { display:flex; gap:0.5rem; }
	/* Secondary "create a table" action, set apart below the primary join row. */
	.create-row { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:0.5rem; margin-top:1rem; padding-top:0.85rem; border-top:1px solid var(--color-border); }
	.create-row .hint { margin:0; }
	.footer-actions { display:flex; justify-content:center; gap:0.75rem; flex-wrap:wrap; margin-top:0.5rem; }
	.feedback-btn { display:inline-flex; align-items:center; justify-content:center; text-decoration:none; }
</style>
