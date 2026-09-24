CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE events (
    id        TEXT PRIMARY KEY,
    title     TEXT NOT NULL,
    start_utc INTEGER NOT NULL,
    end_utc   INTEGER,
    all_day   INTEGER NOT NULL DEFAULT 0,
    location  TEXT,
    deleted   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_events_start ON events(start_utc) WHERE deleted = 0;

-- start_utc is part of the key so a rescheduled event gets reminded again.
CREATE TABLE sent (
    event_id  TEXT NOT NULL,
    start_utc INTEGER NOT NULL,
    kind      TEXT NOT NULL, -- '60m', 'eve', ...
    sent_utc  INTEGER NOT NULL,
    PRIMARY KEY (event_id, start_utc, kind)
) WITHOUT ROWID;

CREATE TABLE jobs_fired (
    job       TEXT NOT NULL,
    day       TEXT NOT NULL, -- local date YYYY-MM-DD
    fired_utc INTEGER NOT NULL,
    PRIMARY KEY (job, day)
) WITHOUT ROWID;
