-- +goose Up
-- 网盘转存账号：夸克/百度 cookie 独立存储，与闲鱼账号体系隔离。
CREATE TABLE IF NOT EXISTS netdisk_accounts (
    id TEXT PRIMARY KEY,
    platform TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    value TEXT NOT NULL,
    remark TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_netdisk_accounts_user_platform ON netdisk_accounts(user_id, platform);

-- +goose Down
DROP TABLE IF EXISTS netdisk_accounts;
