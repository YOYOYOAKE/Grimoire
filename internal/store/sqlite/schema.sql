PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS task_counter (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    value INTEGER NOT NULL
);

INSERT INTO task_counter (id, value)
VALUES (1, 0)
ON CONFLICT(id) DO NOTHING;

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    telegram_message_id INTEGER NOT NULL,
    direction TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL UNIQUE,
    chat_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    status_message_id INTEGER NOT NULL DEFAULT 0,
    prompt TEXT NOT NULL,
    shape TEXT NOT NULL,
    seed INTEGER NULL,
    status TEXT NOT NULL,
    stage TEXT NOT NULL,
    job_id TEXT NULL,
    error_text TEXT NULL,
    created_at DATETIME NOT NULL,
    started_at DATETIME NULL,
    finished_at DATETIME NULL,
    retry_of_task_id TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_stage_status ON tasks(stage, status);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);

CREATE TABLE IF NOT EXISTS gallery_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    task_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    caption TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_gallery_chat_message_id ON gallery_items(chat_id, message_id, id);

CREATE TABLE IF NOT EXISTS app_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);
