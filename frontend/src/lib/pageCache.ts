// In-memory page snapshots, so returning to a page you just left paints
// immediately instead of re-earning every byte from scratch.
//
// Why this exists: SvelteKit unmounts a route's component on navigation, so
// profile → table A → profile → table A re-ran every fetch from cold. That is
// invisible on a fast host and very visible on this one — production charges a
// fixed ~200-480ms per request no matter how trivial it is, and a table load
// spends three rounds of that plus the DB time inside them.
//
// The contract is stale-while-revalidate, not "cache": a snapshot is only ever
// used to paint something on screen *now*, and the page's normal load runs
// immediately afterwards and overwrites it. Nothing here is a source of truth,
// nothing here is written back to the server, and the table page additionally
// has a WebSocket that repairs anything stale within a second of connecting.

import type {
	Game, Player, ToneTopic, Ranking, Asset,
	Law, Rumor, ChatPost, RecordRow, PresenceMember, PlayerActivity,
	DiceRoll, DiceRollDie, VoteView, RollParticipant, BankedDie,
	Plan, PlanToken, Secret, Scene, ScenePeerView, RowState, MyTable, Account,
} from '$lib/api';

// Beyond this age a snapshot is dropped and the page loads normally. The point
// is to cover navigation — flipping between your tables and your profile —
// not to serve genuinely old state. Five minutes covers every realistic
// back-and-forth while keeping the worst case someone can see to "a few
// minutes behind", which the revalidate then corrects.
const MAX_AGE_MS = 5 * 60_000;

// Hard ceiling on retained table snapshots. Expiry alone does not bound this:
// an entry is only dropped when its own key is read, so a table visited once
// and never returned to would sit in the map for the life of the tab. A player
// realistically flips between a handful of tables, so six costs nothing and
// makes the memory a fixed quantity rather than "however many tables you
// happened to open". A snapshot is ~22KB of JSON for a typical table (6KB of
// game state plus a 50-post chat window), more if the player has scrolled a
// long way back through the log before navigating away.
const MAX_TABLES = 6;

export interface TableSnapshot {
	game: Game | null;
	players: Player[];
	toneTopics: ToneTopic[];
	rankings: Ranking[];
	assets: Asset[];
	laws: Law[];
	rumors: Rumor[];
	members: PresenceMember[];
	playerActivity: PlayerActivity[];
	secrets: Secret[];
	currentPlayerID: number | null;
	prologueActivePlayerID: number | null;
	recordRows: RecordRow[];
	plans: Plan[];
	planTokens: PlanToken[];
	activeScene: Scene | null;
	activeScenePeers: ScenePeerView[];
	rowState: RowState | null;
	bankedDice: BankedDie[];
	activeRoll: DiceRoll | null;
	activeRollDice: DiceRollDie[];
	activeRollVotes: VoteView[];
	activeRollParticipants: RollParticipant[];
	// The chat window rides along deliberately. Seeding it avoids an empty-feed
	// flash, and it makes the reconnect cheaper rather than riskier:
	// reconnectResync sees a non-empty window and issues an incremental `after`
	// fetch instead of a full one, and if the gap turns out to exceed the
	// server's catch-up cap it re-windows from scratch on its own.
	//
	// Only ever captured in 'live' mode — a history window is a place the
	// player navigated to deliberately, and restoring one on a fresh mount
	// would strand them in the past with reconnectResync (which no-ops in
	// history mode) declining to catch them up.
	chatPosts: ChatPost[];
	chatHasMoreBefore: boolean;
	chatLastReadPostID: number;
	chatInitialReadMarker: number;
}

interface Entry<T> { at: number; value: T; }

const tableCache = new Map<string, Entry<TableSnapshot>>();
// The account rides along with the tables because the profile page renders
// nothing at all until `me` is set — caching the tables alone still left it
// waiting a full round trip on getMe before it could paint them.
export interface ProfileSnapshot { account: Account; tables: MyTable[]; }

let profileCache: Entry<ProfileSnapshot> | null = null;

function fresh<T>(entry: Entry<T> | null | undefined): T | null {
	if (!entry) return null;
	if (Date.now() - entry.at > MAX_AGE_MS) return null;
	return entry.value;
}

export function readTableSnapshot(gameID: string | number): TableSnapshot | null {
	const key = String(gameID);
	const hit = fresh(tableCache.get(key));
	if (!hit) tableCache.delete(key);
	return hit;
}

export function writeTableSnapshot(gameID: string | number, value: TableSnapshot): void {
	const key = String(gameID);
	// Delete before set so Map iteration order tracks write recency — that is
	// what makes "evict the first key" a correct LRU below. A plain set() on an
	// existing key keeps its original position and would evict the wrong one.
	tableCache.delete(key);
	tableCache.set(key, { at: Date.now(), value });

	// Sweep the expired before evicting the merely old, so a live table is
	// never dropped in favour of a stale one. Deleting while iterating a Map is
	// well-defined.
	const now = Date.now();
	for (const [k, entry] of tableCache) {
		if (now - entry.at > MAX_AGE_MS) tableCache.delete(k);
	}
	while (tableCache.size > MAX_TABLES) {
		const oldest = tableCache.keys().next();
		if (oldest.done) break;
		tableCache.delete(oldest.value);
	}
}

export function readProfileSnapshot(): ProfileSnapshot | null {
	const hit = fresh(profileCache);
	if (!hit) profileCache = null;
	return hit;
}

export function writeProfileSnapshot(value: ProfileSnapshot): void {
	profileCache = { at: Date.now(), value };
}

/** Wipes every snapshot. Must be called on sign-out: these hold one account's
 *  roster, tables and secrets, and a second person signing in on the same
 *  browser must never be painted the first one's data, however briefly. */
export function clearPageCache(): void {
	tableCache.clear();
	profileCache = null;
}
