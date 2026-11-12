// ws-handlers.ts — the WebSocket message dispatcher for the table page,
// extracted from +page.svelte. handleWSMessage mutates the page's reactive
// state through a WSContext whose accessors are backed by the component's
// $state runes, so assignments here stay reactive in the component.

import { EventTypes, type WSMessage } from '$lib/ws';
import { getRoll, listBankedDice, getVisibleSecrets, getPlan, listPlans, listPlanTokens } from '$lib/api';
import type {
	Game, Player, PresenceMember, ToneTopic, Ranking, Asset, Marginalium,
	Law, Rumor, Secret, ChatPost, SceneEntry, RecordRow, RowState, Scene,
	ScenePeerView, SceneSetupDraft, PreparePlanDraft, DiceRoll, DiceRollDie,
	VoteView, RollParticipant, BankedDie, Plan, PlanToken, ActiveRollPayload,
} from '$lib/api';
import { appendLivePost, type ChatFeedContext } from '$lib/chatFeed';
import { acceptToneEcho, type ToneWrite } from '$lib/toneWrites';

/**
 * Mutable view of the table page's WS-synced state. Each property is backed
 * by a component $state rune via get/set accessors, so handleWSMessage can
 * read and assign exactly as the inline version did.
 */
export interface WSContext {
	readonly gameID: string;
	loadGameState: () => void;
	game: Game | null;
	players: Player[];
	members: PresenceMember[];
	toneTopics: ToneTopic[];
	/** In-flight tone writes by topic id — see $lib/toneWrites. Read (not
	 *  written) here, to tell this client's own tone.updated echoes from
	 *  another player's edit; the payload carries no originating player id. */
	readonly toneWrites: Map<number, ToneWrite>;
	rankings: Ranking[];
	assets: Asset[];
	laws: Law[];
	rumors: Rumor[];
	secrets: Secret[];
	/** Owns the contiguous post window, live/history mode, and cursors — see
	 *  $lib/chatFeed. A stable reference; ScenePostCreated mutates its .posts
	 *  field via appendLivePost rather than reassigning this property. */
	readonly chatFeed: ChatFeedContext;
	recordRows: RecordRow[];
	rowState: RowState | null;
	activeScene: Scene | null;
	activeScenePeers: ScenePeerView[];
	sceneSetupDraft: SceneSetupDraft | null;
	preparePlanDraft: PreparePlanDraft | null;
	activeRoll: DiceRoll | null;
	activeRollDice: DiceRollDie[];
	activeRollVotes: VoteView[];
	activeRollParticipants: RollParticipant[];
	bankedDice: BankedDie[];
	plans: Plan[];
	planTokens: PlanToken[];
	prologueActivePlayerID: number | null;
	typingNames: string[];
	readonly typingMap: Map<number, string>;
	readonly typingTimeouts: Map<number, ReturnType<typeof setTimeout>>;
}

// The throne is established the first time a `monarch` title is claimed, and the
// flag never flips back (ADR-007). The backend sets it in the same path that
// adds the monarch-title marginalia, but emits no game-state event — so clients
// learn of it from that marginalia arriving (MarginaliaAdded for a main-character
// claim, or AssetCreated for an extra-peer claim). Flipping it here keeps the
// crown UI live without a refetch; monotonic, so re-seeing the title is a no-op.
function establishThroneIfMonarch(ctx: WSContext, marginalia: Marginalium[]) {
	if (!ctx.game || ctx.game.throne_established) return;
	if (marginalia.some(m => m.title === 'monarch')) {
		ctx.game = { ...ctx.game, throne_established: true };
	}
}

