<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/modalShell.css';
	import '$lib/components/shared/statusText.css';
	import { goto, preloadCode } from '$app/navigation';
	import { readProfileSnapshot, writeProfileSnapshot } from '$lib/pageCache';
	import { onMount } from 'svelte';
	import {
		getMe, listMyTables, updateMe, logout,
		createTable, joinTable, setReminderMute,
		type Account, type MyTable,
	} from '$lib/api';
	import { groupTables } from '$lib/tableGroups';
	import TableRow from '$lib/components/profile/TableRow.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import CharCounter from '$lib/components/CharCounter.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import FeedbackForm from '$lib/components/FeedbackForm.svelte';
	import { getPushState, enablePush, disablePush, onPushPermissionChange, type PushState } from '$lib/push';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
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

	// ── Table list ───────────────────────────────────────────────────────────
	// Three groups: where you're needed, where others are, what's over. Each
	// keeps the server's most-recently-joined-first order (lib/tableGroups.ts).
	const groups = $derived(groupTables(tables));

	// Tables whose bell is mid-request, so a double tap can't fire twice.
	let mutingGames = $state<number[]>([]);

	// The bell silences the wait this table is currently blocking (per-wait, not
	// per-table — see migration 056), so there is no separate un-mute state to
	// keep anywhere: the server clears it when the table moves on.
	//
	// Optimistic, then corrected from the response rather than assumed: with no
	// reminder pending the server keeps no quiet and says so, and the row has
	// to show what is true, not what was asked for.
	async function toggleMute(t: MyTable, setMutedTo: boolean) {
		if (mutingGames.includes(t.game_id)) return;
		mutingGames = [...mutingGames, t.game_id];
		const previous = t.reminder_muted;
		applyMuted(t.game_id, setMutedTo);
		try {
			const res = await setReminderMute(t.game_id, setMutedTo);
			applyMuted(t.game_id, res.muted);
		} catch (e) {
			applyMuted(t.game_id, previous);
			error = e instanceof Error ? e.message : 'Could not change reminders for that table.';
		} finally {
			mutingGames = mutingGames.filter((id) => id !== t.game_id);
		}
	}

	function applyMuted(gameID: number, muted: boolean) {
		tables = tables.map((t) => (t.game_id === gameID ? { ...t, reminder_muted: muted } : t));
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

	// Tracks the values the drafts were last seeded with, so the revalidate
	// pass can tell "untouched" from "the player is typing in this field right
	// now". Seeding from cache makes the page interactive a round trip earlier
	// than it used to be, which is the whole point — but it also means someone
	// can be mid-edit when the network answer lands, and that answer must not
	// overwrite them.
	let seededDrafts = { username: '', email: '', cadence: '24' };

	function applyAccount(acct: Account) {
		me = acct;
		const cadence = acct.notify_cadence_hours == null ? 'off' : String(acct.notify_cadence_hours);
		if (usernameDraft === seededDrafts.username) usernameDraft = acct.username;
		if (emailDraft === seededDrafts.email) emailDraft = acct.email ?? '';
		if (cadenceDraft === seededDrafts.cadence) cadenceDraft = cadence;
		seededDrafts = { username: acct.username, email: acct.email ?? '', cadence };
	}

	async function load() {
		// Paint the tables we last saw before touching the network; the fetch
		// below runs regardless and replaces them. Only on the first load —
		// the visibility/focus refetch below calls this too, and that path
		// already has real content on screen.
		const cached = readProfileSnapshot();
		if (cached && !me) {
			applyAccount(cached.account);
			tables = cached.tables;
			loading = false;
		} else {
			loading = true;
		}
		error = '';
		try {
			// Both in one round trip rather than two: listMyTables is
			// session-authenticated and needs nothing from the account object, so
			// awaiting getMe first only ever added a hop. Production charges a
			// fixed ~200-480ms per request regardless of what it does, which made
			// that hop the single cheapest thing to delete on this page.
			//
			// listMyTables 401s for a signed-out visitor exactly as getMe returns
			// null, so the redirect below still fires; its rejection is caught
			// here and discarded, because the getMe result is what decides.
			const tablesPromise = withTimeout(listMyTables()).catch(() => null);
			const acct = await withTimeout(getMe());
			if (!acct) { goto('/'); return; }
			applyAccount(acct);
			const res = await tablesPromise;
			if (!res) throw new Error('Could not load your tables.');
			tables = res.tables;
			writeProfileSnapshot({ account: acct, tables: res.tables });
			getPushState().then((s) => { pushState = s; });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load profile.';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	// ── Preload the table route ──────────────────────────────────────────────
	// Nearly every player here has exactly one active table and is about to
	// tap it, and production's ~400ms request floor plus ~170KB/s transfer
	// puts the table chunk (~97KB gz) at about a second on the tap path. So
	// fetch it as soon as we know they have a table, while they read the
	// rows. One call covers every table: they all share the [id] route.
	//
	// Programmatic rather than data-sveltekit-preload-code="viewport" on the
	// links: SvelteKit only registers viewport observers for anchors present
	// when navigation completes, and these rows render after a fetch, so the
	// attribute would silently do nothing on a cold load — the visit that
	// matters. The body's global preload-data="hover" still runs on
	// touchstart, but that is ~100ms before navigation, too late to hide a
	// second of download.
	let tableCodePreloaded = false;
	$effect(() => {
		if (tableCodePreloaded || tables.length === 0) return;
		tableCodePreloaded = true;
		void preloadCode(`/table/${tables[0].game_id}`).catch(() => {
			// Best effort: a failed preload just means the tap pays full price.
		});
	});

	// ── Refresh on return ────────────────────────────────────────────────────
	// The table rows (roster, title, waiting-on, unread) come from one fetch
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
			// Leave the rows showing what they already had — a failed
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
		{#if notice}<Notice tone="success" live>{notice}</Notice>{/if}

		{#if tables.length > 0}
			<section class="section" aria-labelledby="tables-heading">
				<h2 id="tables-heading">Your tables</h2>
				{#each groups as group (group.key)}
					<h3 class="group-heading" id={`group-${group.key}`}>{group.heading}</h3>
					<ul class="table-list" aria-labelledby={`group-${group.key}`}>
						{#each group.tables as t (t.game_id)}
							<TableRow table={t} mutePending={mutingGames.includes(t.game_id)} onMute={toggleMute} />
						{/each}
					</ul>
				{/each}
			</section>
		{/if}

		<section class="section">
			<h2>Join a table</h2>
			<p class="hint">Have a code from your host? Enter it to take a seat.</p>
			<div class="join">
				<input class="code-input" aria-label="Join code" placeholder="Join code" bind:value={joinCode} maxlength={6} />
				<button class="action-btn primary" onclick={doJoin} disabled={busy || !joinCode.trim()}>Join</button>
			</div>
			<div class="create-row">
				<span class="hint">Hosting a game?</span>
				<button class="action-btn secondary" onclick={doCreate} disabled={busy}>Create a new table</button>
			</div>
		</section>

		<section class="section">
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
					<button class="action-btn secondary" onclick={togglePush} disabled={pushBusy}>
						{pushBusy ? '…' : pushState === 'on' ? 'Turn off' : 'Turn on'}
					</button>
				{/if}
			</div>
			<p class="hint push-hint">
				The cadence applies to your whole account; push must be turned
				on separately on each device/browser you want reminders on.
			</p>
		</section>

		<section class="section">
			<h2>Account</h2>
			<div class="row">
				<span class="label">Player name</span>
				{#if editingUsername}
					<input aria-label="Player name" bind:value={usernameDraft} maxlength={TEXT_LIMITS.USERNAME} />
					<CharCounter value={usernameDraft} max={TEXT_LIMITS.USERNAME} />
					<button class="action-btn primary" onclick={saveUsername}>Save</button>
					<button class="action-btn secondary" onclick={() => { editingUsername = false; usernameDraft = me?.username ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.username}</span>
					<button class="action-btn secondary" aria-label="Edit player name" onclick={() => { editingUsername = true; }}>Edit</button>
				{/if}
			</div>
			<!-- TODO: Hook up backend email handling -->
			<!-- <div class="row">
				<span class="label">Email</span>
				{#if editingEmail}
					<input type="email" aria-label="Email" bind:value={emailDraft} />
					<button class="action-btn primary" onclick={saveEmail}>Save</button>
					<button class="action-btn secondary" onclick={() => { editingEmail = false; emailDraft = me?.email ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.email ?? 'Not set. For notifications and password recovery.'}</span>
					<button class="action-btn secondary" aria-label="Edit email" onclick={() => { editingEmail = true; }}>Edit</button>
				{/if}
			</div> -->
			<div class="row">
				<span class="label">Password</span>
				{#if editingPassword}
					<input type="password" aria-label="New password" bind:value={passwordDraft} placeholder="Enter a new password" />
					<button class="action-btn primary" onclick={savePassword} disabled={!passwordDraft}>Save</button>
					<button class="action-btn secondary" onclick={() => { editingPassword = false; passwordDraft = ''; }}>Cancel</button>
				{:else}
					<span class="masked">••••••••</span>
					<button class="action-btn secondary" aria-label="Edit password" onclick={() => { editingPassword = true; }}>Edit</button>
				{/if}
			</div>
		</section>

		<div class="footer-actions">
			<button class="action-btn secondary" onclick={() => feedbackOpen = true}>Send feedback</button>
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
	/* One phone-width column at every window width (docs/STYLE_GUIDE.md "Layout
	   widths": the column cap is 440, and extra width becomes centring margin).
	   The gutter is ours, not main's (main.flush in +layout.svelte): inside the
	   cap, so the column measures a true 440 at that viewport. There are no
	   boxes on this page. A section is a heading and space; the tables are rows
	   with a line between them; only the controls (inputs, buttons) are drawn
	   as objects. */
	.profile {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		max-width: 440px;
		margin: 0 auto;
		padding: 0 var(--space-4) var(--space-5);
	}
	.wordmark {
		text-align: center;
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		font-size: var(--text-title);
		color: var(--color-accent);
	}
	/* The small connecting word is the smallest step that reads at this size. */
	.wordmark .the { font-size: var(--text-small); letter-spacing: 0.1em; }

	.section {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	h2 {
		color: var(--color-accent);
		font-size: var(--text-heading);
	}
	.hint {
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}

	/* ── The table list ─────────────────────────────────────────────────────
	   A small heading and a rule per group, then the rows (TableRow). The rule
	   under the heading doubles as the line above the group's first row. */
	.group-heading {
		margin-top: var(--space-4);
		padding-bottom: var(--space-1);
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-label);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}
	h2 + .group-heading { margin-top: var(--space-1); }
	.table-list { list-style: none; }

	/* ── Controls ───────────────────────────────────────────────────────── */
	/* A control standing on the page: the raised surface and the neutral control
	   border. 16px type, because iOS Safari zooms the page when a focused input
	   is set smaller. */
	input, select {
		min-height: var(--size-tap);
		padding: 0 var(--space-3);
		font-family: inherit;
		font-size: var(--text-body);
		color: var(--color-text);
		background: var(--color-surface);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
	}
	input:focus, select:focus {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	input::placeholder { color: var(--color-text-faint); }
	.join { display: flex; gap: var(--space-2); }
	.join input { flex: 1; min-width: 0; }
	.code-input { letter-spacing: 0.15em; text-transform: uppercase; }
	/* "Create a new table" is a secondary button, set under the join row, so Join
	   is the only filled button on the page. */
	.create-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding-top: var(--space-2);
	}

	/* ── Notifications and Account: label, value, control, a line between ─── */
	.row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding: var(--space-2) 0;
		border-bottom: 1px solid var(--color-border);
	}
	.row:last-child { border-bottom: none; }
	/* The field label sits on its own line so the value and its buttons get the
	   full width instead of cramping. */
	.label {
		width: 100%;
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}
	.masked { letter-spacing: 0.15em; color: var(--color-text-secondary); }
	.row input, .row select { flex: 1; min-width: 0; }
	.row span:not(.label) { flex: 1; min-width: 0; }
	.push-row { border-bottom: none; }
	/* The blocked case is a paragraph plus a numbered list, not a value that can
	   sit inline next to its label like the other rows' controls. */
	.push-blocked-row { flex: 1 1 100%; display: flex; flex-direction: column; gap: var(--space-2); }
	.push-hint { margin-top: calc(-1 * var(--space-1)); }

	/* Renders in place of .profile, so it carries the same gutter: main.flush no
	   longer supplies one. */
	.load-error {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-4);
		max-width: 440px;
		margin: 0 auto;
		padding: var(--space-6) var(--space-4) 0;
	}
	.footer-actions {
		display: flex;
		justify-content: center;
		gap: var(--space-3);
		flex-wrap: wrap;
	}
</style>
