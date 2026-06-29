//go:build integration

// handler/plan_lifecycle_harness_test.go — shared end-to-end harness for
// driving a single plan from prepare through complete.
//
// Most plans share the same lifecycle:
//
//	prepare → (rows advance) → kickoff resolution → roll resolves →
//	make-choice → complete.
//
// Tests that exercise that lifecycle were missing for 11 of the 12 plans
// (only Make Demands had end-to-end coverage). This harness lets each
// plan get its own targeted lifecycle test without re-implementing the
// HTTP wiring, session seeding, row-jumping, and roll-forcing each time.
//
// Plans with extra sub-flows (Make Demands drafts, Make War cost-of-battle,
// Festivity multi-guest, Liaise delay-reveal, Duel bouts) layer their own
// route calls on top via `post(...)` — the harness handles the common parts.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// planLifecycle is a one-game integration harness wiring the real chi router
// for the plan endpoints (prepare-plan, resolve, make-choice, complete) plus
// every registered plan's extra routes. Each player has a seeded session
// cookie so post(i, ...) authenticates as players[i].
type planLifecycle struct {
	t       *testing.T
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	store   *db.Store
	manager *hub.Manager
	tg      testGame
	router  http.Handler
	tokens  []string
}

// newPlanLifecycle spins up a fresh game with n players, mounts the plan
// routes, and seeds one session per player. Returns the harness — caller
// drives it via prepare/resolve/makeChoice/complete (or run).
func newPlanLifecycle(t *testing.T, n int) *planLifecycle {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, n)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tokens := make([]string, n)
	for i, p := range tg.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
			Token: tok, AccountID: p.AccountID,
		})
		require.NoError(t, err)
		tokens[i] = tok

		// Plan preparation requires the preparer to own at least one peer
		// (and an unrestricted plan needs a main character). Seed both so
		// the harness doesn't 403 the very first prepare call.
		_, err = q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
			GameID: tg.Game.ID, OwnerID: p.ID, CreatorID: p.ID,
			AssetType: model.AssetPeer, Name: "Seed peer " + p.DisplayName,
		})
		require.NoError(t, err)
		_, err = q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
			GameID: tg.Game.ID, OwnerID: p.ID, CreatorID: p.ID,
			AssetType: model.AssetPeer, Name: p.DisplayName + " (MC)",
			IsMainCharacter: true,
		})
		require.NoError(t, err)
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/prepare-plan", PreparePlan(store, manager))
	r.Get("/api/tables/{id}/asset-suggestions", GetAssetSuggestions(store))
	// Simultaneous-reveal submit — needed by variable-delay plans (Clandestinely
	// Liaise, Make War) to assign their row before resolution.
	r.Post("/api/reveals/{revealId}/submit", SubmitReveal(store, manager))
	r.Get("/api/reveals/{revealId}", GetReveal(store))
	r.Route("/api/plans/{planId}", func(rr chi.Router) {
		rr.Post("/resolve", ResolvePlan(store, manager))
		rr.Post("/make-choice", MakeChoice(store, manager))
		rr.Post("/complete", CompletePlan(store, manager))
		deps := &PlanDeps{Store: store, Manager: manager}
		for _, h := range AllHandlers() {
			for route, fn := range h.ExtraRoutes(deps) {
				rr.Post("/"+route, fn)
			}
		}
	})

	return &planLifecycle{
		t: t, pool: pool, q: q, store: store, manager: manager,
		tg: tg, router: r, tokens: tokens,
	}
}

// rowState computes the current authoritative RowState for the game — the
// exact value the WaitingOnBar renders off. Lets lifecycle tests assert who the
// bar names at each transition (C3), not just the asset-level side effects.
func (h *planLifecycle) rowState() model.RowState {
	h.t.Helper()
	rs, err := ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(h.t, err)
	return rs
}

// assertWaitees asserts the row is in kind and blocked on exactly want
// (order-independent). msg is a short label for the transition under test.
func (h *planLifecycle) assertWaitees(msg string, kind model.RowStateKind, want ...int64) {
	h.t.Helper()
	rs := h.rowState()
	assert.Equalf(h.t, kind, rs.Kind, "%s: row-state kind", msg)
	assert.ElementsMatchf(h.t, want, rs.ActingPlayerIDs, "%s: waitees", msg)
}

