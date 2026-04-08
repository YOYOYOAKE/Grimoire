PRAGMA defer_foreign_keys = ON;

ALTER TABLE tasks RENAME TO tasks_old;

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    source_task TEXT,
    request TEXT NOT NULL CHECK (length(trim(request)) > 0),
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

INSERT INTO tasks(
    id,
    user_id,
    session_id,
    source_task,
    request,
    image,
    status,
    error,
    timeline,
    context,
    progress_message_id,
    result_message_id
)
SELECT
    id,
    user_id,
    session_id,
    source_task,
    request,
    image,
    status,
    error,
    timeline,
    CASE
        WHEN prompt IS NOT NULL AND length(trim(prompt)) > 0 THEN
            json_set(
                json_set(context, '$.version', 2),
                '$.prompt_bundle',
                json_object(
                    'prompt', trim(prompt),
                    'negative_prompt', '',
                    'characters', json('[]')
                )
            )
        ELSE
            json_set(context, '$.version', 2)
    END,
    progress_message_id,
    result_message_id
FROM tasks_old;

DROP TABLE tasks_old;

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_session_id ON tasks(session_id);
CREATE INDEX idx_tasks_source_task ON tasks(source_task);
CREATE INDEX idx_tasks_recoverable_status
    ON tasks(status)
    WHERE status IN ('queued', 'translating', 'drawing');
