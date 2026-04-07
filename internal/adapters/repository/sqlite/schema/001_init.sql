CREATE TABLE users (
    telegram_id TEXT PRIMARY KEY,
    role TEXT NOT NULL CHECK (role IN ('normal', 'banned')),
    preference TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    length INTEGER NOT NULL DEFAULT 0 CHECK (length >= 0),
    summary TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (user_id) REFERENCES users(telegram_id) ON DELETE CASCADE
);

CREATE TABLE session_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_session_messages_session_created_at
    ON session_messages(session_id, created_at DESC, id DESC);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    source_task TEXT,
    request TEXT NOT NULL CHECK (length(trim(request)) > 0),
    prompt TEXT,
    image TEXT,
    status TEXT NOT NULL CHECK (status IN ('queued', 'translating', 'drawing', 'completed', 'failed', 'stopped')),
    error TEXT,
    timeline TEXT NOT NULL,
    context TEXT NOT NULL,
    progress_message_id TEXT,
    result_message_id TEXT,
    FOREIGN KEY (user_id) REFERENCES users(telegram_id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (source_task) REFERENCES tasks(id) ON DELETE SET NULL
);

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_session_id ON tasks(session_id);
CREATE INDEX idx_tasks_source_task ON tasks(source_task);
CREATE INDEX idx_tasks_recoverable_status
    ON tasks(status)
    WHERE status IN ('queued', 'translating', 'drawing');
