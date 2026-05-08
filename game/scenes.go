package game

import "uneasy/model"

// FollowOnPrompt returns the pre-written scene-setup prompt that follows
// resolution of a plan of the given type. The prompts come from the rules;
// see SCENES_PLAN.md "Scene Follow-on Prompts".
//
// All prompts are phrased to complete the sentence "Set a scene..."; the
// caller is responsible for whatever framing they want to display alongside.
func FollowOnPrompt(pt model.PlanType) string {
	switch pt {
	case model.PlanMakeDemands:
		return "Set a scene of your character grappling with the demands of the people they surround themselves with."
	case model.PlanProposeDecree:
		return "Set a scene of your character interacting with one of the laws under the public record."
	case model.PlanExchangeCourtiers:
		return "Set a scene where your character must get their bearings in an unfamiliar situation."
	case model.PlanMakeWar:
		return "Set a scene that shows the impacts the start of this war has on your character's life, if any such impacts exist."
	case model.PlanMakeIntroductions:
		return "Set a scene of your character meeting somebody for the first time."
	case model.PlanSeekAnswers:
		return "Set a scene of your character learning something new."
	case model.PlanChronicleHistories:
		return "Set a scene that shows how a moment in your character's past shaped them into the person they are today."
	case model.PlanClandestinelyLiaise:
		return "Set a scene of your character doing something they'd prefer to keep secret."
	case model.PlanSpreadPropaganda:
		return "Set a scene that shows your character confronting a new idea."
	case model.PlanSpreadRumors:
		return "Set a scene of your character engaging in the time-honored tradition of gossip."
	case model.PlanProposeDuel:
		return "Set a scene that shows your character in a heated disagreement."
	case model.PlanHostFestivity:
		return "Set a scene of your character recovering from the events of this social occasion."
	}
	return ""
}

// FreeScenePromptFallback is shown to the focus player when there is no
// resolved-plan prompt to follow on from (i.e. a row with no plans, or the
// first scene at the start of a row).
const FreeScenePromptFallback = "No follow-on prompt — set any scene."

// MaxCustomLocationLen caps the user-supplied custom location string.
const MaxCustomLocationLen = 80
