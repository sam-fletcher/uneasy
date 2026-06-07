// Single source of truth for the "send feedback" link, used by the in-game
// Help panel and the Profile page. Change FEEDBACK_EMAIL to wherever you want
// player feedback to land (a shared inbox, a forwarding address, etc.).
export const FEEDBACK_EMAIL = 'caesarr7@gmail.com';

const SUBJECT = 'Uneasy Lies the Head — feedback';
const BODY = 'What were you doing?\n\n\nWhat happened / what was confusing?\n\n\nAnything else?\n\n';

export const feedbackHref =
	`mailto:${FEEDBACK_EMAIL}` +
	'?subject=' + encodeURIComponent(SUBJECT) +
	'&body=' + encodeURIComponent(BODY);
