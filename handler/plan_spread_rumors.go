package handler

// handler/plan_spread_rumors.go — Spread Rumors plan handler (Phase 3b).
//
// Spread Rumors (esteem, delay 4): The preparer starts a rumor about an asset.
// Difficulty depends on the target:
//   - Target is a main character: 6 - target's esteem rank (target's esteem status)
//   - Any other asset:            preparer's rank on the esteem track
//
// Preparing: target_asset_id required; preparation_notes holds the rumor text —
// a DRAFT (principle 2). It may be kept secret for now (text in a Secret).
//
// Pre-roll: OnResolve parks the plan with no roll. The preparer states the rumor
// and how it is spreading via sr-cast-roll, which posts that narration publicly,
// records the stated rumor, and creates the roll. Stating it is what makes a
// secret rumor public — on a make or a mar (book p.59: "keep it secret for now").
//
// Make: server creates a rumors row from the STATED rumor. Then choose N options
// equal to dice result (repeatable):
//   - "break_target"  → tear a marginalia on the target asset
//   - "leverage_target" → leverage the target asset
//   - "take_asset"    → transfer any of the victim's assets (requires the
//                       victim's consent — see request/respond-take-consent)
//   - "hide_source"   → the rumor row is created with no source; write a secret
//                       on own asset (hides the spreader's CHARACTER only)
//   - "reveal_source" → gloating: "Reveal yourself as the source of the rumor.
//                       Relish in its glory." Posts a log line; changes nothing
//                       (attribution already names the preparer unless (d)).
//
// Mar: the target-asset owner (the "mar actor") writes the counter-rumor — the
// rumor that spreads about the preparer instead (sr-counter-rumor) — then chooses
// options equal to (difficulty - result), all aimed at the preparer's MAIN
// CHARACTER as it stands when the options are applied (rulings Q1–Q2):
//   - break_target    → the mar actor tears a marginalia on that MC
//   - leverage_target → leverages that MC
//   - take_asset      → any of the preparer's assets, consent-gated
//   - hide_source     → hides the MAR ACTOR's character as the source
//   - reveal_source   → the mar actor gloats
// The rumor row is the counter-rumor (text = the actor's, target = that MC,
// source = the actor unless (d)); the preparer's failed rumor never reaches the
// Rumors sheet.
//
// Extra routes (preparer on a make, target-asset owner on a mar):
//   POST /api/plans/:planId/sr-cast-roll          {"rumor": "...", "how": "..."}
//   POST /api/plans/:planId/sr-counter-rumor      {"text": "..."}               (mar actor)
//   POST /api/plans/:planId/break-target          {"marginalia_id": M}
//   POST /api/plans/:planId/hide-source           {"secret_asset_id": N, "secret_text"?: "..."}
//   POST /api/plans/:planId/request-take-consent  {"choices": [...], "result": "...", "take_asset_ids": [A, …]}
//   POST /api/plans/:planId/respond-take-consent  {"agree": true|false}
//
// "take asset" is consent-gated: unlike break/leverage (which only touch the
// rumor's subject — the target asset on a make, the preparer's MC on a mar), a
// take can claim ANY of the victim's assets — the target-asset owner on make, the
// preparer on mar — and the aggressor may take several (one per "take_asset"
// pick). The aggressor names the specific assets up front via
// request-take-consent; nothing is committed until the victim agrees via
// respond-take-consent. On agree the choices are applied and the assets transfer;
// on disagree nothing happens and the aggressor returns to the option picker with
// "take asset" disabled. See srRequestTakeConsentHandler.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadRumors, srHandler{})
}

type srHandler struct{}

// Make/mar option keys for Spread Rumors. Defined as constants because several
// are referenced across the handler, its sub-flow routes, and CanComplete.
const (
	srOptBreakTarget = "break_target"
	srOptHideSource  = "hide_source"
)

func (srHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 4}
}

func (srHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetAssetID == nil {
		return nil, "spread_rumors requires target_asset_id"
	}
	// The subject must be a real, living asset of THIS game: the id arrives raw in
	// the request, and everything downstream (difficulty, the mar actor, the rumor
	// row) trusts it.
	asset, err := v.Q.GetAssetByID(ctx, *v.TargetAssetID)
	if err != nil || (v.Game != nil && asset.GameID != v.Game.ID) {
		return nil, "spread_rumors target asset not found in this game"
	}
	if asset.IsDestroyed {
		return nil, "spread_rumors cannot target a destroyed asset"
	}
	if v.Notes == "" && !v.KeptSecret {
		return nil, "spread_rumors requires preparation_notes with the rumor text"
	}
	return nil, ""
}

