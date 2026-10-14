-- The draft of a dialog: what is known so far and which question waits for an answer.
-- AUTOINCREMENT: a replaced draft's id is never given to the next one, so the buttons
-- of an old card cannot act on a new draft.
CREATE TABLE pending (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id     INTEGER NOT NULL UNIQUE, -- one draft per chat, a new one replaces it
    intent_raw  TEXT NOT NULL,           -- JSON of intent.Intent
    asking      TEXT,                    -- field the current question is about, NULL while the card is shown
    llm_log_id  INTEGER,                 -- NULL for strict-syntax drafts
    updated_utc INTEGER NOT NULL         -- the 10-minute timeout counts from the last answer
);
