package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// netdisk cookie 加密 scope。与闲鱼 cookie 使用不同 scope，避免密文串用。
const netdiskCookieScope = "netdisk-cookie"

// NetdiskAccount 是网盘转存账号的存储视图。Value 恒为解密后的明文 cookie，
// 明文只在该 repository 内部短暂存在，绝不进入 HTTP 响应。
type NetdiskAccount struct {
	ID         string
	Platform   string
	UserID     int64
	Value      string
	Remark     string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastUsedAt *time.Time
}

// NetdiskAccountSummary 是允许返回给前端的非敏感账号视图。
type NetdiskAccountSummary struct {
	ID         string     `json:"id"`
	Platform   string     `json:"platform"`
	Remark     string     `json:"remark"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// NetdiskAccounts 读写 netdisk_accounts 表。
type NetdiskAccounts struct {
	DB      *sql.DB
	Dialect Dialect
	codec   *secretCodec
}

// normalizePlatform 规整平台标识，非法平台返回错误。
func normalizePlatform(platform string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(platform))
	if p != "quark" && p != "baidu" {
		return "", fmt.Errorf("不支持的网盘平台: %s", platform)
	}
	return p, nil
}

// List 返回某用户下全部网盘账号的非敏感摘要，可按平台过滤。platform 为空表示不过滤。
func (n *NetdiskAccounts) List(ctx context.Context, userID int64, platform string) ([]NetdiskAccountSummary, error) {
	if userID <= 0 {
		return nil, errors.New("user_id 不能为空")
	}
	query := `SELECT id, platform, remark, status, created_at, updated_at, last_used_at FROM netdisk_accounts WHERE user_id=?`
	args := []any{userID}
	if p := strings.TrimSpace(platform); p != "" {
		query += ` AND platform=?`
		args = append(args, p)
	}
	query += ` ORDER BY platform, created_at`
	rows, err := n.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NetdiskAccountSummary, 0)
	for rows.Next() {
		var item NetdiskAccountSummary
		var lastUsed sql.NullTime
		if err := rows.Scan(&item.ID, &item.Platform, &item.Remark, &item.Status, &item.CreatedAt, &item.UpdatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			item.LastUsedAt = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Get 读取单个账号的明文 cookie。调用方必须在当前用例内使用，不得写入响应。
func (n *NetdiskAccounts) Get(ctx context.Context, id string, userID int64) (NetdiskAccount, error) {
	if id == "" || userID <= 0 {
		return NetdiskAccount{}, errors.New("账号 ID 和 user_id 不能为空")
	}
	var item NetdiskAccount
	var lastUsed sql.NullTime
	err := n.DB.QueryRowContext(ctx,
		`SELECT id, platform, user_id, value, remark, status, created_at, updated_at, last_used_at
		 FROM netdisk_accounts WHERE id=? AND user_id=?`, id, userID).
		Scan(&item.ID, &item.Platform, &item.UserID, &item.Value, &item.Remark, &item.Status, &item.CreatedAt, &item.UpdatedAt, &lastUsed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NetdiskAccount{}, ErrNotFound
		}
		return NetdiskAccount{}, err
	}
	plain, err := n.codec.decrypt(netdiskCookieScope, id, item.Value)
	if err != nil {
		return NetdiskAccount{}, err
	}
	item.Value = plain
	if lastUsed.Valid {
		t := lastUsed.Time
		item.LastUsedAt = &t
	}
	return item, nil
}

// Create 创建新账号。id 由调用方生成（如 uuid）。
func (n *NetdiskAccounts) Create(ctx context.Context, id, platform, cookie string, userID int64, remark string) (NetdiskAccountSummary, error) {
	if id == "" || userID <= 0 {
		return NetdiskAccountSummary{}, errors.New("账号 ID 和 user_id 不能为空")
	}
	p, err := normalizePlatform(platform)
	if err != nil {
		return NetdiskAccountSummary{}, err
	}
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return NetdiskAccountSummary{}, errors.New("cookie 不能为空")
	}
	encrypted, err := n.codec.encrypt(netdiskCookieScope, id, cookie)
	if err != nil {
		return NetdiskAccountSummary{}, err
	}
	res, err := n.DB.ExecContext(ctx,
		dialectInsertIgnorePrefix(n.Dialect)+` INTO netdisk_accounts (id, platform, user_id, value, remark, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`+dialectInsertIgnore(n.Dialect, []string{"id"}),
		id, p, userID, encrypted, remark)
	if err != nil {
		return NetdiskAccountSummary{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return NetdiskAccountSummary{}, err
	}
	if affected == 0 {
		return NetdiskAccountSummary{}, ErrAlreadyExists
	}
	return NetdiskAccountSummary{ID: id, Platform: p, Remark: remark, Status: "active"}, nil
}

// UpdateValue 更新账号 cookie（加密写入）。
func (n *NetdiskAccounts) UpdateValue(ctx context.Context, id, cookie string, userID int64) error {
	if id == "" || userID <= 0 {
		return errors.New("账号 ID 和 user_id 不能为空")
	}
	encrypted, err := n.codec.encrypt(netdiskCookieScope, id, strings.TrimSpace(cookie))
	if err != nil {
		return err
	}
	res, err := n.DB.ExecContext(ctx,
		`UPDATE netdisk_accounts SET value=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`,
		encrypted, id, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateMeta 更新备注与状态。status 允许 active/disabled。
func (n *NetdiskAccounts) UpdateMeta(ctx context.Context, id, remark, status string, userID int64) error {
	if id == "" || userID <= 0 {
		return errors.New("账号 ID 和 user_id 不能为空")
	}
	if status != "" && status != "active" && status != "disabled" {
		return errors.New("账号状态只能为 active 或 disabled")
	}
	setParts := make([]string, 0, 3)
	args := make([]any, 0, 4)
	if remark != "" {
		setParts = append(setParts, "remark=?")
		args = append(args, remark)
	}
	if status != "" {
		setParts = append(setParts, "status=?")
		args = append(args, status)
	}
	if len(setParts) == 0 {
		return nil
	}
	setParts = append(setParts, "updated_at=CURRENT_TIMESTAMP")
	args = append(args, id, userID)
	query := `UPDATE netdisk_accounts SET ` + strings.Join(setParts, ", ") + ` WHERE id=? AND user_id=?`
	res, err := n.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsed 更新最近使用时间。
func (n *NetdiskAccounts) TouchLastUsed(ctx context.Context, id string) {
	_, _ = n.DB.ExecContext(ctx, `UPDATE netdisk_accounts SET last_used_at=CURRENT_TIMESTAMP WHERE id=?`, id)
}

// Delete 删除账号。
func (n *NetdiskAccounts) Delete(ctx context.Context, id string, userID int64) error {
	if id == "" || userID <= 0 {
		return errors.New("账号 ID 和 user_id 不能为空")
	}
	res, err := n.DB.ExecContext(ctx, `DELETE FROM netdisk_accounts WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
