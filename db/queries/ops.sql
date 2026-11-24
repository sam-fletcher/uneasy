-- Operational monitoring (docs/OPERATIONS.md "Alerts").

-- name: GetBackupLastSuccess :one
-- When the weekly backup workflow last finished. No row until the first
-- backup after migration 059 has run; callers treat that as "unknown", not stale.
SELECT last_success_at FROM ops_backup_status WHERE id;
