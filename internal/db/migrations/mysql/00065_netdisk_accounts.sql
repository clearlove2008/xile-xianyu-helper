-- +goose Up
-- 网盘转存账号：夸克/百度 cookie 独立存储，与闲鱼账号体系隔离。
CREATE TABLE IF NOT EXISTS netdisk_accounts (
    id VARCHAR(64) PRIMARY KEY,
    platform VARCHAR(16) NOT NULL,
    user_id BIGINT NOT NULL,
    value TEXT NOT NULL,
    remark VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    KEY idx_netdisk_accounts_user_platform (user_id, platform),
    CONSTRAINT fk_netdisk_accounts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS netdisk_accounts;