export function handleWSMessage(ctx: WSContext, msg: WSMessage) {
	switch (msg.type) {
		case EventTypes.PhaseChanged: {
			const newPhase = msg.payload.phase as Game['phase'];
			if (ctx.game) ctx.game = { ...ctx.game, phase: newPhase };
			ctx.loadGameState();
			break;
		}
		case EventTypes.PlayerJoined: {
			const player = msg.payload.player as Player;
			if (!ctx.players.find(p => p.id === player.id)) {
				ctx.players = [...ctx.players, player];
				ctx.members = [...ctx.members, { id: player.id, display_name: player.display_name, online: false }];
			}
			break;
		}
		case EventTypes.PresenceSnapshot: {
			const snap = msg.payload.members as PresenceMember[];
			ctx.members = ctx.members.map(m => ({
				...m,
				online: snap.some(s => s.id === m.id)
			}));
			break;
		}
		case EventTypes.TypingUpdate: {
			const { player_id, display_name, typing } = msg.payload as {
				player_id: number; display_name: string; typing: boolean;
			};
			const t = ctx.typingTimeouts.get(player_id);
			if (t) clearTimeout(t);
			if (typing) {
				ctx.typingMap.set(player_id, display_name);
				ctx.typingTimeouts.set(player_id, setTimeout(() => {
					ctx.typingMap.delete(player_id);
					ctx.typingNames = [...ctx.typingMap.values()];
				}, 4000));
			} else {
				ctx.typingMap.delete(player_id);
				ctx.typingTimeouts.delete(player_id);
			}
			ctx.typingNames = [...ctx.typingMap.values()];
			break;
		}
		case EventTypes.ToneUpdated: {
			const { topic_id, topic, status } = msg.payload as {
				topic_id: number; topic: string; status: string;
			};
			// Applying our own echoes made a rapidly-cycled tile replay the
			// cycle one step per round trip. A topic with no write in flight
			// — every other player's edit, and every freshly added topic —
			// falls straight through.
			if (!acceptToneEcho(ctx.toneWrites, topic_id, status as ToneTopic['status'])) break;
			const idx = ctx.toneTopics.findIndex(t => t.id === topic_id);
			if (idx >= 0) {
				ctx.toneTopics = ctx.toneTopics.map(t =>
					t.id === topic_id ? { ...t, status: status as ToneTopic['status'] } : t
				);
			} else {
				ctx.toneTopics = [...ctx.toneTopics, {
					id: topic_id, game_id: Number(ctx.gameID), topic,
					status: status as ToneTopic['status']
				}];
			}
			break;
		}
		case EventTypes.RankingsUpdated: {
			ctx.rankings = msg.payload.rankings as Ranking[];
			// A ranking update may clear plan tokens for a full category.
			refreshPlanTokens(ctx);
			break;
		}
		case EventTypes.FocusChanged: {
			if (ctx.game) ctx.game = { ...ctx.game, focus_player_id: msg.payload.player_id as number };
			// rowState is updated separately by RowStateChanged.
			ctx.sceneSetupDraft = null;
			ctx.preparePlanDraft = null;
			break;
		}
		case EventTypes.PrologueTurnAdvanced: {
			ctx.prologueActivePlayerID = (msg.payload.current_player_id as number | null) ?? null;
			break;
		}
		case EventTypes.RowAdvanced: {
			const newRow = msg.payload.row_number as number;
			if (ctx.game) ctx.game = { ...ctx.game, current_row: newRow };
			// Chat is now a single continuous game-wide feed; no reset needed.
			break;
		}
		case EventTypes.SceneEnded: {
			// activeScene is content state (which scene is showing); rowState
			// is step state. Both need to update — the latter via
			// RowStateChanged, which arrives separately.
			ctx.activeScene = null;
			ctx.activeScenePeers = [];
			break;
		}
		case EventTypes.SceneStarted: {
			const scene = msg.payload.scene as Scene;
			const peers = msg.payload.peers as ScenePeerView[];
			ctx.activeScene = scene;
			ctx.activeScenePeers = peers;
			ctx.sceneSetupDraft = null;
			break;
		}
		case EventTypes.SceneSetupDraft: {
			ctx.sceneSetupDraft = msg.payload as SceneSetupDraft;
			break;
		}
		case EventTypes.PreparePlanDraft: {
			ctx.preparePlanDraft = msg.payload as PreparePlanDraft;
			break;
		}
		case EventTypes.RowStateChanged: {
			ctx.rowState = msg.payload.row_state as RowState;
			break;
		}
		case EventTypes.ScenePeerClaimed: {
			const { scene_id, peer_asset_id, controller_id } = msg.payload as {
				scene_id: number; peer_asset_id: number; controller_id: number;
			};
			if (ctx.activeScene && ctx.activeScene.id === scene_id) {
				ctx.activeScenePeers = ctx.activeScenePeers.map(p =>
					p.peer_asset_id === peer_asset_id
						? { ...p, controller_player_id: controller_id }
						: p
				);
			}
			break;
		}
		case EventTypes.ScenePostCreated: {
			const post = msg.payload.post as ChatPost;
			appendLivePost(ctx.chatFeed, post);
			break;
		}
		case EventTypes.SceneEntryCreated: {
			const entry = msg.payload.entry as SceneEntry;
			ctx.recordRows = ctx.recordRows.map(row =>
				row.row_number === entry.row_number
					? { ...row, entries: row.entries.find(e => e.id === entry.id)
						? row.entries
						: [...row.entries, entry] }
					: row
			);
			break;
		}
		// Asset events
		case EventTypes.AssetCreated: {
			const asset = msg.payload.asset as Asset;
			if (!ctx.assets.find(a => a.id === asset.id)) {
				ctx.assets = [...ctx.assets, asset];
			}
			// Extra-peer monarch claim (≤3-player games) arrives as a new asset
			// carrying its title marginalia — establish the throne if so.
			establishThroneIfMonarch(ctx, asset.marginalia ?? []);
			break;
		}
		case EventTypes.AssetUpdated: {
			const asset = msg.payload.asset as Asset;
			ctx.assets = ctx.assets.map(a => a.id === asset.id ? asset : a);
			break;
		}
		case EventTypes.AssetTaken: {
			// The take payload is a raw asset row — it carries the new owner_id but
			// NOT marginalia or secret_count (those aren't columns on the asset
			// table). Merge over the existing entry so we apply the ownership change
			// without dropping marginalia; a take never alters the notes. Replacing
			// wholesale would leave marginalia undefined and crash every consumer
			// that reads it (isNeedlesslyAtRisk, AssetCardSelectable, the header
			// at-risk count), which throws inside a derived and freezes reactivity.
			const asset = msg.payload.asset as Asset;
			ctx.assets = ctx.assets.map(a => a.id === asset.id ? { ...a, ...asset } : a);
			break;
		}
		case EventTypes.AssetLeveraged: {
			const { asset_id } = msg.payload as { asset_id: number };
			ctx.assets = ctx.assets.map(a =>
				a.id === asset_id ? { ...a, is_leveraged: true } : a
			);
			break;
		}
		case EventTypes.AssetRefreshed: {
			const { asset_id } = msg.payload as { asset_id: number };
			ctx.assets = ctx.assets.map(a =>
				a.id === asset_id ? { ...a, is_leveraged: false } : a
			);
			break;
		}
		case EventTypes.AssetDestroyed: {
			const { asset_id, destroyed_at } = msg.payload as { asset_id: number; destroyed_at?: string };
			// Use the server's stamp so the tombstone order matches a reload's;
			// the client clock is only a fallback for a payload without one.
			const stamp = destroyed_at ?? new Date().toISOString();
			// Keep the asset, flagged, as a reload's list does (the server's list
			// includes destroyed assets). Panels look names up by id — "Broke bob's
			// resource" — and the retinue shows a tombstone. Every picker already
			// filters is_destroyed; dropping the row made live and reload disagree.
			ctx.assets = ctx.assets.map(a =>
				a.id === asset_id ? { ...a, is_destroyed: true, destroyed_at: stamp } : a
			);
			break;
		}
		case EventTypes.MarginaliaAdded: {
			const { asset_id, marginalia } = msg.payload as { asset_id: number; marginalia: Marginalium };
			ctx.assets = ctx.assets.map(a => {
				if (a.id !== asset_id) return a;
				const already = a.marginalia.find(m => m.id === marginalia.id);
				return already ? a : { ...a, marginalia: [...a.marginalia, marginalia] };
			});
			// Main-character monarch claim establishes the throne (ADR-007).
			establishThroneIfMonarch(ctx, [marginalia]);
			break;
		}
		case EventTypes.MarginaliaUpdated: {
			const { asset_id, marginalia } = msg.payload as { asset_id: number; marginalia: Marginalium };
			ctx.assets = ctx.assets.map(a => {
				if (a.id !== asset_id) return a;
				return { ...a, marginalia: a.marginalia.map(m => m.id === marginalia.id ? marginalia : m) };
			});
			break;
		}
		case EventTypes.MarginaliaTorn: {
			const { asset_id, position } = msg.payload as { asset_id: number; position: number };
			ctx.assets = ctx.assets.map(a => {
				if (a.id !== asset_id) return a;
				return {
					...a,
					marginalia: a.marginalia.map(m =>
						m.position === position ? { ...m, is_torn: true } : m
					)
				};
			});
			break;
		}
		// Dice roll events
		case EventTypes.RollCreated: {
			const roll = msg.payload.roll as DiceRoll;
			// Fetch full details (includes dice + participants) so we're in sync.
			fetchRoll(ctx, roll);
			// A plan-linked roll may have been cast in the same request that mutated
			// the plan's resolution_data (e.g. Chronicle Histories' cast-roll flips
			// invoke_phase_closed). The preparer's own API response refreshes their
			// plan; other clients only get this event, so refetch the plan here or
			// their pre-roll UI stays stale until a manual page refresh.
			if (roll.plan_id != null) refreshPlan(ctx, roll.plan_id);
			break;
		}
		case EventTypes.RollLeverageAdded: {
			const { roll_id, die } = msg.payload as { roll_id: number; die: DiceRollDie };
			supersedeRollFetch(ctx, roll_id);
			// Append the broadcast die row directly. Refetching the whole roll
			// per event raced: two commits in quick succession issued overlapping
			// getRoll calls, and an older response landing last clobbered the
			// newer dice list, so a die went missing until a manual refresh.
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				if (!ctx.activeRollDice.some(d => d.id === die.id)) {
					ctx.activeRollDice = [...ctx.activeRollDice, die];
				}
				// Banked-die spend or asset leverage may have changed our
				// unspent-banked list; refresh.
				listBankedDice(ctx.gameID).then(d => { ctx.bankedDice = d.dice; }).catch(() => {});
			}
			break;
		}
		case EventTypes.RollStageChanged: {
			const { roll_id, stage } = msg.payload as { roll_id: number; stage: DiceRoll['stage'] };
			supersedeRollFetch(ctx, roll_id);
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRoll = { ...ctx.activeRoll, stage };
			}
			break;
		}
		case EventTypes.RollIntentSet: {
			const { roll_id, player_id, intent } = msg.payload as {
				roll_id: number; player_id: number; intent: 'aid' | 'interfere';
			};
			supersedeRollFetch(ctx, roll_id);
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRollParticipants = ctx.activeRollParticipants.map(p =>
					p.player_id === player_id ? { ...p, intent } : p
				);
			}
			break;
		}
		case EventTypes.RollReadyChanged: {
			const { roll_id, player_id, is_ready } = msg.payload as {
				roll_id: number; player_id: number; is_ready: boolean;
			};
			supersedeRollFetch(ctx, roll_id);
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRollParticipants = ctx.activeRollParticipants.map(p =>
					p.player_id === player_id ? { ...p, is_ready } : p
				);
			}
			break;
		}
		case EventTypes.RollVoteCast: {
			const { roll_id, player_id } = msg.payload as {
				roll_id: number; player_id: number;
			};
			supersedeRollFetch(ctx, roll_id);
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				// Hidden ballot: only that the player voted.
				ctx.activeRollVotes = [
					...ctx.activeRollVotes.filter(v => v.player_id !== player_id),
					{ roll_id, player_id, voted: true }
				];
			}
			break;
		}
		case EventTypes.RollVoteResolved: {
			const { roll_id, adjusted_difficulty, ballot } = msg.payload as {
				roll_id: number; adjusted_difficulty: number; ballot: VoteView[];
			};
			supersedeRollFetch(ctx, roll_id);
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRoll = { ...ctx.activeRoll, adjusted_difficulty };
				ctx.activeRollVotes = ballot;
			}
			break;
		}
		case EventTypes.RollResolved: {
			const resolvedRoll = msg.payload.roll as DiceRoll;
			const resolvedDice = msg.payload.dice as DiceRollDie[];
			const onScreen = ctx.activeRoll?.id === resolvedRoll.id;
			// A roll cast and resolved in one request (the last duel bout, a
			// festivity guest roll nobody can leverage) sends roll.created and
			// roll.resolved back to back, so this can arrive while the roll's own
			// fetch is still out and nothing is on screen for it yet. The event
			// carries the resolved roll and its dice, so install them now rather
			// than dropping the event and waiting for a fetch that predates it.
			// A page resync still out is the same case: its snapshot may predate
			// this resolution, and the roll may not be on screen for it to patch.
			if (onScreen || rollFetchInFlight(ctx, resolvedRoll.id) || rollSyncPending(ctx)) {
				if (!onScreen) {
					// Whatever roll was still showing is not this one: its
					// participants and ballot must not sit under the new roll.
					ctx.activeRollVotes = [];
					ctx.activeRollParticipants = [];
				}
				ctx.activeRoll = resolvedRoll;
				ctx.activeRollDice = resolvedDice;
			}
			// The payload has no participants or ballot. If a fetch was out, it
			// predates the resolution: drop it and fetch again.
			supersedeRollFetch(ctx, resolvedRoll.id);
			break;
		}
		// Plan events
		case EventTypes.PlanPrepared: {
			const prepared = msg.payload.plan as Plan;
			const idx = ctx.plans.findIndex(p => p.id === prepared.id);
			ctx.plans = idx >= 0
				? ctx.plans.map(p => p.id === prepared.id ? prepared : p)
				: [...ctx.plans, prepared];
			// Mirror the plan onto the public record so its chip appears
			// without a full reload. row_number is null while a variable-delay
			// plan awaits its reveal; this event re-broadcasts once the real
			// row is assigned (see upsertPlanIntoRecord).
			upsertPlanIntoRecord(ctx, prepared);
			// Preparing places a token; refresh the pips for every viewer.
			refreshPlanTokens(ctx);
			// A secret Spread Rumors prep stashes the rumor text as a hidden
			// Secret on the preparer's asset (not broadcast, to avoid leaking the
			// text). Refetch visible secrets so the preparer sees their new note;
			// the per-viewer endpoint keeps it hidden from everyone else.
			if (prepared.plan_type === 'spread_rumors') {
				getVisibleSecrets(ctx.gameID).then(d => { ctx.secrets = d.secrets; }).catch(() => {});
			}
			// Plan was committed; clear the highlight broadcast.
			ctx.preparePlanDraft = null;
			break;
		}
		case EventTypes.PlanResolving: {
			const resolving = msg.payload.plan as Plan;
			nextPlanSeq(resolving.id);
			ctx.plans = ctx.plans.map(p => p.id === resolving.id ? resolving : p);
			upsertPlanIntoRecord(ctx, resolving);
			break;
		}
		case EventTypes.PlanResolved: {
			const { plan_id, result } = msg.payload as { plan_id: number; result: string };
			// Drop any refetch still in flight — it predates this event — and
			// take a fresh snapshot, since the patch below only knows the status.
			refreshPlan(ctx, plan_id);
			// The backend stores status 'resolved' (MarkPlanResolved); mirror
			// that exactly so live state matches a reload and every 'resolved'
			// check across the UI (chip styling, scene prompts, demand/war
			// filters) fires.
			const patch = (p: Plan): Plan =>
				p.id === plan_id
					? { ...p, status: 'resolved', result: result as Plan['result'] }
					: p;
			ctx.plans = ctx.plans.map(patch);
			ctx.recordRows = ctx.recordRows.map(row => {
				if (!row.plans.some(p => p.id === plan_id)) return row;
				return { ...row, plans: row.plans.map(patch) };
			});
			// The plan's dice roll is kept visible through the resolving sub-flow
			// so the make/mar option picker can read its outcome. Once the plan
			// leaves 'resolving', that roll is done — clear it so the resolved
			// roll panel doesn't linger. This mirrors GetActiveRollForGame, which
			// stops returning the roll the moment its plan is no longer resolving
			// (so a reload already drops it); without this, the live view kept the
			// panel up until the next reload (most visible on Chronicle Histories,
			// whose long multi-step resolution invites frequent reloads).
			dropRollFetchOfPlan(ctx, plan_id);
			if (ctx.activeRoll && ctx.activeRoll.plan_id === plan_id) {
				ctx.activeRoll = null;
				ctx.activeRollDice = [];
				ctx.activeRollVotes = [];
				ctx.activeRollParticipants = [];
			}
			break;
		}
		case EventTypes.PlanUpdated: {
			// The server's catch-all: this plan's stored state changed in a
			// request, whichever route made it. The payload is only the id, so
			// this refetches through the viewer-aware endpoint (per-viewer masking
			// applies). Latest-wins in refreshPlan makes the overlap with a route's
			// own specific event harmless.
			const { plan_id } = msg.payload as { plan_id: number };
			refreshPlan(ctx, plan_id);
			break;
		}
		case EventTypes.PlanDelayedArrival:
		case EventTypes.LiaisePhaseChanged:
		case EventTypes.LiaiseChoicesRevealed:
		case EventTypes.LiaiseKeepSecretSubmitted:
		case EventTypes.DuelChampionElected:
		case EventTypes.PlanChoiceApplied:
		case EventTypes.DuelStakesSelected:
		case EventTypes.DuelBoutDeclared:
		case EventTypes.DuelBoutResolved:
		case EventTypes.DuelBoutsComplete:
		case EventTypes.FestivityGuestRolled:
		case EventTypes.FestivityGuestChose:
		case EventTypes.FestivityHostChose:
		case EventTypes.FestivityInsistHostMar:
		case EventTypes.FestivityUpdated:
		case EventTypes.FestivityChallengeIssued:
		case EventTypes.FestivityChallengeDeclined:
		case EventTypes.FestivityDuelTriggered:
		case EventTypes.WarDeclared:
		case EventTypes.DemandPrepared:
		case EventTypes.DemandResolved:
		case EventTypes.DemandDraftPick:
		case EventTypes.DemandCounterPending:
		case EventTypes.DemandCounterPlaced:
		case EventTypes.DemandRetargeted:
		case EventTypes.RumorTakeConsentRequested:
		case EventTypes.RumorTakeConsentResolved:
		case EventTypes.DecreeCouncilDeclined:
		case EventTypes.DecreeDebateStarted: {
			const { plan_id } = msg.payload as { plan_id: number };
			refreshPlan(ctx, plan_id);
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.DecreeCouncilJoined: {
			const { plan_id } = msg.payload as { plan_id: number };
			refreshPlan(ctx, plan_id);
			// Joining the council mints an ephemeral 'decree' banked die for the
			// roll. Refresh the unspent-banked list so the +1 die shows up the
			// moment the roll opens, rather than only after a manual page refresh.
			listBankedDice(ctx.gameID).then(d => { ctx.bankedDice = d.dice; }).catch(() => {});
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.DemandLeverageSet: {
			// Adds dice to the target plan's open roll without going through
			// RollLeverageAdded; refresh dice if it's the active roll.
			const { plan_id, roll_id } = msg.payload as { plan_id: number; roll_id: number };
			// Through the roll fetch guard: a bare getRoll here let a late answer
			// overwrite the dice roll.resolved had just installed.
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				fetchRoll(ctx, ctx.activeRoll);
			} else {
				supersedeRollFetch(ctx, roll_id);
			}
			refreshPlan(ctx, plan_id);
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.WarPlayerJoined:
		case EventTypes.WarBattleCostDue:
		case EventTypes.WarBattleCostPaid:
		case EventTypes.WarPlayerSurrendered:
		case EventTypes.WarAssetSeized:
		case EventTypes.WarEntryCompleted:
		case EventTypes.WarPeaceProposed:
		case EventTypes.WarPeaceVote:
		case EventTypes.WarEnded: {
			// War events expose war_id only; refresh all plans so any
			// war-bearing plans pick up updated participants / costs / peace state.
			refreshAllPlans(ctx);
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.LawEnacted: {
			const { law } = msg.payload as { law: Law };
			if (law) {
				const idx = ctx.laws.findIndex(l => l.id === law.id);
				ctx.laws = idx >= 0 ? ctx.laws.map(l => l.id === law.id ? law : l) : [...ctx.laws, law];
			}
			break;
		}
		case EventTypes.LawUpdated: {
			const { law } = msg.payload as { law: Law };
			if (law) ctx.laws = ctx.laws.map(l => l.id === law.id ? law : l);
			break;
		}
		case EventTypes.RumorCreated: {
			const { rumor } = msg.payload as { rumor: Rumor };
			if (rumor) {
				const idx = ctx.rumors.findIndex(r => r.id === rumor.id);
				ctx.rumors = idx >= 0 ? ctx.rumors.map(r => r.id === rumor.id ? rumor : r) : [...ctx.rumors, rumor];
			}
			break;
		}
		case EventTypes.RumorUpdated: {
			const { rumor } = msg.payload as { rumor: Rumor };
			if (rumor) ctx.rumors = ctx.rumors.map(r => r.id === rumor.id ? rumor : r);
			break;
		}
		case EventTypes.SecretCreated: {
			// A new secret exists. Its existence is public, so bump the asset's
			// secret_count for everyone (the open/struck-eye totals stay live
			// without an asset refetch). The text stays gated — refetch visible
			// secrets so anyone with visibility picks up the content.
			const { asset_id } = msg.payload as { asset_id: number };
			ctx.assets = ctx.assets.map(a =>
				a.id === asset_id ? { ...a, secret_count: a.secret_count + 1 } : a
			);
			getVisibleSecrets(ctx.gameID).then(d => { ctx.secrets = d.secrets; }).catch(() => {});
			break;
		}
		case EventTypes.SecretVisibilityGrant: {
			// No new secret — only the viewer's readable set may grow. Refetch
			// visible secrets; secret_count (existence) is unchanged.
			getVisibleSecrets(ctx.gameID).then(d => { ctx.secrets = d.secrets; }).catch(() => {});
			break;
		}
		case EventTypes.RevealSubmitted:
		case EventTypes.RevealComplete: {
			// Reveal widgets subscribe to these directly; no plan ID in payload.
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.EndgameModeSet: {
			const mode = (msg.payload as { mode: string }).mode;
			// The tally also closed the window and completed the deferred row
			// advance; row.advanced / row_state.changed carry the rest.
			if (ctx.game) ctx.game = { ...ctx.game, ending_mode: mode, ending_vote_open: false };
			break;
		}
		case EventTypes.EndgameVoteCast: {
			// EndgameVotePanel owns the ballot and merges this by player_id —
			// the vote is an upsert, so a changed vote must REPLACE that player's
			// entry rather than append a second one (the optimistic-append/WS
			// duplicate trap: two keyed rows for one id freeze reactivity).
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.ShakeUpStepChanged:
		case EventTypes.ShakeUpRolled:
		case EventTypes.ShakeUpSpendOpened:
		case EventTypes.ShakeUpAdjusted:
		case EventTypes.ShakeUpSpendCommitted:
		case EventTypes.ShakeUpSpendAbandoned:
		case EventTypes.ShakeUpPassed:
		case EventTypes.ShakeUpEnded: {
			if (msg.type === EventTypes.ShakeUpStepChanged && ctx.game) {
				const p = msg.payload as { category: string; step: number };
				ctx.game = { ...ctx.game, shake_up_category: p.category, shake_up_step: p.step };
			}
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.PrologueChoiceClaimed:
		case EventTypes.PrologueRankingStepChanged:
		case EventTypes.PrologueTrackRanked:
		case EventTypes.PrologueSetAsidesPlaced:
		case EventTypes.PrologueCommittedHeartsChanged:
		case EventTypes.PrologueDoneChanged:
		case EventTypes.PrologueExtraPeerCreated:
		case EventTypes.PrologueClosingReadyChanged: {
			// Step changes update the game's ranking_step locally so the
			// view re-renders the right sub-flow without a full reload.
			if (msg.type === EventTypes.PrologueRankingStepChanged && ctx.game) {
				const step = (msg.payload as { step: string }).step;
				ctx.game = { ...ctx.game, prologue_ranking_step: step ? (step as Game['prologue_ranking_step']) : null };
			}
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
	}
}

/**
 * Mirror a plan onto the public record's per-row plan list so its chip
 * appears/updates without a full reload. `row_number` is null while a
 * variable-delay plan (Make War / Clandestinely Liaise) awaits its delay
 * reveal; the reveal closing assigns the real row and re-broadcasts the plan,
 * so we upsert into the assigned row and drop any stale copy from another row.
 */
function upsertPlanIntoRecord(ctx: WSContext, plan: Plan) {
	if (plan.row_number == null) return;
	const targetRow = plan.row_number;
	ctx.recordRows = ctx.recordRows.map(row => {
		const without = row.plans.filter(p => p.id !== plan.id);
		if (row.row_number === targetRow) {
			// Insert by row_order so the live record matches resolution order
			// (and a reload, which sorts by row_order). Appending instead would
			// show plans in arrival order — e.g. a Make Demands plan slotted in
			// before its target, or a delay-reveal war landing on a busy row,
			// would display out of sequence until the next full reload.
			return { ...row, plans: [...without, plan].sort((a, b) => a.row_order - b.row_order) };
		}
		// Leave untouched rows referentially stable to avoid needless rerenders.
		return without.length === row.plans.length ? row : { ...row, plans: without };
	});
}

/**
 * Re-fetch the plan tokens so the prep-grid pips stay accurate. Called when a
 * token may have appeared (plan.prepared) or been cleared (rankings.updated).
 * Best-effort: a failed fetch is harmless because the next resync refills them.
 */
function refreshPlanTokens(ctx: WSContext) {
	listPlanTokens(ctx.gameID)
		.then(d => { ctx.planTokens = d.tokens; })
		.catch(() => { /* ignore — resync keeps us in sync */ });
}

// Latest-wins guard for single-plan refetches. A fetch answers with the plan as
// it stood when the server read it, but the response can land after a later WS
// event already patched the plan (or after a newer fetch) — and writing it back
// would roll the plan backwards. A duel that settles itself fires a burst of
// plan events within milliseconds, so a `resolving` snapshot used to overwrite
// the `plan.resolved` patch and leave the finished panel on screen until a
// reload. Every fetch, and every WS patch that carries plan state, takes a new
// sequence number; a response only applies if it still holds the latest one.
const planSeq = new Map<number, number>();
function nextPlanSeq(planID: number): number {
	const seq = (planSeq.get(planID) ?? 0) + 1;
	planSeq.set(planID, seq);
	return seq;
}

function refreshPlan(ctx: WSContext, planID: number) {
	const seq = nextPlanSeq(planID);
	getPlan(planID).then(detail => {
		if (planSeq.get(planID) !== seq) return;
		const next = detail.plan;
		const idx = ctx.plans.findIndex(p => p.id === planID);
		ctx.plans = idx >= 0
			? ctx.plans.map(p => p.id === planID ? next : p)
			: [...ctx.plans, next];
	}).catch(() => {
		// A dropped single-plan refetch would otherwise leave this client stuck
		// on stale plan state — e.g. a resolved take-consent that never advances
		// to "hide source" until a manual page refresh. Fall back to a full
		// plans reload (the same path a manual refresh takes) so we self-heal.
		refreshAllPlans(ctx);
	});
}

// Latest-wins guard for the active roll's fetch — the same shape as planSeq
// above, but the table shows one roll at a time, so one counter per page
// (keyed by its ctx) rather than one per id. One at a time holds because only
// one non-shake-up roll can be open (a partial unique index), and the shake-up
// opens the next roller's roll in the same request that resolves the previous
// one: roll.resolved(N) is followed at once by roll.created(N+1), and the
// single slot plus the sequence number below keep N's late answer out.
//
// roll.created fetches the roll, and the response can land after later events
// about that roll. The roll event handlers only patch a roll that is already
// on screen, so with the fetch still out they dropped the event, and the older
// response then installed the roll as it stood before it: "Dice roll in
// progress…" on a roll that had resolved, or a duel's choices that never came,
// until a reload. A fetch now applies only while it still holds the latest
// sequence number. Anything that would have patched a roll that is still
// loading takes a new number (dropping the stale fetch) and fetches again.
//
// The page's own resync (loadGameState, on load and on every reconnect) reads
// the active roll too, and is a third writer of the same four fields. It used
// to write them unguarded, which went wrong both ways: a snapshot read before
// a roll resolved could land after roll.resolved and put the unresolved roll
// back, and a snapshot that read "no roll" (the plan had finished) could be
// undone by a roll fetch still out for that roll. So the resync draws from this
// counter as well (syncActiveRoll): its answer applies only if no roll event or
// fetch took a newer number while it was out, and an answer that does apply
// takes a number itself, which drops any fetch still out. Resyncs also queue
// behind each other by generation: a reconnect can start one while another is
// still out, and only the newest answer may apply, however they land.
// While the newest resync is out, a roll event treats it like an in-flight
// fetch of the roll on screen: it supersedes it with a fetch, so the resync's
// older answer is not the last word and the roll still ends up complete.
interface RollFetchState {
	/** Every roll fetch, and every resync answer that applies, takes the next. */
	seq: number;
	/** The roll whose fetch is in flight (as announced, or as on screen), or
	 *  null once that fetch has landed or been superseded. */
	inFlight: DiceRoll | null;
	/** Taken by each page resync as it starts, so a later one supersedes an
	 *  earlier one. Separate from seq: a resync that fails must not cost a
	 *  fetch still out its number. */
	resyncGen: number;
	/** The newest resync is still out: no answer, failure or newer number yet. */
	resyncPending: boolean;
}
const rollFetchStates = new WeakMap<WSContext, RollFetchState>();
function rollFetchState(ctx: WSContext): RollFetchState {
	let state = rollFetchStates.get(ctx);
	if (!state) {
		state = { seq: 0, inFlight: null, resyncGen: 0, resyncPending: false };
		rollFetchStates.set(ctx, state);
	}
	return state;
}

/** True while a fetch for this roll is in flight and nothing newer has landed. */
function rollFetchInFlight(ctx: WSContext, rollID: number): boolean {
	return rollFetchState(ctx).inFlight?.id === rollID;
}

/** True while the newest page resync is out and nothing newer has taken a number. */
function rollSyncPending(ctx: WSContext): boolean {
	return rollFetchState(ctx).resyncPending;
}

/**
 * Takes the next number. Any fetch or resync answer that holds an older one is
 * dropped when it lands, and the resync still out has been outdated, so it is
 * no longer pending. Every writer of the counter goes through here.
 */
function takeNumber(state: RollFetchState): number {
	state.inFlight = null;
	state.resyncPending = false;
	return ++state.seq;
}

/**
 * The page's resync of the active roll: awaits `read` and installs the answer
 * (or clears the roll, when the answer has none), unless it has been
 * outdated. The ticket is taken before the await, in the caller's own tick, so
 * an event that arrives while the read is out counts as newer than it:
 *
 * - a roll event or fetch took a number since: that one is newer than this
 *   snapshot, whenever the server read it;
 * - a later resync started: it owns the answer now (a reconnect can start one
 *   while another is out, and landing order says nothing about which read is
 *   newer).
 *
 * An answer that applies takes a number itself, which drops a fetch still out;
 * otherwise a fetch for a roll this answer just cleared would put it straight
 * back. A read that fails costs nothing: it rejects (a tolerated read passes a
 * promise that resolves null, which leaves the screen alone), and a fetch
 * still out is untouched. Shake-up rolls never reach the voting stage, so the
 * endgame resync passes `votes: false` to leave the ballot as it is.
 */
export async function syncActiveRoll(
	ctx: WSContext,
	read: Promise<ActiveRollPayload | null>,
	{ votes = true }: { votes?: boolean } = {},
): Promise<void> {
	const state = rollFetchState(ctx);
	const seq = state.seq;
	const gen = ++state.resyncGen;
	state.resyncPending = true;
	let data: ActiveRollPayload | null;
	try {
		data = await read;
	} finally {
		// This resync is over, answered or failed; unless a later one has taken
		// over the flag.
		if (state.resyncGen === gen) state.resyncPending = false;
	}
	if (!data || state.resyncGen !== gen || state.seq !== seq) return;
	takeNumber(state);
	if (data.roll) {
		ctx.activeRoll = data.roll;
		ctx.activeRollDice = data.dice;
		if (votes) ctx.activeRollVotes = data.votes;
		ctx.activeRollParticipants = data.participants;
	} else {
		// No active roll server-side (none open, and any plan-linked roll's plan
		// has finished resolving): clear any stale one.
		ctx.activeRoll = null;
		ctx.activeRollDice = [];
		if (votes) ctx.activeRollVotes = [];
		ctx.activeRollParticipants = [];
	}
}

/**
 * Fetches `roll` and installs the answer, unless something newer has started
 * since. `roll` is also what to show if the fetch fails and the screen is
 * empty of it: the roll as it was announced.
 */
function fetchRoll(ctx: WSContext, roll: DiceRoll) {
	const state = rollFetchState(ctx);
	const seq = takeNumber(state);
	state.inFlight = roll;
	// True, and clears the in-flight mark, only if this fetch is still the latest.
	const claimLatest = () => {
		if (state.seq !== seq) return false;
		state.inFlight = null;
		return true;
	};
	getRoll(roll.id).then(data => {
		if (!claimLatest()) return;
		ctx.activeRoll = data.roll;
		ctx.activeRollDice = data.dice;
		ctx.activeRollVotes = data.votes;
		ctx.activeRollParticipants = data.participants;
	}).catch(() => {
		if (!claimLatest()) return;
		// A resolution that got here first has already put the roll on screen;
		// the announced roll predates it, so only fill an empty screen.
		if (ctx.activeRoll?.id === roll.id) return;
		ctx.activeRoll = roll;
		ctx.activeRollDice = [];
		ctx.activeRollVotes = [];
		ctx.activeRollParticipants = [];
	});
}

/**
 * An event arrived about a roll. If that roll's fetch is still out, the fetch
 * predates the event and the handler's patch may have found no roll to patch,
 * so supersede the fetch with a new one. A no-op once the fetch has landed.
 *
 * A page resync still out is the same: its snapshot may predate the event, and
 * would overwrite what the handler patches in. When the roll is on screen,
 * supersede the resync too, with a fetch of that roll. A roll that isn't on
 * screen is left to the resync, which may be the only thing that brings it.
 */
function supersedeRollFetch(ctx: WSContext, rollID: number) {
	const state = rollFetchState(ctx);
	if (state.inFlight?.id === rollID) fetchRoll(ctx, state.inFlight);
	else if (state.resyncPending && ctx.activeRoll?.id === rollID) fetchRoll(ctx, ctx.activeRoll);
}

/**
 * The plan finished and its roll is cleared with it (plan.resolved). A fetch
 * for that roll still in flight would put it straight back, so drop it. So
 * would a resync still out, if the roll on screen is this plan's.
 */
function dropRollFetchOfPlan(ctx: WSContext, planID: number) {
	const state = rollFetchState(ctx);
	if (state.inFlight?.plan_id === planID || (state.resyncPending && ctx.activeRoll?.plan_id === planID)) {
		takeNumber(state);
	}
}

function refreshAllPlans(ctx: WSContext) {
	listPlans(ctx.gameID).then(data => { ctx.plans = data.plans; }).catch(() => {});
}
