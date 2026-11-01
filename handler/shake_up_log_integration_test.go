//go:build integration

// handler/shake_up_log_integration_test.go — the Shake-Up half of the
// log-completeness sweep (see log_completeness_integration_test.go for the plans).
//
// Same rule: every board change and every table-visible decision posts a
// correctly attributed line. The Shake-Up's posts carry no row anchor — the
// game's current_row is 14, which scene_posts.row_number's FK would silently
// drop — so each test also pins that the post actually landed.
//
// Gaps these pin:
//   - BeginShakeUp refreshed every leveraged asset and flipped the phase with no
//     post at all (and the Esteem boundary never existed — only knowledge and
//     power got one);
//   - ShakeUpPass broadcast shake_up.passed but wrote nothing, although the
//     overhaul plan specified a Minor "lets it stand" line;
//   - ShakeUpAnnounce named only the asset: not whose it was, which marginalia a
//     break would tear, or which title a claim would stamp — the very things the
//     other players bid against;
//   - the claim-title commit line dropped the marginalia wording it stamped.
// (The focus-pass half lives in focus_pass_log_integration_test.go.)

package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/hub"
	"uneasy/model"
)

// postsWithCode returns the game's log posts carrying the given system code.
func postsWithCode(t *testing.T, q *dbgen.Queries, gameID int64, code string) []dbgen.ScenePost {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []dbgen.ScenePost
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == code {
			out = append(out, p)
		}
	}
	return out
}

func TestShakeUpLog_Begin_PostsBoundaryAndRefresh(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	tg := newTestGame(t, q, 2)
	owned, err := q.ListAssetsByOwner(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, owned)
	leveraged := owned[0]
	require.NoError(t, q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{ID: leveraged.ID, IsLeveraged: true}))

	require.NoError(t, BeginShakeUp(ctx, q, manager, tg.Game.ID))

	got, err := q.GetAssetByID(ctx, leveraged.ID)
	require.NoError(t, err)
	assert.False(t, got.IsLeveraged, "the Shake-Up opens by refreshing everything leveraged")

	cats := postsWithCode(t, q, tg.Game.ID, "shake_up.category")
	require.Len(t, cats, 1, "the Esteem pass needs its boundary like the other two")
	assert.Equal(t, "Shake-Up — Esteem", cats[0].Body)
	assert.Equal(t, model.SeverityBoundary, cats[0].Severity)

	began := postsWithCode(t, q, tg.Game.ID, "shake_up.began")
	require.Len(t, began, 1)
	assert.Contains(t, began[0].Body, playerDisplayName(ctx, q, tg.Players[0].ID),
		"the refresh names whose board changed, as a player mark")
	assert.Contains(t, began[0].Body, "refreshed "+assetMark(leveraged.Name))
	assert.Nil(t, began[0].RowNumber, "current_row is 14 here; an anchor would drop the post")
}

func TestShakeUpLog_Begin_NothingLeveraged(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 2)
	require.NoError(t, BeginShakeUp(ctx, q, hub.NewManager(), tg.Game.ID))

	began := postsWithCode(t, q, tg.Game.ID, "shake_up.began")
	require.Len(t, began, 1, "the Shake-Up's opening is logged even when there is nothing to refresh")
	assert.Contains(t, began[0].Body, "no asset was leveraged")
}

func TestShakeUpLog_Pass_PostsLetsItStand(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	passerIdx := (spenderIdx + 1) % 3
	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

	h.pass(t, passerIdx, spendID)

	passes := postsWithCode(t, h.q, h.gameID(), "shake_up.passed")
	require.Len(t, passes, 1)
	assert.Equal(t, fmt.Sprintf("%s lets %s's spend stand",
		playerDisplayName(ctx, h.q, h.seeded.Players[passerIdx].ID),
		playerDisplayName(ctx, h.q, h.seeded.Players[spenderIdx].ID)), passes[0].Body)
	assert.Equal(t, model.SeverityMinor, passes[0].Severity)

	// Passing again is an idempotent no-op — it must not repeat the line.
	h.pass(t, passerIdx, spendID)
	assert.Len(t, postsWithCode(t, h.q, h.gameID(), "shake_up.passed"), 1)
}

