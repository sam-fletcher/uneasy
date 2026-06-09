import { apiFetch } from './client';
import type {
	Plan, PlanType, PlanDetail, PlanToken, EligiblePlan, IneligiblePlan, RankingCategory,
	ResolutionData, Choice, KeptSecret, DiceRoll, DiceRollDie, Asset,
} from './types';

export function listPlans(gameID: string | number): Promise<{ plans: Plan[] }> {
	return apiFetch(`/tables/${gameID}/plans`);
}

/**
 * Plan tokens placed in the game — one per (plan_type, player). Drives the
 * prep-grid pips that show which players hold a token on each plan's shield.
 * Visible to every viewer, so the colour info is available in read-only mode.
 */
export function listPlanTokens(gameID: string | number): Promise<{ tokens: PlanToken[] }> {
	return apiFetch(`/tables/${gameID}/plan-tokens`);
}

/** Check which Phase 2 plans the current player is eligible to prepare. */
export function getPlanEligibility(gameID: string | number): Promise<{
	eligible: EligiblePlan[];
	ineligible: IneligiblePlan[];
}> {
	return apiFetch(`/tables/${gameID}/plan-eligibility`);
}

/** Prepare a plan (focus player only). */
export function preparePlan(
	gameID: string | number,
	params: {
		plan_type: PlanType;
		target_player_id?: number | null;
		target_asset_id?: number | null;
		target_plan_id?: number | null;
		peer_count?: number;
		enemy_player_ids?: number[];
		duel_type?: 'arms' | 'wits';
		/** Clandestinely Liaise: the preparer's & partner's meeting peers. */
		preparer_peer_id?: number | null;
		partner_peer_id?: number | null;
		preparation_notes?: string | null;
	}
): Promise<{ plan: Plan }> {
	return apiFetch(`/tables/${gameID}/prepare-plan`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

/** Get full plan details including computed difficulty and resolution state. */
export function getPlan(planID: number): Promise<PlanDetail> {
	return apiFetch(`/plans/${planID}`);
}

/**
 * Exchange Courtiers fair trade step.
 *
 * - Target player offers a peer:  { action: 'offer', offered_asset_id: X }
 * - Preparer accepts the offer:   { action: 'accept' }
 * - Preparer declines (roll):     { action: 'decline' }
 */
export function fairTrade(
	planID: number,
	body: { action: 'offer'; offered_asset_id: number }
		| { action: 'accept' }
		| { action: 'decline' }
): Promise<{ plan_id: number; roll?: DiceRoll; result?: string }> {
	return apiFetch(`/plans/${planID}/fair-trade`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

/** Record make/mar option choices after the dice roll resolves. */
export function makeChoice(
	planID: number,
	result: 'make' | 'mar',
	choices: string[]
): Promise<{ plan_id: number; result: string; choices: string[] }> {
	return apiFetch(`/plans/${planID}/make-choice`, {
		method: 'POST',
		body: JSON.stringify({ result, choices })
	});
}

/** Mark the plan as resolved (after choices applied). */
export function completePlan(planID: number): Promise<{
	plan_id: number;
	result: 'make' | 'mar';
}> {
	return apiFetch(`/plans/${planID}/complete`, { method: 'POST' });
}

/**
 * Exchange Courtiers — messy break.
 * Called by the TARGET player after a make + "messy" outcome to tear a
 * marginalia on any asset. Must be called before CompletePlan is allowed.
 *
 * @param marginaliaID  The DB id of the marginalia to tear.
 */
export function messyBreak(planID: number, marginaliaID: number): Promise<{
	plan_id: number;
	marginalia_id: number;
	asset_id: number;
}> {
	return apiFetch(`/plans/${planID}/messy-break`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID }),
	});
}

/**
 * Exchange Courtiers mar — the target claims one of the preparer's peers
 * (riposte/forfeit). Called by the target once per required claim.
 */
export function ecClaimPeer(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/claim-peer`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/**
 * Exchange Courtiers mar — riposte: the preparer optionally breaks one of
 * their own peers before the target claims it.
 */
export function ecRiposteBreak(planID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/riposte-break`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID }),
	});
}