func (srHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetAssetID == nil {
		return 0, errors.New("spread_rumors plan has no target asset")
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		return 0, fmt.Errorf("could not load target asset: %w", err)
	}

	if asset.IsMainCharacter {
		// Difficulty = target player's esteem STATUS = 6 - rank.
		if asset.OwnerID == 0 {
			return 0, errors.New("main character asset has no owner")
		}
		targetRank, errRank := playerRankInCategory(ctx, q, plan.GameID, asset.OwnerID, model.CategoryEsteem)
		if errRank != nil {
			return 0, fmt.Errorf("could not determine target esteem rank: %w", errRank)
		}
		return gamepkg.SpreadRumorsDifficulty(targetRank, true), nil
	}

	// Difficulty = preparer's esteem rank.
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer esteem rank: %w", err)
	}
	return gamepkg.SpreadRumorsDifficulty(preparerRank, false), nil
}

// OnResolve opens the pre-roll narration step and returns nil — no roll yet —
// unless the rumor's subject is gone, in which case the plan pauses for its
// preparer to re-aim it first (plan_reaim.go). The check comes before the step
// because the difficulty is read off the target asset (at cast time, now, so it is
// read as late as it can be).
//
// The plan then sits in 'resolving' until the preparer states the rumor and how
// it is spreading; sr-cast-roll posts that for the table to read, then creates the
// roll (plan_preroll.go). No log entry here: the standard plan.resolving post
// marks the start and cast-roll posts the narration itself.
func (srHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	if held, err := reaimHoldAtResolve(ctx, deps, plan, srHandler{}); held || err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	resData.EnsureSpreadRumors().PreRollDone = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open pre-roll step: %w", err)
	}
	return nil, nil
}

// CheckChoice gates make-choice behind the steps that must come first. Before the
// roll — or while it is still open — there is nothing to choose; on a mar the options belong to the mar actor —
// never the preparer, whose rumor just failed — and wait for the counter-rumor the
// actor writes first (book p.59: "The target player describes the rumor that
// spreads about you instead").
func (srHandler) CheckChoice(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	player *dbgen.Player,
	resData *ResolutionData,
	roll *dbgen.DiceRoll,
	result string,
) error {
	if roll == nil {
		return errors.New("state the rumor and cast the dice before choosing options")
	}
	if roll.Outcome == nil {
		return errDiceNotSettled
	}
	if result != marOutcome {
		return nil
	}
	if isActor, err := srIsMarActor(ctx, q, plan, player.ID); err != nil {
		return err
	} else if !isActor {
		return httpErr(http.StatusForbidden, "only the target asset's owner chooses on a mar")
	}
	if sr := resData.SpreadRumors; sr == nil || sr.CounterRumorText == "" {
		return errors.New("write the counter-rumor before choosing options")
	}
	return nil
}

// srIsMarActor reports whether playerID is the mar actor: the live owner of the
// plan's target asset ("the target player"). Read live on purpose — see StaleAim.
func srIsMarActor(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, playerID int64) (bool, error) {
	if plan.TargetAssetID == nil {
		return false, nil
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		return false, err
	}
	return asset.OwnerID == playerID, nil
}

// srPreparerMainCharacter returns the preparer's living main character — whichever
// asset holds the flag right now — or nil when they have none (the replacement
// gate is mid-flight).
func srPreparerMainCharacter(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (*dbgen.Asset, error) {
	mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: plan.GameID, OwnerID: plan.PreparerID,
	})
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return &mc, nil
}

// srRumorSubject is the asset the rumor is about, as this branch sees it: the
// plan's target on a make, the preparer's main character (pinned when the options
// were applied) on a mar. nil when a mar finds no main character.
func srRumorSubject(plan *dbgen.Plan, sr *gamepkg.SpreadRumorsResolutionData, result string) *int64 {
	if result == marOutcome {
		return sr.CounterTargetAssetID
	}
	return plan.TargetAssetID
}

// srRumorText resolves the text of the rumor row this branch publishes: the
// STATED rumor on a make, the counter-rumor on a mar. The preparer's failed rumor
// is never a candidate on a mar, and the prep-time note and any kept Secret are
// never read back — only what a player stated at the table is published. There is
// no placeholder: a branch with no text is an error, not a blank card on the
// public record.
func srRumorText(sr *gamepkg.SpreadRumorsResolutionData, result string) (string, error) {
	if result == marOutcome {
		if sr.CounterRumorText == "" {
			return "", errors.New("the counter-rumor has not been written")
		}
		return sr.CounterRumorText, nil
	}
	if sr.RumorText == "" {
		return "", errors.New("the rumor has not been stated")
	}
	return sr.RumorText, nil
}

