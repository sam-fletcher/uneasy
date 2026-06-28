-- 036_shake_up_break_marginalia.up.sql
-- Shake-Up "break a … asset" now tears ONE marginalia (the canonical break),
-- so a break spend must carry which marginalia the breaker chose — mirroring
-- every other hostile break in the game. Previously the break effect destroyed
-- the whole asset in one token, skipping the secret-reveal-on-break, the
-- "how has it changed?" prompt, and the marginalia.torn action-log entry.
ALTER TABLE shake_up_spends
  ADD COLUMN target_marginalia_id BIGINT REFERENCES marginalia(id);