func TestShakeUpLog_Announce_NamesOwnerAndMarginalia(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	ownerIdx := 1 - spenderIdx
	owner := h.seeded.Players[ownerIdx]

	// A take: the line says whose asset is being aimed at.
	mcID, err := findMainCharacter(ctx, h.q, owner.ID)
	require.NoError(t, err)
	mc, err := h.q.GetAssetByID(ctx, mcID)
	require.NoError(t, err)
	h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": mcID,
	})
	announced := postsWithCode(t, h.q, h.gameID(), "shake_up.spend_announced")
	require.Len(t, announced, 1)
	assert.Contains(t, announced[0].Body,
		fmt.Sprintf("targeting %s's %s", playerDisplayName(ctx, h.q, owner.ID), assetMark(mc.Name)))
}

func TestShakeUpLog_Announce_BreakNamesTheMarginalia(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	owner := h.seeded.Players[1-spenderIdx]
	res, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.gameID(), OwnerID: owner.ID, CreatorID: owner.ID,
		AssetType: model.AssetResource, Name: "Granary",
	})
	require.NoError(t, err)
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: res.ID, Position: 1, Text: "well-stocked",
	})
	require.NoError(t, err)

	h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptBreakResource, "target_asset_id": res.ID, "target_marginalia_id": m.ID,
	})

	announced := postsWithCode(t, h.q, h.gameID(), "shake_up.spend_announced")
	require.Len(t, announced, 1)
	assert.Contains(t, announced[0].Body, "targeting "+playerDisplayName(ctx, h.q, owner.ID)+"'s **Granary**")
	assert.Contains(t, announced[0].Body, `to tear "well-stocked"`)
}

func TestShakeUpLog_ClaimTitle_AnnounceAndCommitCarryTitleAndWording(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()
	power := gamepkg.ShakeUpCategoryPower
	spending := gamepkg.ShakeUpStepSpending
	require.NoError(t, h.q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
		ID: h.gameID(), ShakeUpCategory: &power, ShakeUpStep: &spending,
	}))

	spenderIdx := h.currentActorIdx(t)
	mcID, err := findMainCharacter(ctx, h.q, h.seeded.Players[spenderIdx].ID)
	require.NoError(t, err)
	mc, err := h.q.GetAssetByID(ctx, mcID)
	require.NoError(t, err)

	spendID := h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptClaimTitle, "target_asset_id": mcID,
		"target_title_id": gamepkg.TitleMonarch, "title_flavor": "crowned at the Midwinter Accord",
	})

	announced := postsWithCode(t, h.q, h.gameID(), "shake_up.spend_announced")
	require.Len(t, announced, 1)
	assert.Contains(t, announced[0].Body, "claiming the title **The Monarch**")
	assert.Contains(t, announced[0].Body, `"crowned at the Midwinter Accord"`)
	assert.Contains(t, announced[0].Body, "on "+assetMark(mc.Name))

	h.pass(t, 1-spenderIdx, spendID)
	status, body := h.commit(t, spenderIdx, spendID)
	require.Equal(t, http.StatusOK, status, body)

	posts := committedPosts(t, h.q, h.gameID())
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `: "crowned at the Midwinter Accord"`,
		"the commit line carries the marginalia wording the peer's card now shows")
}

// A shake-up roll's leverage already posts through the generic roll.commit path;
// this pins that it still lands with no plan/row anchor (current_row is 14).
func TestShakeUpLog_RollLeverage_PostsWithoutRowAnchor(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	roll := asMap(t, h.activeRoll(t, 0), "roll")
	rollID := asInt64(t, roll["id"])
	actorID := asInt64(t, roll["actor_id"])
	assets, err := h.q.ListAssetsByOwner(ctx, actorID)
	require.NoError(t, err)
	require.NotEmpty(t, assets)

	status, body := h.do(t, h.playerIdxByID(t, actorID), "POST", fmt.Sprintf("/api/rolls/%d/leverage", rollID),
		map[string]any{"asset_id": assets[0].ID})
	require.Equal(t, http.StatusOK, status, body)

	commits := postsWithCode(t, h.q, h.gameID(), "roll.commit")
	require.Len(t, commits, 1)
	assert.Contains(t, commits[0].Body, assetMark(assets[0].Name))
	assert.Nil(t, commits[0].RowNumber)
}