// ApplyChoice creates the rumors row (both branches) and applies the inline
// effects — leverage_target, reveal_source, and the hide_source flag. "break_target",
// "take_asset", and the hide-source secret go through extra routes.
//
// Every option acts on the rumor's subject for its branch (srRumorSubject) and
// every option leaves a trace: nothing here burns a pick silently.
func (srHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}
	sr := resData.EnsureSpreadRumors()
	onMar := result == marOutcome

	// A mar's subject is the preparer's main character AS IT STANDS NOW. Pin it
	// before the row is made so the row, break and leverage all agree.
	if onMar {
		mc, mcErr := srPreparerMainCharacter(ctx, deps.Q, plan)
		if mcErr != nil {
			return fmt.Errorf("could not find the preparer's main character: %w", mcErr)
		}
		sr.CounterTargetAssetID = nil
		if mc != nil {
			sr.CounterTargetAssetID = &mc.ID
		}
	}
	subject := srRumorSubject(plan, sr, result)

	rumorText, err := srRumorText(sr, result)
	if err != nil {
		return err
	}
	existingRumors, _ := deps.Q.ListRumors(ctx, plan.GameID)
	displayOrder := int16(len(existingRumors))

	// Source attribution drives both the Rumors panel ("Spread by: …") and the
	// chat-log message. The source is whoever authored the rumor — the preparer on
	// a make, the mar actor on a mar — unless they hide themselves, in which case
	// the row carries no source (the hide-source route later records the secret).
	// Hiding hides the CHARACTER only; the players always know (principle 5), and
	// the hide-source log line says so.
	hideSource := slices.Contains(choices, srOptHideSource)
	author := plan.PreparerID
	if onMar {
		author = sr.CounterRumorAuthorID
	}
	var sourcePlayerID *int64
	if !hideSource {
		sourcePlayerID = &author
	}

	rumor, err := deps.Q.CreateRumor(ctx, dbgen.CreateRumorParams{
		GameID:         game.ID,
		Text:           rumorText,
		TargetAssetID:  subject,
		OriginPlanID:   &plan.ID,
		SourcePlayerID: sourcePlayerID,
		DisplayOrder:   displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create rumor: %w", err)
	}
	sr.RumorID = &rumor.ID
	if hideSource {
		sr.SourceHidden = true
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	// Chat-log entry. EmitRumorCreated names the source when one is set and
	// stays anonymous otherwise, so a hidden spreader is never named here.
	EmitRumorCreated(ctx, deps.Q, deps.Manager, plan.GameID, rumor)

	for _, choice := range choices {
		if choice == "leverage_target" {
			if err := srLeverageSubject(ctx, deps, plan, &game, subject, onMar); err != nil {
				return err
			}
		}
	}
	if gloats := pickedIn(choices, "reveal_source"); gloats > 0 {
		srLogGloat(ctx, deps, plan, subject, author, gloats, hideSource, onMar)
	}
	return nil
}

// pickedIn counts how many times option appears in choices.
func pickedIn(choices []string, option string) int {
	n := 0
	for _, c := range choices {
		if c == option {
			n++
		}
	}
	return n
}

// srLeverageSubject applies one "leverage_target" pick to the rumor's subject —
// the target asset on a make, the preparer's main character on a mar. A mar that
// finds no main character, or an asset that is already leveraged, says so in the
// log rather than quietly spending the pick.
func srLeverageSubject(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	game *dbgen.Game,
	subject *int64,
	onMar bool,
) error {
	if subject == nil {
		if onMar {
			srLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s has no main character to leverage — the counter-rumor's pick comes to nothing.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID)))
		}
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *subject)
	if err != nil {
		return fmt.Errorf("could not load the rumor's subject: %w", err)
	}
	if asset.IsLeveraged {
		srLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s was already leveraged — the pick changes nothing.", assetMark(asset.Name)))
		return nil
	}
	if err := deps.Q.SetAssetLeveraged(
		ctx,
		dbgen.SetAssetLeveragedParams{ID: asset.ID, IsLeveraged: true},
	); err != nil {
		return fmt.Errorf("could not leverage target asset: %w", err)
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{AssetID: asset.ID})
	EmitAssetLeveraged(ctx, deps.Q, deps.Manager, plan.GameID, asset, logRow(*game))
	return nil
}

// srLogGloat posts option (e): "Reveal yourself as the source of the rumor. Relish
// in its glory." It is an expressive beat with no mechanical effect — it does NOT
// touch attribution (DESIGN_PRINCIPLES.md principle 5; FINDINGS Q1) — so the log
// line is the whole of what it does, and is written to be conspicuous. The gloater
// is named as a player: the players always know. If the same batch also hides the
// source, the gloating happens where the court can't see it.
func srLogGloat(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	subject *int64,
	gloater int64,
	times int,
	hidden, onMar bool,
) {
	name := playerDisplayName(ctx, deps.Q, gloater)
	about := playerDisplayName(ctx, deps.Q, plan.PreparerID)
	if subject != nil {
		about = assetDisplayName(ctx, deps.Q, *subject)
	}
	var body string
	switch {
	case onMar && hidden:
		body = fmt.Sprintf("%s relishes the counter-rumor about %s in private, savouring a triumph "+
			"the whole court is whispering about and no one can trace back to them.", name, about)
	case onMar:
		body = fmt.Sprintf("%s relishes the counter-rumor about %s — the whole court is whispering, "+
			"and %s makes very sure everyone knows whose handiwork it is, basking in every gasp.", name, about, name)
	case hidden:
		body = fmt.Sprintf("%s relishes the rumor about %s in private, savouring a triumph "+
			"the whole court is whispering about and no one can trace back to them.", name, about)
	default:
		body = fmt.Sprintf("%s basks in the glory of the rumor about %s — it is on every tongue at court, "+
			"and %s makes very sure everyone knows whose handiwork it is.", name, about, name)
	}
	if times > 1 {
		body += fmt.Sprintf(" And then they do it %d times over.", times)
	}
	srLog(ctx, deps, plan, model.SeverityImportant, body)
}

// CanComplete blocks completion while a take-asset consent request is still
// awaiting the owner's answer (the analogue of Seek Answers' pending-question
// gate — completing here would strand an in-flight asset transfer), and until
// every committed break_target / hide_source pick has been performed or forfeited
// (sr-forfeit-step). The make/mar effects are server-authoritative: the client's
// "all steps done" gate is UX, not the only enforcement.
func (srHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sr := resData.SpreadRumors
	if sr == nil {
		return nil
	}
	if sr.PendingTakeConsent != nil {
		return errors.New("a take-asset request is awaiting the owner's consent before you can complete")
	}
	return subflowPicksRemaining(resData,
		subflowProgress{srOptBreakTarget, "break-target", sr.BreakTargetDone},
		subflowProgress{srOptHideSource, "hide-source", sr.HideSourceDone},
	)
}

// MaxChoices: make picks equal to the result (repeatable); the mar counter-rumor
// picks equal to (difficulty − result).
func (srHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return int(rollResult)
	}
	return int(difficulty - rollResult)
}

