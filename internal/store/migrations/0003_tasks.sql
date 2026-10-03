CREATE TABLE tasks (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    remind_utc  INTEGER NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open', -- open|done|cancelled
    reminded    INTEGER NOT NULL DEFAULT 0,
    created_utc INTEGER NOT NULL
);
CREATE INDEX idx_tasks_due ON tasks(remind_utc) WHERE status = 'open';

CREATE TABLE notes (
    id     INTEGER PRIMARY KEY,
    text   TEXT NOT NULL,
    at_utc INTEGER NOT NULL,
    done   INTEGER NOT NULL DEFAULT 0
);
