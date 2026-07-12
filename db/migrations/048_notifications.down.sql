-- 048_notifications.down.sql
DROP TABLE IF EXISTS pending_notifications;
DROP TABLE IF EXISTS push_subscriptions;
ALTER TABLE accounts DROP COLUMN IF EXISTS notify_cadence_hours;