// PreparedDescriptor customizes the plan.prepared log post. It always names the
// target asset the rumor is about; when the preparer kept the rumor secret it
// names the asset holding the hidden text rather than the text itself (which
// would otherwise leak the secret), otherwise it states the rumor openly.
func (srHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	resData *ResolutionData,
) (string, bool) {
	about := fallbackAssetName
	if plan.TargetAssetID != nil {
		about = assetDisplayName(ctx, q, *plan.TargetAssetID)
	}
	if sr := resData.SpreadRumors; sr != nil && sr.IsSecret && sr.SecretAssetID != nil {
		keeper := assetDisplayName(ctx, q, *sr.SecretAssetID)
		return fmt.Sprintf(
			"prepared Spread Rumors: there's a rumor brewing about %s, but it's a secret kept by %s.",
			about, keeper), true
	}
	notes := ""
	if plan.PreparationNotes != nil {
		notes = strings.TrimSpace(*plan.PreparationNotes)
	}
	if notes == "" {
		return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s.", about), true
	}
	return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s — %q", about, notes), true
}

func (srHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"sr-cast-roll":         srCastRollHandler(deps),
		"sr-counter-rumor":     srCounterRumorHandler(deps),
		"break-target":         srBreakTargetHandler(deps),
		"hide-source":          srHideSourceHandler(deps),
		"request-take-consent": srRequestTakeConsentHandler(deps),
		"respond-take-consent": srRespondTakeConsentHandler(deps),
		"sr-forfeit-step":      srForfeitStepHandler(deps),
	}
}

