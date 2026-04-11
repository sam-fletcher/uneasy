-- Migration 010: Add resolution_data to plans.
-- Stores JSON for plan-specific resolution state:
--   peer_count (Make Introductions), fair trade state (Exchange Courtiers),
--   and make/mar choices applied.

ALTER TABLE plans ADD COLUMN resolution_data TEXT;
