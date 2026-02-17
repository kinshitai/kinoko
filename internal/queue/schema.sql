CREATE TABLE IF NOT EXISTS queue_entries (
    session_id        TEXT PRIMARY KEY,
    library_id        TEXT NOT NULL,
    log_content_path  TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
    claimed_by        TEXT NOT NULL DEFAULT '',
    claimed_at        TIMESTAMP,
    retry_count       INTEGER NOT NULL DEFAULT 0,
    last_error        TEXT NOT NULL DEFAULT '',
    next_retry_at     TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS session_metadata (
    session_id          TEXT PRIMARY KEY,
    started_at          TIMESTAMP NOT NULL,
    ended_at            TIMESTAMP NOT NULL,
    duration_minutes    REAL NOT NULL,
    tool_call_count     INTEGER NOT NULL,
    error_count         INTEGER NOT NULL,
    message_count       INTEGER NOT NULL,
    error_rate          REAL NOT NULL,
    has_successful_exec BOOLEAN NOT NULL,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    agent_model         TEXT NOT NULL DEFAULT '',
    user_id             TEXT NOT NULL DEFAULT '',
    library_id          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_queue_entries_status ON queue_entries(status);
CREATE INDEX IF NOT EXISTS idx_queue_entries_next_retry ON queue_entries(next_retry_at);