// srLog emits a Spread Rumors action-log entry anchored to the plan's row.
func srLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.spread_rumors",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// ResolvingWaitees names whoever the table is actually waiting on once a Spread
// Rumors resolution is past its pre-roll:
//
//   - a pending "take asset" consent → the victim (AwaitTakeConsent), so the table
//     blocks on them rather than the resolving plan's focus player;
//   - a mar with the counter-rumor, the options or a sub-flow step still owed → the
//     mar actor (the target asset's live owner), who drives the whole mar branch;
//   - a make with sub-flow steps still owed under a Make Demands perform_steps win
//     → the winner, who stands in for the preparer (the server refuses the preparer
//     those steps, as in Spread Propaganda).
//
// Anything else — including the pre-roll, which the preparer casts — rides the
// generic PlanResolving case and names the preparer. Once nothing is owed the
// preparer completes (completion is always theirs), which is that same case.
func (srHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sr := resData.SpreadRumors; sr != nil && sr.PendingTakeConsent != nil {
		victim := sr.PendingTakeConsent.VictimID
		return model.RowState{Kind: model.RowStateAwaitTakeConsent, ActingPlayerIDs: []int64{victim}}, true
	}
	if planRollIsMar(ctx, q, plan) {
		if err := (srHandler{}).CanComplete(plan, &resData); err == nil && len(resData.MakeMarChoices) > 0 {
			return model.RowState{}, false // nothing owed: the preparer completes
		}
		if plan.TargetAssetID == nil {
			return model.RowState{}, false
		}
		asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
		if err != nil {
			return model.RowState{}, false
		}
		return model.RowState{Kind: model.RowStatePlanResolving, ActingPlayerIDs: []int64{asset.OwnerID}}, true
	}
	winner := performStepsWinner(ctx, q, plan)
	if winner == 0 || winner == plan.PreparerID || len(resData.MakeMarChoices) == 0 {
		return model.RowState{}, false
	}
	if err := (srHandler{}).CanComplete(plan, &resData); err == nil {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStatePlanResolving, ActingPlayerIDs: []int64{winner}}, true
}

// ── Re-aim (TargetReAimer) ───────────────────────────────────────────────────

// StaleAim: a rumor is ABOUT an asset, so it stands while that asset does. A
// change of owner leaves it valid on purpose — the gossip follows the asset, not
// whoever holds it this week — and the difficulty and the mar-branch actor (both
// read off the asset's live owner, never cached) simply move with it. Only the
// asset ceasing to exist makes "start a rumor about it" meaningless.
//
// The kept-secret references (resolution_data's SecretAssetID / SecretID) are
// deliberately NOT checked: secrets are never deleted, and a make reads the text
// by SecretID, so losing the asset that shelters the secret changes who can read
// it, not whether the rumor can be published.
func (srHandler) StaleAim(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (AimLoss, error) {
	if plan.TargetAssetID == nil {
		return AimLoss{}, nil
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		if !isNoRows(err) {
			return AimLoss{}, err
		}
		const gone = "The asset this rumor was about no longer exists."
		return AimLoss{Plain: gone, Log: gone}, nil
	}
	if asset.IsDestroyed {
		return AimLoss{
			Plain: fmt.Sprintf("%s, the subject of the rumor, has been destroyed.", asset.Name),
			Log: fmt.Sprintf("%s, the subject of the rumor, has been destroyed.",
				assetMark(asset.Name)),
		}, nil
	}
	return AimLoss{}, nil
}

// ReAimOptions: any living asset in the game. Preparation accepts any asset for a
// rumor — including the preparer's own (the take-consent flow even has a
// "self-consent" case for it) — so the re-aim does too.
func (srHandler) ReAimOptions(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) ([]ReAimOption, error) {
	assets, err := q.ListAssetsByGame(ctx, plan.GameID) // live assets only
	if err != nil {
		return nil, err
	}
	out := make([]ReAimOption, 0, len(assets))
	for _, a := range assets {
		// The rumor's player column is left as it was; only the subject asset moves.
		out = append(out, ReAimOption{TargetPlayerID: plan.TargetPlayerID, TargetAssetID: new(a.ID)})
	}
	return out, nil
}

func (srHandler) DescribeAim(ctx context.Context, q *dbgen.Queries, _ *dbgen.Plan, opt ReAimOption) string {
	if opt.TargetAssetID == nil {
		return "at a new subject"
	}
	return "at " + assetDisplayName(ctx, q, *opt.TargetAssetID)
}
