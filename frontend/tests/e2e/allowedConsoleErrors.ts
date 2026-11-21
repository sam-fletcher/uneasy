/**
 * The `console.error` lines an e2e test may log without failing.
 *
 * tests/e2e/fixtures.ts fails any test whose page logs a `console.error` (and
 * any test whose page throws something nothing caught, which has no
 * allow-list: that is always a bug). Chrome writes a red "Failed to load
 * resource" line for every 4xx/5xx a page's own fetch gets, so a few specs log
 * on purpose: they fake a server failure, cut the socket, or load a logged-out
 * page. Each such line is named here, with the spec that makes it and why.
 *
 * Keep it short. An entry is a place the suite agrees to stay blind, so it has
 * to be narrow: the spec's file name (always), the test within it when the line
 * comes from one test, and a pattern that names the URL or message rather than
 * every error. Past ten entries, ask whether the harness
 * should change instead (the 403s a page gets once its game is deleted are not
 * here: `deleteGame` in helpers.ts tells the guard to stop listening).
 *
 * This is plain data and a matcher, with no Playwright in it, so that
 * src/lib/e2eFixtures.test.ts (part of `make check-fast`) can check the entries
 * themselves: a spec that was renamed, and a tracker item that was finished.
 */

export interface AllowedConsoleError {
	/** Matched against the line Chrome logged, with ` @ <url>` appended when it names one. */
	match: RegExp;
	/** The spec file that makes the line (`chat.spec.ts`). Required: an entry with no spec would allow its line everywhere. */
	spec: string;
	/** Part of the title of the one test that makes the line, when it is one test and not the whole spec. */
	test?: string;
	/** Why the app is meant to log this, or what the bug is. One sentence. */
	reason: string;
	/** For a line that is an app bug: the `docs/work/next/<item>.md` that tracks it, without the `.md`.
	 *  A test fails once that file is gone, so the entry leaves with the fix. */
	item?: string;
}

export const ALLOWED_CONSOLE_ERRORS: readonly AllowedConsoleError[] = [
	{
		spec: 'chat.spec.ts',
		test: 'a failed send restores the text',
		match: /status of 500 .* @ \S+\/api\/tables\/\d+\/posts$/,
		reason: 'The spec fulfils POST /posts with a 500 to check that a failed send puts the text back.',
	},
	{
		spec: 'make_war.spec.ts',
		test: 'a failed refresh never reopens it',
		match: /status of 500 .* @ \S+\/api\/tables\/\d+\/wars$/,
		reason: 'The spec fulfils GET /wars with a 500 to check that a failed refresh never reopens a closed war sheet.',
	},
	{
		spec: 'connection_notice.spec.ts',
		match: /^WebSocket connection to 'wss:\/\/127\.0\.0\.1:\d+\/' failed: .*ERR_CONNECTION_REFUSED/,
		reason: 'The spec points the page at a dead port to make the socket drop, and the page retries it.',
	},
	{
		spec: 'connection_notice.spec.ts',
		test: 'a reconnect whose fetch fails keeps the notice up',
		match: /^Failed to load resource: net::ERR_FAILED @ \S+\/api\/tables\/\d+\/state$/,
		reason: 'The spec aborts the first /state fetch after a reconnect to check that the notice stays up.',
	},
	{
		spec: 'styleguide.spec.ts',
		match: /status of 401 .* @ \S+\/api\/accounts\/me$/,
		reason: 'The style guide is opened logged out, and a 401 is how /accounts/me says so (getMe treats it as the answer, not a failure).',
	},
	{
		spec: 'make_demands.spec.ts',
		match: /status of 400 .* @ \S+\/api\/plans\/undefined$/,
		reason: 'Bug: ws-handlers reads plan_id from demand.prepared, whose payload is { plan }, and fetches /plans/undefined.',
		item: 'demand-prepared-plan-id',
	},
	{
		spec: 'public_record.spec.ts',
		test: 'a war the server stored as "make"',
		match: /status of 404 .* @ \S+\/api\/plans\/\d+\/war-state$/,
		reason: 'Bug: a Make War plan shown before its war exists makes the page ask for war-state, which answers 404 by design.',
		item: 'make-war-state-404',
	},
	{
		// The guard's own canary (tests/e2e/fixtures.spec.ts): an allow-listed line must not fail a test.
		spec: 'fixtures.spec.ts',
		test: 'a console.error the allow-list names',
		match: /^canary: an allow-listed console\.error/,
		reason: 'The canary that checks the allow-list lets a named line through.',
	},
];

/** Which test a console.error came from: its spec file, and its own title. */
export interface Origin {
	spec: string;
	title: string;
}

/**
 * Whether `text` (a console.error line, with ` @ <url>` when Chrome gave one)
 * from this spec and test is on the list.
 */
export function isAllowedConsoleError(
	text: string,
	from: Origin,
	allowed: readonly AllowedConsoleError[] = ALLOWED_CONSOLE_ERRORS,
): boolean {
	return allowed.some(
		a => a.spec === from.spec && (a.test === undefined || from.title.includes(a.test)) && a.match.test(text),
	);
}