// post issues an authenticated POST as players[i] and returns the status
// and decoded JSON body (or empty map if the response has no body).
func (h *planLifecycle) post(playerIdx int, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest("POST", path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// get drives a GET request as the given player and returns the status and
// decoded JSON body.
func (h *planLifecycle) get(playerIdx int, path string) (int, map[string]any) {
	h.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// prepare drives POST /api/tables/{id}/prepare-plan as the current focus
// player. Asserts 201 and returns the refreshed plan from the DB.
func (h *planLifecycle) prepare(req PreparePlanRequest) dbgen.Plan {
	h.t.Helper()
	focusIdx := h.focusPlayerIdx()
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/prepare-plan"
	code, body := h.post(focusIdx, path, req)
	require.Equalf(h.t, http.StatusCreated, code, "prepare-plan failed: %v", body)
	planBlob, _ := json.Marshal(body["plan"])
	var p dbgen.Plan
	require.NoError(h.t, json.Unmarshal(planBlob, &p))
	refreshed, err := h.q.GetPlanByID(context.Background(), p.ID)
	require.NoError(h.t, err)
	return refreshed
}

// jumpToRow advances game.current_row directly to row. Used to skip the
// natural N-row gap between preparation and resolution without exercising
// every intervening plan.
func (h *planLifecycle) jumpToRow(row int16) {
	h.t.Helper()
	require.NoError(h.t, h.q.SetCurrentRow(context.Background(), dbgen.SetCurrentRowParams{
		ID: h.tg.Game.ID, CurrentRow: row,
	}))
	h.tg.Game.CurrentRow = row
}

// resolve drives POST /api/plans/{planId}/resolve from the focus player and
// returns the dice roll the handler created (or nil if the plan has no
// roll, e.g. War, Liaise, Festivity). Asserts 200.
func (h *planLifecycle) resolve(planID int64) *dbgen.DiceRoll {
	h.t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/resolve"
	code, body := h.post(h.preparerIdxFor(planID), path, nil)
	require.Equalf(h.t, http.StatusOK, code, "resolve failed: %v", body)
	if body["roll"] == nil {
		return nil
	}
	rollBlob, _ := json.Marshal(body["roll"])
	var roll dbgen.DiceRoll
	require.NoError(h.t, json.Unmarshal(rollBlob, &roll))
	return &roll
}

// forceRoll sets the roll's outcome directly, bypassing actual dice
// resolution. result is the numeric tally to record; for tests that don't
// care it's fine to pass 0.
func (h *planLifecycle) forceRoll(rollID int64, outcome string, result int16) {
	h.t.Helper()
	ctx := context.Background()
	require.NoError(h.t, h.q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: rollID, Result: &result, Outcome: &outcome,
	}))
	// Mirror finalizeRoll: plans that auto-apply their outcome on roll resolution
	// (Propose Decree) do so here too, so tests exercise the real post-roll flow
	// instead of a path forceRoll alone would bypass.
	resolved, err := h.q.GetDiceRollByID(ctx, rollID)
	require.NoError(h.t, err)
	require.NoError(h.t, applyAutoChoiceOnRoll(ctx, h.q, h.manager, &resolved))
}

// makeChoice drives POST /api/plans/{planId}/make-choice as the focus
// player with the given make/mar result and choice keys. Asserts 200.
func (h *planLifecycle) makeChoice(planID int64, result string, choices []string) {
	h.t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/make-choice"
	code, body := h.post(h.preparerIdxFor(planID), path, map[string]any{
		"result": result, "choices": choices,
	})
	require.Equalf(h.t, http.StatusOK, code, "make-choice failed: %v", body)
}

// complete drives POST /api/plans/{planId}/complete from the focus player.
// Asserts 200.
func (h *planLifecycle) complete(planID int64) {
	h.t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/complete"
	code, body := h.post(h.preparerIdxFor(planID), path, nil)
	require.Equalf(h.t, http.StatusOK, code, "complete failed: %v", body)
}

// run is the all-in-one convenience for plans that fit the common shape:
// prepare → jump to the plan's target row → resolve → force the dice roll
// → make-choice → complete. Returns the final plan row from the DB.
//
// Not suitable for plans whose row is decided post-prep by a simultaneous
// reveal (Make War, Clandestinely Liaise) or that resolve without a roll
// (Festivity, sometimes Exchange Courtiers via fair trade) — those tests
// should call the individual steps directly and inject their own sub-flow.
func (h *planLifecycle) run(req PreparePlanRequest, outcome string, choices []string) dbgen.Plan {
	h.t.Helper()
	plan := h.prepare(req)
	require.NotNil(h.t, plan.RowNumber, "run() requires a deterministic target row")
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(h.t, roll, "run() requires a dice roll; use step-by-step calls for roll-less plans")
	h.forceRoll(roll.ID, outcome, 0)
	h.makeChoice(plan.ID, outcome, choices)
	h.complete(plan.ID)
	refreshed, err := h.q.GetPlanByID(context.Background(), plan.ID)
	require.NoError(h.t, err)
	return refreshed
}

// preparerIdxFor returns the index into Players of the plan's preparer — the
// player who resolves it (resolve/make-choice/complete are preparer-gated).
func (h *planLifecycle) preparerIdxFor(planID int64) int {
	h.t.Helper()
	plan, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(h.t, err)
	for i, p := range h.tg.Players {
		if p.ID == plan.PreparerID {
			return i
		}
	}
	h.t.Fatalf("preparer id %d not in test players", plan.PreparerID)
	return -1
}

// focusPlayerIdx returns the index into Players of the current focus
// player. Used for focus-player actions (scene-setting, prepare).
func (h *planLifecycle) focusPlayerIdx() int {
	h.t.Helper()
	g, err := h.q.GetGameByID(context.Background(), h.tg.Game.ID)
	require.NoError(h.t, err)
	require.NotNil(h.t, g.FocusPlayerID, "no focus player set")
	for i, p := range h.tg.Players {
		if p.ID == *g.FocusPlayerID {
			return i
		}
	}
	h.t.Fatalf("focus player id %d not in test players", *g.FocusPlayerID)
	return -1
}

// setFocus reassigns the focus player. Useful for tests where the prepare
// flow handed focus to the next seat but the test wants to drive resolution
// from the original preparer.
func (h *planLifecycle) setFocus(playerIdx int) {
	h.t.Helper()
	id := h.tg.Players[playerIdx].ID
	require.NoError(h.t, h.q.SetFocusPlayer(context.Background(), dbgen.SetFocusPlayerParams{
		ID: h.tg.Game.ID, FocusPlayerID: &id,
	}))
}

// assetID is a convenience helper for tests that need a target asset on
// a peer. Creates the peer owned by players[ownerIdx] and returns its ID.
func (h *planLifecycle) seedPeer(ownerIdx int, name string) int64 {
	h.t.Helper()
	a, err := h.q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
		GameID:    h.tg.Game.ID,
		OwnerID:   h.tg.Players[ownerIdx].ID,
		CreatorID: h.tg.Players[ownerIdx].ID,
		AssetType: model.AssetPeer,
		Name:      name,
	})
	require.NoError(h.t, err)
	return a.ID
}

