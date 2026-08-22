package model

import "time"

// ReminderState summarises whether turn reminders will actually reach a
// player. It is deliberately a verdict rather than the raw settings: the UI
// needs to answer "can I expect a ping to reach them, or do I message them
// another way?", and the three inputs (cadence, device count, pending timer)
// only mean something together.
//
// What this can and cannot know is worth stating, because the gap is the
// whole reason the "no_device" case exists. The server knows the account's
// cadence, that at least one browser completed a push subscription, and that
// a timer is pending. It cannot know whether the OS actually displayed the
// notification (Focus modes, DND, battery optimisation), whether anyone
// looked, or that a browser has since revoked permission — that last one
// reaches us only indirectly, when the relay 404/410s and
// handler/push_notifications.go prunes the subscription. So this type says
// what is *set up*, never what was *delivered*, and the UI copy must not
// promise more.
type ReminderState string

const (
	// ReminderScheduled — a pending_notifications row exists, so a ping is
	// already queued. ReminderDueAt carries when.
	ReminderScheduled ReminderState = "scheduled"

	// ReminderReady — the account has a cadence and at least one subscribed
	// device, but nothing is pending. Normal for a player the game isn't
	// waiting on; also the ≤60s window before the reconcile ticker notices a
	// new waitee.
	ReminderReady ReminderState = "ready"

	// ReminderNoDevice — a cadence is set but no browser has ever completed a
	// subscription (or the last one was pruned as dead). This is the silent
	// failure the whole feature exists to surface: the player believes they
	// are covered and no ping can reach them.
	ReminderNoDevice ReminderState = "no_device"

	// ReminderOff — the account's cadence is NULL. A deliberate choice, not a
	// fault, but it means only an out-of-band message will reach them.
	ReminderOff ReminderState = "off"

	// ReminderExhausted — this player has been blocking this table for longer
	// than reminderGiveUpDays, so the reminders have stopped. Reminders back
	// off as a wait ages and eventually give up entirely, because the only
	// things that ever clear a timer are the player acting and the game
	// ending — neither of which happens to a game a group has quietly
	// abandoned, and a table nobody intends to finish should not nag its last
	// waitee forever.
	//
	// It is a distinct verdict rather than a flavour of "off" because it is
	// nobody's setting: the player still has a cadence and a device, and if
	// the table ever moves again their timer is deleted and rebuilt from
	// scratch at full cadence. What it tells a tablemate is that this
	// particular silence has already been noticed, and that a nudge now has to
	// come from a person.
	ReminderExhausted ReminderState = "exhausted"
)

// PlayerActivity is one seat's presence and reminder summary, as served
// alongside the roster in GetGameState.
//
// LastActiveAt is "last had the table on screen" (migration 055), not socket
// state — live presence still arrives over the WebSocket as PresenceMember,
// and the two are deliberately separate because they have different
// lifetimes: one is a durable timestamp, the other evaporates on redeploy.
type PlayerActivity struct {
	PlayerID     int64      `json:"player_id"`
	LastActiveAt *time.Time `json:"last_active_at"`

	Reminder ReminderState `json:"reminder"`
	// ReminderDueAt is set only for ReminderScheduled.
	ReminderDueAt *time.Time `json:"reminder_due_at"`
}
