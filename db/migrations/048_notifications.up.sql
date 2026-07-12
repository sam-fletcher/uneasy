-- 048_notifications.up.sql
-- adr/NOTIFICATIONS_PLAN.md Session 3: schema for web-push turn reminders.
--
-- accounts.notify_cadence_hours is the single per-account timing knob (the
-- old notify_mode TEXT enum sketch never shipped): NULL = off, otherwise one
-- of the five fixed cadence options. push_subscriptions holds one row per
-- browser/device a player has opted push into. pending_notifications rows
-- ARE the reconciler's timers — one per (player, game) currently named by
-- ComputeWaitState; the ticker upserts/deletes them to match the live waitee
-- set and sends whichever are past due_at (handler/push_notifications.go).
--
-- Both new tables cascade off games/players/accounts per the migration-041
-- invariant (deleting a game must clean up its whole ownership graph).

ALTER TABLE accounts
  ADD COLUMN notify_cadence_hours SMALLINT DEFAULT 24
    CHECK (notify_cadence_hours IN (1, 3, 8, 24, 72));

CREATE TABLE push_subscriptions (
  id         BIGSERIAL   PRIMARY KEY,
  account_id BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  endpoint   TEXT        NOT NULL UNIQUE,
  p256dh     TEXT        NOT NULL,
  auth       TEXT        NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_account ON push_subscriptions(account_id);

CREATE TABLE pending_notifications (
  player_id        BIGINT      PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
  game_id          BIGINT      NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  first_waiting_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  due_at           TIMESTAMPTZ NOT NULL
);
CREATE INDEX pending_notifications_due_at ON pending_notifications(due_at);
CREATE INDEX pending_notifications_game_id ON pending_notifications(game_id);