/**
 * Make Introductions — create a single peer during the pre-roll naming step.
 * Called once per peer until peer_count peers exist; then call
 * finalizeIntroductionsPeers to create the dice roll.
 */
export function createIntroductionsPeer(
	planID: number,
	params: { name: string; marginalia?: string[] }
): Promise<{ plan_id: number; asset: Asset; created_peer_ids: number[] }> {
	return apiFetch(`/plans/${planID}/create-peer`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

/**
 * Make Introductions — finalize the pre-roll naming step. Creates the
 * dice roll; resolution proceeds normally from there. Returns the roll.
 */
export function finalizeIntroductionsPeers(planID: number): Promise<{
	plan_id: number;
	roll: DiceRoll;
}> {
	return apiFetch(`/plans/${planID}/finalize-peers`, { method: 'POST' });
}

/**
 * Make Introductions mar — resolve one introduced peer. outcome is
 * other_retinue | broken_arrival | delayed | broken_journey. other_retinue and
 * broken_arrival need targetPlayerID; broken_journey needs text.
 */
export function introductionsMar(
	planID: number,
	params: { peer_asset_id: number; outcome: string; target_player_id?: number; text?: string }
): Promise<{ plan_id: number; resolved: number; peer_count: number }> {
	return apiFetch(`/plans/${planID}/introductions-mar`, {
		method: 'POST',
		body: JSON.stringify(params),
	});
}

/**
 * Make Introductions mar — the assigned author writes a broken-arrival peer's
 * marginalia.
 */
export function introductionsMarginalia(
	planID: number,
	peerAssetID: number,
	text: string
): Promise<{ plan_id: number; peer_asset_id: number; marginalia_id: number }> {
	return apiFetch(`/plans/${planID}/introductions-marginalia`, {
		method: 'POST',
		body: JSON.stringify({ peer_asset_id: peerAssetID, text }),
	});
}

// ── Plans (Phase 3 — Tier 1) ─────────────────────────────────────────────────
//
// Thin wrappers, one per endpoint in PHASE3_SPEC.md §New Endpoints — Tier 1.
// Response shapes are loose (plan_id + occasional extras) because backends
// mostly echo the plan and rely on WS for detailed state.

type PlanEcho = { plan_id: number } & Record<string, unknown>;

/** Seek Answers — break a marginalia on a target resource asset. */
export function breakResource(planID: number, assetID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-resource`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID, marginalia_id: marginaliaID }),
	});
}

/** Seek Answers — grant the preparer visibility into a secret-bearing asset. */
export function revealSecret(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/reveal-secret`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/**
 * Spread Rumors — break a marginalia.
 *
 * On make (preparer) the marginalia must belong to the plan's target asset
 * and `assetID` may be omitted. On mar (target-asset owner) `assetID` is
 * required and must be one of the preparer's assets.
 */
export function breakTarget(planID: number, marginaliaID: number, assetID?: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-target`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID, asset_id: assetID ?? null }),
	});
}

/**
 * Spread Rumors — consent-based asset transfer.
 *
 * On make (preparer) the server transfers plan.target_asset_id; omit
 * `assetID`. On mar (target-asset owner) `assetID` must be one of the
 * preparer's assets, which is then transferred to the target-asset owner.
 * (Named `takeRumorAsset` to avoid collision with the asset-level `takeAsset`.)
 */
export function takeRumorAsset(planID: number, consent: boolean, assetID?: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/take-asset`, {
		method: 'POST',
		body: JSON.stringify({ consent, asset_id: assetID ?? null }),
	});
}

/** Spread Rumors — hide the rumor source behind a secret-bearing asset. */
export function hideSource(planID: number, secretAssetID: number, secretText: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/hide-source`, {
		method: 'POST',
		body: JSON.stringify({ secret_asset_id: secretAssetID, secret_text: secretText }),
	});
}

/**
 * Spread Propaganda mar (a) — the preparer hands one of their own peers to
 * another player. Called by the preparer after choosing the "give_peer" mar
 * option; completion is blocked until it succeeds.
 */
export function spGivePeer(planID: number, peerAssetID: number, toPlayerID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/give-peer`, {
		method: 'POST',
		body: JSON.stringify({ peer_asset_id: peerAssetID, to_player_id: toPlayerID }),
	});
}

/**
 * Spread Propaganda mar (c) — "break yourself." The preparer tears one
 * marginalia from one of their own assets. Completion is blocked until it
 * succeeds.
 */
export function spBreakSelf(planID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-self`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID }),
	});
}

/** Chronicle Histories — add an artifact to the invoked list (pre-roll or via make option). */
export function invokeArtifact(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/invoke-artifact`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/** Chronicle Histories — preparer closes the pre-roll invoke phase and casts the dice. */
export function castChronicleRoll(planID: number): Promise<{ plan_id: number; roll?: DiceRoll }> {
	return apiFetch(`/plans/${planID}/cast-roll`, { method: 'POST' });
}

/** Chronicle Histories — break a marginalia on an invoked artifact. */
export function breakArtifact(planID: number, assetID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-artifact`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID, marginalia_id: marginaliaID }),
	});
}

/**
 * Chronicle Histories — a player submits their one mar choice.
 * `assetID` is required for break_artifact / invoke_another; `marginaliaID`
 * is additionally required for break_artifact (the break is applied atomically
 * server-side, with auto-destroy on the artifact's last marginalium).
 */
export function marChoice(
	planID: number,
	choice: string,
	assetID?: number,
	marginaliaID?: number,
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/mar-choice`, {
		method: 'POST',
		body: JSON.stringify({
			choice,
			asset_id: assetID ?? null,
			marginalia_id: marginaliaID ?? null,
		}),
	});
}

// ── Plans (Phase 3 — Tier 2) ─────────────────────────────────────────────────

/** Propose Decree — join the council by leveraging one or more assets. */
export function joinCouncil(planID: number, assetIDs: number[]): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-council`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/** Propose Decree — the current signatory closes the council and triggers the roll. */
export function callRoll(planID: number): Promise<{ plan_id: number; roll?: DiceRoll }> {
	return apiFetch(`/plans/${planID}/call-roll`, { method: 'POST' });
}

/**
 * Name the asset a plan created during its make step. Preparer-gated; optional
 * — the asset keeps its placeholder name until named. The route differs per
 * plan ('name-resource' for Propose Decree, 'name-artifact' for Spread
 * Propaganda) because all plans' extra routes share one mount, so route names
 * must be globally unique.
 */
export function namePlanAsset(
	planID: number,
	route: 'name-resource' | 'name-artifact',
	name: string
): Promise<{ plan_id: number; asset: Asset }> {
	return apiFetch(`/plans/${planID}/${route}`, {
		method: 'POST',
		body: JSON.stringify({ name }),
	});
}

/**
 * Propose Decree — on a mar, the current council amender rewrites the full law
 * body (lowest power first). `text` replaces the law's text; the next amender
 * works from that output.
 */
export function amendDecree(planID: number, text: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/amend-decree`, {
		method: 'POST',
		body: JSON.stringify({ text }),
	});
}