// seedSecret writes a secret on assetID authored by players[authorIdx]. Only
// the author can see it until someone takes or breaks the asset (which grants
// the new owner / breaker visibility) — used to assert that transfers carry
// secret visibility with them.
func (h *planLifecycle) seedSecret(assetID int64, authorIdx int, text string) int64 {
	h.t.Helper()
	s, err := h.q.CreateSecret(context.Background(), dbgen.CreateSecretParams{
		AssetID:  assetID,
		AuthorID: h.tg.Players[authorIdx].ID,
		Text:     text,
	})
	require.NoError(h.t, err)
	return s.ID
}

// assertSecretVisible asserts that players[viewerIdx] can read the secret on
// assetID identified by secretID.
func (h *planLifecycle) assertSecretVisible(msg string, assetID, secretID int64, viewerIdx int) {
	h.t.Helper()
	visible, err := h.q.ListVisibleSecrets(context.Background(), dbgen.ListVisibleSecretsParams{
		AssetID:  assetID,
		PlayerID: h.tg.Players[viewerIdx].ID,
	})
	require.NoError(h.t, err)
	for _, s := range visible {
		if s.ID == secretID {
			return
		}
	}
	h.t.Fatalf("%s: secret %d on asset %d not visible to player index %d", msg, secretID, assetID, viewerIdx)
}

// seedPeerWithMarginalia creates a peer owned by players[ownerIdx] carrying one
// marginalia at position 1, and returns (assetID, marginaliaID). The marginalia
// text is fixed ("a note") and unasserted; callers that need a marginalia
// target (Make War cost-of-battle, Clandestinely Liaise tear/rewrite) use the
// returned IDs.
func (h *planLifecycle) seedPeerWithMarginalia(ownerIdx int, name string) (assetID, margID int64) {
	h.t.Helper()
	assetID = h.seedPeer(ownerIdx, name)
	m, err := h.q.CreateMarginalia(context.Background(), dbgen.CreateMarginaliaParams{
		AssetID: assetID, Position: 1, Text: "a note",
	})
	require.NoError(h.t, err)
	return assetID, m.ID
}
