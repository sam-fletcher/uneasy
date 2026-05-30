-- Add target_marginalia_id to liaise_choices.
--
-- Clandestinely Liaise's "break_peer" Things-We-Share option breaks the
-- partner's peer. Per the breaking rules the breaker chooses which marginalia
-- to tear, so the choice must record both the target asset and the specific
-- marginalia. (look_at_secret / take_gift / leverage_partner use only
-- target_asset_id; break_peer additionally sets target_marginalia_id.)

ALTER TABLE liaise_choices
  ADD COLUMN target_marginalia_id BIGINT REFERENCES marginalia(id);
