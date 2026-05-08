// Mirror of game.FollowOnPrompt (Go side) for the scene-setup form, which
// needs the prompt before the scene exists in the DB. Once the scene has
// been created the server stores the prompt on the row and the client uses
// `scene.prompt` directly — only the setup form needs this map.
//
// Keep these strings in sync with /uneasy/game/scenes.go.

import type { PlanType, Plan } from './api';

const FOLLOW_ON_PROMPTS: Record<PlanType, string> = {
	make_demands:
		"Set a scene of your character grappling with the demands of the people they surround themselves with.",
	propose_decree:
		"Set a scene of your character interacting with one of the laws under the public record.",
	exchange_courtiers:
		"Set a scene where your character must get their bearings in an unfamiliar situation.",
	make_war:
		"Set a scene that shows the impacts the start of this war has on your character's life, if any such impacts exist.",
	make_introductions:
		"Set a scene of your character meeting somebody for the first time.",
	seek_answers:
		"Set a scene of your character learning something new.",
	chronicle_histories:
		"Set a scene that shows how a moment in your character's past shaped them into the person they are today.",
	clandestinely_liaise:
		"Set a scene of your character doing something they'd prefer to keep secret.",
	spread_propaganda:
		"Set a scene that shows your character confronting a new idea.",
	spread_rumors:
		"Set a scene of your character engaging in the time-honored tradition of gossip.",
	propose_duel:
		"Set a scene that shows your character in a heated disagreement.",
	host_festivity:
		"Set a scene of your character recovering from the events of this social occasion.",
};

export const FREE_SCENE_PROMPT_FALLBACK =
	"No follow-on prompt — set any scene.";

/**
 * Picks the prompt for a brand-new scene by finding the most recently
 * resolved plan on the given row, if any. Mirrors the server's selection
 * logic in handler/scenes.go.
 */
export function followOnPromptForRow(plans: Plan[], rowNumber: number): string {
	const resolvedOnRow = plans.filter(
		p => p.row_number === rowNumber && p.status === 'resolved'
	);
	if (resolvedOnRow.length === 0) return FREE_SCENE_PROMPT_FALLBACK;

	// `resolved_at` is the canonical ordering column server-side; fall back
	// to id (monotonic) if it's missing for any reason.
	const sorted = [...resolvedOnRow].sort((a, b) => {
		const at = a.resolved_at ?? '';
		const bt = b.resolved_at ?? '';
		if (at !== bt) return bt.localeCompare(at);
		return b.id - a.id;
	});
	return FOLLOW_ON_PROMPTS[sorted[0].plan_type] ?? FREE_SCENE_PROMPT_FALLBACK;
}