/**
 * Propose Decree — the signatory places their addendum: an "and"/"but"
 * connector plus optional free text. This is a required blocking step; pass a
 * blank addendum (and omit the connector) to place an empty rider. The
 * connector is required only when addendum text is provided.
 */
export function setAddendum(
	planID: number,
	addendum: string,
	connector?: 'and' | 'but',
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/set-addendum`, {
		method: 'POST',
		body: JSON.stringify({ addendum, connector: connector ?? '' }),
	});
}

/** Clandestinely Liaise — focus player advances to the next phase. */
export function advanceLiaise(planID: number): Promise<{ plan_id: number; phase: string }> {
	return apiFetch(`/plans/${planID}/advance-liaise`, { method: 'POST' });
}

/** Clandestinely Liaise (phase 2) — commit a secret-bearing asset. */
export function keepSecret(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/keep-secret`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/**
 * Clandestinely Liaise (phase 3) — submit a "Things We Share" choice. Every
 * option targets the PARTNER's asset (`target_asset_id`). `break_peer` also
 * requires `target_marginalia_id` (the breaker picks which marginalia to tear;
 * the break applies atomically server-side with auto-destroy on the last one).
 */
export function shareChoice(
	planID: number,
	body: {
		choice: string;
		target_asset_id?: number | null;
		target_marginalia_id?: number | null;
		update_text?: string;
	}
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/share-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Clandestinely Liaise (phase 4) — submit re-delay die face (0 = cancel). */
export function redelayReveal(planID: number, face: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/redelay-reveal`, {
		method: 'POST',
		body: JSON.stringify({ face }),
	});
}

export interface BankedDie {
	id: number;
	game_id: number;
	player_id: number;
	source: string;
	created_at: string;
	used_at: string | null;
	used_roll_id: number | null;
}

/** List the calling player's unspent banked dice in this game. */
export function listBankedDice(gameID: string | number): Promise<{ dice: BankedDie[] }> {
	return apiFetch(`/tables/${gameID}/banked-dice`);
}

/** Spend a banked die on this roll. Direction follows the player's intent. */
export function useBankedDie(rollID: number, bankedDieID: number): Promise<{ die: DiceRollDie; banked_die_id: number }> {
	return apiFetch(`/rolls/${rollID}/use-banked-die`, {
		method: 'POST',
		body: JSON.stringify({ banked_die_id: bankedDieID }),
	});
}

// ── Simultaneous reveals (shared) ────────────────────────────────────────────

export type RevealType = 'make_war_delay' | 'liaise_delay' | 'liaise_redelay';

export interface SimultaneousRevealEntry {
	player_id: number;
	/** Null until the reveal completes; then populated for every participant. */
	face: number | null;
	revealed_at: string | null;
}

export interface SimultaneousReveal {
	id: number;
	game_id: number;
	plan_id: number | null;
	reveal_type: RevealType;
	is_complete: boolean;
	result_delay: number | null;
	entries: SimultaneousRevealEntry[];
}

/** Submit a die face for a simultaneous reveal. */
export function submitReveal(revealID: number, face: number): Promise<{ reveal_id: number; is_complete: boolean }> {
	return apiFetch(`/reveals/${revealID}/submit`, {
		method: 'POST',
		body: JSON.stringify({ face }),
	});
}

/** Fetch reveal state. Faces stay hidden until every participant has submitted. */
export function getReveal(revealID: number): Promise<SimultaneousReveal> {
	return apiFetch(`/reveals/${revealID}`);
}

// ── Plans (Phase 3 — Tier 3) ─────────────────────────────────────────────────

// Propose Duel.

/** A staked asset as seen by a caller. `hidden_die` is null for stakes the
 * caller doesn't own unless the stake has already been resolved in a bout. */
export interface DuelStake {
	id: number;
	plan_id: number;
	player_id: number;
	asset_id: number;
	is_resolved: boolean;
	is_winner: boolean | null;
	hidden_die: number | null;
}

export interface DuelBout {
	id: number;
	plan_id: number;
	bout_number: number;
	declarer_id: number;
	declarer_stake_id: number;
	responder_id: number;
	responder_stake_id: number | null;
	declaration: 'high' | 'low' | null;
	declarer_die: number | null;
	responder_die: number | null;
	winner_id: number | null;
	is_match: boolean;
	created_at: string;
	resolved_at: string | null;
}

export interface DuelStateResponse {
	plan_id: number;
	stakes: DuelStake[];
	bouts: DuelBout[];
}

/** Propose Duel — current stakes + bout history visible to the caller. */
export function getDuelState(planID: number): Promise<DuelStateResponse> {
	return apiFetch(`/plans/${planID}/duel-state`);
}

/** Propose Duel — elect a peer as champion. Pass null to fight yourself. */
export function electChampion(planID: number, assetID: number | null): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/elect-champion`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/** Propose Duel — simultaneously reveal stake count (1..1+status). */
export function stakeReveal(planID: number, count: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/stake-reveal`, {
		method: 'POST',
		body: JSON.stringify({ count }),
	});
}

/** Propose Duel — select the specific assets to stake. Response includes the
 * caller's newly created stakes (with hidden dice). */
export function selectStakes(
	planID: number,
	assetIDs: number[],
): Promise<{ plan_id: number; staked: number; stakes: DuelStake[] }> {
	return apiFetch(`/plans/${planID}/select-stakes`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/** Propose Duel — declarer picks a stake and declares high or low. */
export function boutDeclare(planID: number, stakeID: number, declaration: 'high' | 'low'): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/bout-declare`, {
		method: 'POST',
		body: JSON.stringify({ stake_id: stakeID, declaration }),
	});
}

/** Propose Duel — responder picks their stake; server resolves the bout. */
export function boutRespond(planID: number, stakeID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/bout-respond`, {
		method: 'POST',
		body: JSON.stringify({ stake_id: stakeID }),
	});
}

// Host Festivity.

/** Host Festivity — join as a guest. */
export function joinFestivity(planID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-festivity`, { method: 'POST' });
}

/** Host Festivity — guest commits to rolling or opts out. */
export function guestRoll(planID: number, action: 'roll' | 'opt_out'): Promise<{ plan_id: number; roll?: DiceRoll }> {
	return apiFetch(`/plans/${planID}/guest-roll`, {
		method: 'POST',
		body: JSON.stringify({ action }),
	});
}

/** Host Festivity — guest submits their make/mar choice after rolling. */
export function guestChoice(
	planID: number,
	body: { choice: string } & Record<string, unknown>
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/guest-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Host Festivity — host submits a make choice on behalf of a mar/opt-out guest. */
export function hostChoice(
	planID: number,
	body: { target_player_id: number; choice: string } & Record<string, unknown>
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/host-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Host Festivity — challenge a guest to a duel; spawns a Propose Duel plan. */
export function challengeDuel(
	planID: number,
	targetPlayerID: number,
	notes?: string,
): Promise<{ plan_id: number; duel_plan_id?: number; challenger_id?: number; target_id?: number; must_accept?: boolean }> {
	return apiFetch(`/plans/${planID}/challenge-duel`, {
		method: 'POST',
		body: JSON.stringify({ target_player_id: targetPlayerID, preparation_notes: notes ?? '' }),
	});
}

/** Host Festivity — challenged guest accepts or declines the duel. */
export function respondChallenge(planID: number, accept: boolean): Promise<{ plan_id: number; accepted: boolean; duel_plan_id?: number }> {
	return apiFetch(`/plans/${planID}/respond-challenge`, {
		method: 'POST',
		body: JSON.stringify({ accept }),
	});
}

/** Host Festivity — guest with an unspent IOU forces the host to take a mar option. */
export function insistHostMar(
	planID: number,
	body: { mar_option: string; asset_id?: number; rumor_text?: string },
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/insist-host-mar`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

// Make War.

export interface WarParticipantInfo {
	player_id: number;
	side: 1 | 2;
	joined_at_row: number;
	surrendered_at_row: number | null;
	entry_payment_complete: boolean;
}

export interface WarBattleCostInfo {
	id: number;
	row_number: number;
	payer_id: number;
	opponent_id: number;
	choice: string;
	asset_id_1: number | null;
	asset_id_2: number | null;
	surrendered: boolean;
	is_entry: boolean;
}

export interface WarOutstandingCost {
	payer_id: number;
	opponent_id: number;
}

export interface WarPeaceVote { player_id: number; accepted: boolean }

export interface WarPeaceProposalInfo {
	id: number;
	proposer_id: number;
	terms: string;
	status: 'open' | 'accepted' | 'rejected';
	votes: WarPeaceVote[];
	awaiting: number[];
}

export interface WarSurrenderClaimInfo {
	id: number;
	surrendered_id: number;
	claimant_id: number;
}

export interface WarStateResponse {
	war_id: number;
	origin_plan_id: number;
	status: 'active' | 'ended';
	started_at_row: number;
	ended_at_row: number | null;
	end_reason: string | null;
	current_row: number;
	participants: WarParticipantInfo[];
	battle_costs: WarBattleCostInfo[];
	/** Reverse-power-ordered (payer, opponent) pairs still owed *this* row. */
	outstanding_costs: WarOutstandingCost[];
	open_proposal: WarPeaceProposalInfo | null;
	open_claims: WarSurrenderClaimInfo[];
}

/** Make War — full war state for the panel (participants, costs, peace, claims). */
export function getWarState(planID: number): Promise<WarStateResponse> {
	return apiFetch(`/plans/${planID}/war-state`);
}

/** List all active wars in a game. Used by the turn indicator to flag rows
 * blocked on outstanding cost-of-battle payments. */
export function listWars(gameID: number | string): Promise<{ wars: WarStateResponse[] }> {
	return apiFetch(`/tables/${gameID}/wars`);
}

/** Make War — uninvited player joins a side. Free during the delay reveal;
 * after the war is active, the joiner owes a cost-of-battle entry to every
 * existing opposing participant before counting as fully joined. */
export function joinWar(planID: number, side: 1 | 2): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-war`, {
		method: 'POST',
		body: JSON.stringify({ side }),
	});
}

/** Make War — pay this row's cost of battle to one opponent.
 * `surrender:true` after the payment marks the payer surrendered. */
export function payBattleCost(
	planID: number,
	body: {
		opponent_id: number;
		choice: 'break_asset' | 'leverage_two';
		marginalia_id?: number;
		asset_id_1?: number;
		asset_id_2?: number;
		surrender?: boolean;
	}
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/pay-battle-cost`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Make War — late joiner pays cost of battle entry to one existing opponent. */
export function payWarEntry(
	planID: number,
	body: {
		opponent_id: number;
		choice: 'break_asset' | 'leverage_two';
		marginalia_id?: number;
		asset_id_1?: number;
		asset_id_2?: number;
	}
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/pay-war-entry`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Make War — claim one asset from a surrendered opposing participant. */
export function takeSurrenderAsset(
	planID: number,
	surrenderedID: number,
	assetID: number,
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/take-surrender-asset`, {
		method: 'POST',
		body: JSON.stringify({ surrendered_id: surrenderedID, asset_id: assetID }),
	});
}

/** Make War — propose peace terms. Only the active payer (whose turn it is to
 * pay cost) may propose; the proposer auto-votes accept. If the vote is not
 * unanimous they must still pay using break_asset/leverage_two. */
export function proposePeace(planID: number, terms: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/propose-peace`, {
		method: 'POST',
		body: JSON.stringify({ terms }),
	});
}

/** Make War — vote on an open peace proposal. A single 'reject' closes it;
 * unanimous accepts end the war. */
export function votePeace(
	planID: number,
	proposalID: number,
	accepted: boolean,
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/vote-peace`, {
		method: 'POST',
		body: JSON.stringify({ proposal_id: proposalID, accepted }),
	});
}

// Make Demands.

/** Make Demands — pick a draft option on your turn in the alternating draft. */
export function draftChoice(planID: number, option: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/draft-choice`, {
		method: 'POST',
		body: JSON.stringify({ option }),
	});
}

/**
 * Make Demands — target places a counter Make Demands.
 * Pass `targetPlanID = null` to attach the counter to the target's next prepared plan.
 */
export function counterDemand(planID: number, targetPlanID: number | null): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/counter-demand`, {
		method: 'POST',
		body: JSON.stringify({ target_plan_id: targetPlanID }),
	});
}

/**
 * Make Demands — control_leverage winner sets leverage on the *target plan*.
 * Mounted on the target plan, NOT the demand plan. Adds dice from the target
 * preparer's own assets onto the target plan's open roll.
 */
export function demandLeverage(targetPlanID: number, assetIDs: number[]): Promise<{
	plan_id: number;
	roll_id: number;
	asset_ids: number[];
}> {
	return apiFetch(`/plans/${targetPlanID}/demand-leverage`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/**
 * Make Demands — keep_or_change_target winner re-aims the *target plan*.
 * Mounted on the target plan, NOT the demand plan. Re-validates against the
 * target plan type's preparation rules before persisting.
 */
export function demandRetarget(
	targetPlanID: number,
	params: { target_player_id?: number | null; target_asset_id?: number | null },
): Promise<{
	plan_id: number;
	target_player_id: number | null;
	target_asset_id: number | null;
}> {
	return apiFetch(`/plans/${targetPlanID}/demand-retarget`, {
		method: 'POST',
		body: JSON.stringify(params),
	});
}
