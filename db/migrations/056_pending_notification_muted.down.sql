-- 056_pending_notification_muted.down.sql
-- Drops the per-wait mute. Nothing to preserve: every muted row is silenced
-- only until the table next moves past its player, so the column holds no
-- durable user setting — reverting simply un-silences the waits currently
-- muted, and reminders resume at their backed-off cadence.
ALTER TABLE pending_notifications DROP COLUMN muted;
