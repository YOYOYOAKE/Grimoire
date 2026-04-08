PRAGMA defer_foreign_keys = ON;

ALTER TABLE sessions RENAME TO sessions_old;
ALTER TABLE session_messages RENAME TO session_messages_old;
ALTER TABLE tasks RENAME TO tasks_old;

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
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

INSERT INTO sessions(id, user_id, length, summary)
SELECT id, user_id, length, summary FROM sessions_old;

CREATE TABLE active_sessions (
    user_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL UNIQUE,
    FOREIGN KEY (user_id) REFERENCES users(telegram_id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

INSERT INTO active_sessions(user_id, session_id)
SELECT user_id, id FROM sessions;

INSERT INTO session_messages(id, session_id, role, content, created_at)
SELECT id, session_id, role, content, created_at FROM session_messages_old;

INSERT INTO tasks(id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id)
SELECT id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id
FROM tasks_old;

DROP TABLE tasks_old;
DROP TABLE session_messages_old;
DROP TABLE sessions_old;

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_session_messages_session_created_at
    ON session_messages(session_id, created_at DESC, id DESC);
CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_session_id ON tasks(session_id);
CREATE INDEX idx_tasks_source_task ON tasks(source_task);
CREATE INDEX idx_tasks_recoverable_status
    ON tasks(status)
    WHERE status IN ('queued', 'translating', 'drawing');
