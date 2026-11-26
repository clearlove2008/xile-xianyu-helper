-- +goose Up
-- 工作流任务：关键词选品 + 多账号自动铺货的无人值守配置。
CREATE TABLE IF NOT EXISTS workflow_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword TEXT NOT NULL,
    search_cookie_id TEXT NOT NULL,
    publish_cookie_ids TEXT NOT NULL DEFAULT '[]',
    price_min_cents INTEGER NOT NULL DEFAULT 0,
    price_max_cents INTEGER NOT NULL DEFAULT 0,
    target_count INTEGER NOT NULL DEFAULT 1,
    interval_minutes INTEGER NOT NULL DEFAULT 120,
    publish_interval_seconds INTEGER NOT NULL DEFAULT 30,
    use_source_image INTEGER NOT NULL DEFAULT 0,
    price_cents INTEGER NOT NULL DEFAULT 0,
    quantity INTEGER NOT NULL DEFAULT 1,
    enabled INTEGER NOT NULL DEFAULT 0,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_workflow_tasks_due ON workflow_tasks(enabled, deleted_at, next_run_at);

-- 工作流发布去重：同一账号不重复发布同一源商品。
CREATE TABLE IF NOT EXISTS workflow_published_goods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cookie_id TEXT NOT NULL,
    source_item_id TEXT NOT NULL,
    task_id INTEGER NOT NULL DEFAULT 0,
    item_id TEXT,
    title TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(cookie_id, source_item_id)
);

-- 工作流运行日志：按任务记录选品、改写、生图、发布的过程与结果。
CREATE TABLE IF NOT EXISTS workflow_run_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_workflow_run_logs_task ON workflow_run_logs(task_id, id);

-- +goose Down
DROP TABLE IF EXISTS workflow_run_logs;
DROP TABLE IF EXISTS workflow_published_goods;
DROP TABLE IF EXISTS workflow_tasks;
