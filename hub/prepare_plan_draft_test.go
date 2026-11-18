package hub

// The prepare_plan.draft relay: while the focus player fills in a plan's prep form,
// the server fans their draft out to every client at the table so each watcher's
// read-only mirror fills in as it is typed. Nothing is stored. A secret's existence
// is public and its text is not, so a kept-secret Spread Rumors rumor must not reach
// the other clients with its text, whatever the sending client did.

import (
	"encoding/json"
	"strings"
	"testing"

	"uneasy/model"
)

const secretRumor = "The duchess is not who she says she is."

// relayDraft has a focus player (id 1) send a prepare_plan.draft command with the
// given body and returns the broadcast a watcher (id 2) receives, raw and decoded.
func relayDraft(t *testing.T, cmd string) (raw []byte, payload model.PreparePlanDraftPayload) {
	t.Helper()
	m := NewManager()
	h := m.GetOrCreate(7)
	sender, watcher := testClient(h, 1), testClient(h, 2)
	h.Register(sender)
	h.Register(watcher)

	sender.handleCommand([]byte(cmd))

	waitFor(t, func() bool {
		for _, msg := range drain(watcher.send) {
			var env struct {
				Type    string                        `json:"type"`
				Payload model.PreparePlanDraftPayload `json:"payload"`
			}
			if json.Unmarshal(msg, &env) == nil && env.Type == model.EventPreparePlanDraft {
				raw, payload = msg, env.Payload
				return true
			}
		}
		return false
	}, "the watcher never received the draft")
	return raw, payload
}

func TestPreparePlanDraftKeepsASecretRumorsTextBack(t *testing.T) {
	// Exactly what an unfixed client sends: the text, with the box ticked.
	raw, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
		"plan_type":"spread_rumors",
		"prep":{"filter_owner_id":2,"target_asset_id":20,"notes":"`+secretRumor+`",
		        "keep_secret":true,"secret_asset_id":11}}}`)

	if strings.Contains(string(raw), secretRumor) {
		t.Fatalf("a kept-secret rumor's text reached another client: %s", raw)
	}
	if _, has := got.Prep["notes"]; has {
		t.Fatalf("a kept-secret draft still carries a notes key: %v", got.Prep)
	}
	// That the rumor is secret, and which asset holds it, stay public.
	if got.Prep["keep_secret"] != true || got.Prep["secret_asset_id"] != float64(11) {
		t.Fatalf("the secret flag and holder must still be relayed, got %v", got.Prep)
	}
	if got.Prep["target_asset_id"] != float64(20) || got.Prep["filter_owner_id"] != float64(2) {
		t.Fatalf("the rest of the draft must be relayed untouched, got %v", got.Prep)
	}
	// The relay still stamps who sent it, as it does for every draft.
	if got.PlayerID != 1 || got.PlanType != "spread_rumors" {
		t.Fatalf("player and plan type must survive, got %+v", got)
	}
}

func TestPreparePlanDraftStripsWhateverKeepSecretSays(t *testing.T) {
	// Failing closed: the flag arrives as JSON from a client, and an old client always sends
	// it with a Spread Rumors draft, ticked or not. Any value of it marks the text as a
	// rumor's, so none is allowed to carry the text through.
	for _, flag := range []string{`true`, `false`, `"true"`, `1`, `null`} {
		raw, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
			"plan_type":"spread_rumors",
			"prep":{"notes":"`+secretRumor+`","keep_secret":`+flag+`}}}`)
		if strings.Contains(string(raw), secretRumor) {
			t.Errorf("keep_secret=%s let the text through: %s", flag, raw)
		}
		if _, has := got.Prep["notes"]; has {
			t.Errorf("keep_secret=%s left a notes key: %v", flag, got.Prep)
		}
	}
}

func TestPreparePlanDraftStripsASpreadRumorsDraftWhateverPlanItIsLabelled(t *testing.T) {
	// An old client stamps plan_type when its 150ms send timer fires, not when the key is
	// pressed, and keeps the old panel mounted while the next card's code loads. A preparer
	// who types a rumor and taps another card inside that window sends the rumor's text and
	// its keep_secret flag, ticked or not, under the NEXT card's plan_type, and the watcher's
	// mirror of that plan copies `notes` into its box. Only Spread Rumors' panel sends
	// keep_secret, so the key marks the text whatever the label says.
	for _, planType := range []string{"seek_answers", "chronicle_histories", ""} {
		for _, flag := range []string{"true", "false"} {
			raw, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
				"plan_type":"`+planType+`",
				"prep":{"notes":"`+secretRumor+`","keep_secret":`+flag+`,"secret_asset_id":11}}}`)
			if strings.Contains(string(raw), secretRumor) {
				t.Errorf(
					"plan_type=%q keep_secret=%s: the text got through under another label: %s",
					planType,
					flag,
					raw,
				)
			}
			if _, has := got.Prep["notes"]; has {
				t.Errorf("plan_type=%q keep_secret=%s: a notes key survived: %v", planType, flag, got.Prep)
			}
		}
	}
}

func TestPreparePlanDraftStripsASpreadRumorsDraftWithNoFlagAtAll(t *testing.T) {
	// A draft that is plainly Spread Rumors by its label carries no text, whatever else it
	// does or doesn't say: a rumor's text is only ever shown to the table when the plan is
	// prepared, never as it is typed.
	raw, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
		"plan_type":"spread_rumors","prep":{"notes":"`+secretRumor+`","target_asset_id":20}}}`)
	if strings.Contains(string(raw), secretRumor) {
		t.Fatalf("a Spread Rumors draft's text reached another client: %s", raw)
	}
	if got.Prep["target_asset_id"] != float64(20) {
		t.Fatalf("the rest of the draft must be relayed, got %v", got.Prep)
	}
}

func TestPreparePlanDraftLeavesOtherPlansAlone(t *testing.T) {
	// Every other plan's prep text becomes public the moment it is prepared (it is in
	// the plan.prepared log), so its draft is public too.
	_, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
		"plan_type":"seek_answers","prep":{"notes":"asking the old nurse"}}}`)
	if got.Prep["notes"] != "asking the old nurse" {
		t.Fatalf("another plan's notes must be relayed, got %v", got.Prep)
	}
}

func TestPreparePlanDraftRelaysACardHighlightWithNoForm(t *testing.T) {
	// A card picked but the form not started: prep is null, and that must not trip the
	// redaction.
	_, got := relayDraft(t, `{"type":"prepare_plan.draft","payload":{
		"plan_type":"spread_rumors","prep":null}}`)
	if got.PlanType != "spread_rumors" || got.Prep != nil {
		t.Fatalf("a bare highlight must pass through, got %+v", got)
	}
}
