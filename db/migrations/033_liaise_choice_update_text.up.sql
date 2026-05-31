-- Add update_text to liaise_choices.
--
-- Clandestinely Liaise's "update_peer" Things-We-Share option lets the actor
-- EDIT one marginalia on the partner's meeting peer (rewriting its text), as
-- opposed to "break_peer" which tears one. Because effects are deferred until
-- both players submit, the authored replacement text must be recorded on the
-- choice alongside the target marginalia.

ALTER TABLE liaise_choices
  ADD COLUMN update_text TEXT;
