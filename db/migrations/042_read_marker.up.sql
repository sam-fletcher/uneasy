-- 042_read_marker.up.sql
-- Chat Overhaul Phase 1a (adr/CHAT_OVERHAUL_PLAN.md). Unread state is
-- server-side so it stays correct across devices, unlike a client-local
-- "seen" flag. Tracks the newest scene_posts.id each player has read;
-- monotonic (only ever moves forward — see the read-marker endpoint).
ALTER TABLE players ADD COLUMN last_read_post_id BIGINT NOT NULL DEFAULT 0;
