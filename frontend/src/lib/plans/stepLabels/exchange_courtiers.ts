// Exchange Courtiers: the target names a peer for a fair trade, the preparer
// accepts or declines it, the dice roll, and then a make is the preparer's to
// describe while a mar is the target's. A target's step may follow (a messy break),
// and a riposte is the preparer's again. The cursor is the server's own `phase`
// (game.ExchangeCourtiersPhase), so this reads it rather than rebuilding it from the
// choices.
//
// The server calls the target's steps `await_courtier_response`, one kind for three
// different asks (the offer, a mar's choice, a messy break), so the plan says which;
// "respond to the exchange" is for when it can't. The preparer's steps ride
// `plan_resolving`.

import { parseExchangeCourtiersData } from '$lib/plans/resolutionData/exchange_courtiers';
import type { StepLabelFn } from '../stepLabel';

export const exchangeCourtiersStepLabel: StepLabelFn = ({ plan, roll }) => {
	const ec = parseExchangeCourtiersData(plan);
	switch (ec.phase ?? 'fair_trade') {
		case 'fair_trade':
			// The target opens with an offer; once it is on the table the preparer
			// takes it or leaves it.
			if (ec.fair_trade_asset_id == null) return 'propose a fair trade';
			return ec.fair_trade_accepted == null ? 'accept or decline the trade' : null;
		case 'roll':
			// After the dice. A make is the preparer's to describe; a mar hands the
			// choice to the target. Before they land the roll is the open step.
			if (roll?.outcome === 'mar') return 'choose how to respond to the mar';
			return roll?.outcome === 'make' ? 'say how the trade went' : null;
		case 'riposte':
			return 'break or surrender the peer';
		case 'messy_break':
			return 'break a marginalia';
		case 'done':
			return null;
	}
};
