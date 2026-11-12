-- Temporarily disabled devices: not checked, left out of the location state
-- and of the live view. disabled_until (optional) re-enables them automatically.
ALTER TABLE devices ADD COLUMN disabled_at INTEGER;
ALTER TABLE devices ADD COLUMN disabled_until INTEGER;
