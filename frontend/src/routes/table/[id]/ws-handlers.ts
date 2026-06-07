// ws-handlers.ts — the WebSocket message dispatcher for the table page,
// extracted from +page.svelte. handleWSMessage mutates the page's reactive
// state through a WSContext whose accessors are backed by the component's
// $state runes, so assignments here stay reactive in the component.

import { EventTypes, type WSMessage } from '$lib/ws';
import { getRoll, listBankedDice, getVisibleSecrets, getPlan, listPlans } from '$lib/api';
import type {
	Game, Player, PresenceMember, ToneTopic, Ranking, Asset, Marginalium,
	Law, Rumor, Secret, ChatPost, SceneEntry, RecordRow, RowState, Scene,
	ScenePeerView, SceneSetupDraft, PreparePlanDraft, DiceRoll, DiceRollDie,
	VoteView, RollParticipant, BankedDie, Plan,
} from '$lib/api';

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
	rankings: Ranking[];
	assets: Asset[];
	laws: Law[];
	rumors: Rumor[];
	secrets: Secret[];
	chatPosts: ChatPost[];
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
	prologueActivePlayerID: number | null;
	typingNames: string[];
	readonly typingMap: Map<number, string>;
	readonly typingTimeouts: Map<number, ReturnType<typeof setTimeout>>;
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
			if (!ctx.chatPosts.find(p => p.id === post.id)) {
				ctx.chatPosts = [...ctx.chatPosts, post];
			}
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
			break;
		}
		case EventTypes.AssetUpdated: {
			const asset = msg.payload.asset as Asset;
			ctx.assets = ctx.assets.map(a => a.id === asset.id ? asset : a);
			break;
		}
		case EventTypes.AssetTaken: {
			const asset = msg.payload.asset as Asset;
			ctx.assets = ctx.assets.map(a => a.id === asset.id ? asset : a);
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
			const { asset_id } = msg.payload as { asset_id: number };
			ctx.assets = ctx.assets.filter(a => a.id !== asset_id);
			break;
		}
		case EventTypes.MarginaliaAdded: {
			const { asset_id, marginalia } = msg.payload as { asset_id: number; marginalia: Marginalium };
			ctx.assets = ctx.assets.map(a => {
				if (a.id !== asset_id) return a;
				const already = a.marginalia.find(m => m.id === marginalia.id);
				return already ? a : { ...a, marginalia: [...a.marginalia, marginalia] };
			});
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
			getRoll(roll.id).then(data => {
				ctx.activeRoll = data.roll;
				ctx.activeRollDice = data.dice;
				ctx.activeRollVotes = data.votes;
				ctx.activeRollParticipants = data.participants;
			}).catch(() => {
				ctx.activeRoll = roll;
				ctx.activeRollDice = [];
				ctx.activeRollVotes = [];
				ctx.activeRollParticipants = [];
			});
			break;
		}
		case EventTypes.RollLeverageAdded: {
			const { roll_id } = msg.payload as { roll_id: number };
			// Re-fetch dice so we have the actual DB row with its ID.
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				getRoll(roll_id).then(data => {
					ctx.activeRollDice = data.dice;
				}).catch(() => {});
				// Banked-die spend or asset leverage may have changed our
				// unspent-banked list; refresh.
				listBankedDice(ctx.gameID).then(d => { ctx.bankedDice = d.dice; }).catch(() => {});
			}
			break;
		}
		case EventTypes.RollStageChanged: {
			const { roll_id, stage } = msg.payload as { roll_id: number; stage: DiceRoll['stage'] };
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRoll = { ...ctx.activeRoll, stage };
			}
			break;
		}
		case EventTypes.RollIntentSet: {
			const { roll_id, player_id, intent } = msg.payload as {
				roll_id: number; player_id: number; intent: 'aid' | 'interfere';
			};
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
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				ctx.activeRoll = { ...ctx.activeRoll, adjusted_difficulty };
				ctx.activeRollVotes = ballot;
			}
			break;
		}
		case EventTypes.RollResolved: {
			const resolvedRoll = msg.payload.roll as DiceRoll;
			const resolvedDice = msg.payload.dice as DiceRollDie[];
			if (ctx.activeRoll && ctx.activeRoll.id === resolvedRoll.id) {
				ctx.activeRoll = resolvedRoll;
				ctx.activeRollDice = resolvedDice;
			}
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
			// Plan was committed; clear the highlight broadcast.
			ctx.preparePlanDraft = null;
			break;
		}
		case EventTypes.PlanResolving: {
			const resolving = msg.payload.plan as Plan;
			ctx.plans = ctx.plans.map(p => p.id === resolving.id ? resolving : p);
			upsertPlanIntoRecord(ctx, resolving);
			break;
		}
		case EventTypes.PlanResolved: {
			const { plan_id, result } = msg.payload as { plan_id: number; result: string };
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
			break;
		}
		case EventTypes.PlanDelayedArrival:
		case EventTypes.LiaisePhaseChanged:
		case EventTypes.LiaiseChoicesRevealed:
		case EventTypes.LiaiseKeepSecretSubmitted:
		case EventTypes.DuelChampionElected:
		case EventTypes.DuelStakesRevealed:
		case EventTypes.DuelStakesSelected:
		case EventTypes.DuelBoutDeclared:
		case EventTypes.DuelBoutResolved:
		case EventTypes.DuelBoutsComplete:
		case EventTypes.FestivityGuestJoined:
		case EventTypes.FestivityGuestRolled:
		case EventTypes.FestivityGuestChose:
		case EventTypes.FestivityHostChose:
		case EventTypes.FestivityInsistHostMar:
		case EventTypes.FestivityPhaseChanged:
		case EventTypes.FestivityChallengeIssued:
		case EventTypes.FestivityChallengeDeclined:
		case EventTypes.FestivityDuelTriggered:
		case EventTypes.WarDeclared:
		case EventTypes.DemandPrepared:
		case EventTypes.DemandResolved:
		case EventTypes.DemandDraftPick:
		case EventTypes.DemandCounterPending:
		case EventTypes.DemandCounterPlaced:
		case EventTypes.DemandRetargeted: {
			const { plan_id } = msg.payload as { plan_id: number };
			refreshPlan(ctx, plan_id);
			window.dispatchEvent(new CustomEvent(`uneasy:${msg.type}`, { detail: msg.payload }));
			break;
		}
		case EventTypes.DemandLeverageSet: {
			// Adds dice to the target plan's open roll without going through
			// RollLeverageAdded; refresh dice if it's the active roll.
			const { plan_id, roll_id } = msg.payload as { plan_id: number; roll_id: number };
			if (ctx.activeRoll && ctx.activeRoll.id === roll_id) {
				getRoll(roll_id).then(data => { ctx.activeRollDice = data.dice; }).catch(() => {});
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
		case EventTypes.SecretCreated:
		case EventTypes.SecretVisibilityGrant: {
			// SecretCreated payload omits text. SecretVisibilityGrant grows
			// the viewer's set when relevant. Either way, refetch.
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
			if (ctx.game) ctx.game = { ...ctx.game, ending_mode: mode };
			break;
		}
		case EventTypes.ShakeUpStepChanged:
		case EventTypes.ShakeUpRolled:
		case EventTypes.ShakeUpSpendOpened:
		case EventTypes.ShakeUpAdjusted:
		case EventTypes.ShakeUpSpendCommitted:
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
		case EventTypes.PrologueExtraPeerCreated: {
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
			return { ...row, plans: [...without, plan] };
		}
		// Leave untouched rows referentially stable to avoid needless rerenders.
		return without.length === row.plans.length ? row : { ...row, plans: without };
	});
}

function refreshPlan(ctx: WSContext, planID: number) {
	getPlan(planID).then(detail => {
		const next = detail.plan;
		const idx = ctx.plans.findIndex(p => p.id === planID);
		ctx.plans = idx >= 0
			? ctx.plans.map(p => p.id === planID ? next : p)
			: [...ctx.plans, next];
	}).catch(() => {});
}

function refreshAllPlans(ctx: WSContext) {
	listPlans(ctx.gameID).then(data => { ctx.plans = data.plans; }).catch(() => {});
}
