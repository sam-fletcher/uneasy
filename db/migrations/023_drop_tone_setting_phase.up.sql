-- Tone Setting is no longer a discrete phase. Tones are seeded at table
-- creation and are accessible throughout lobby + prologue, locking when
-- the main event begins. Existing tone_setting games revert to lobby so
-- they can transition cleanly to prologue under the new flow.
UPDATE games SET phase = 'lobby' WHERE phase = 'tone_setting';
