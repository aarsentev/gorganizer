CREATE TABLE trackers (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL CHECK (length(trim(name)) > 0),
    kind        TEXT NOT NULL CHECK (kind IN ('boolean', 'number', 'rating')),
    unit        TEXT NOT NULL DEFAULT '', -- e.g. pages; values are whole numbers
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
    created_utc INTEGER NOT NULL
);

CREATE TABLE tracker_entries (
    tracker_id  INTEGER NOT NULL REFERENCES trackers(id),
    -- The date being answered, never recalculated from the submission timestamp.
    day         TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    value       INTEGER NOT NULL CHECK (typeof(value) = 'integer' AND value >= 0),
    created_utc INTEGER NOT NULL,
    updated_utc INTEGER NOT NULL,
    PRIMARY KEY (tracker_id, day)
) WITHOUT ROWID;
