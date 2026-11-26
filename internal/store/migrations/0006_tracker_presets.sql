-- Preset identity is independent of the displayed (translated) name. NULL leaves
-- ordinary trackers independent; enabling a preset twice must reuse the same row.
ALTER TABLE trackers ADD COLUMN preset TEXT CHECK (preset IS NULL OR length(trim(preset)) > 0);
CREATE UNIQUE INDEX idx_trackers_preset ON trackers(preset);
