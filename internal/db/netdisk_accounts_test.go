package db

import (
	"context"
	"strings"
	"testing"
)

// TestNetdiskAccounts_CRUD 覆盖网盘账号的创建、加密读取、更新、列表与删除。
func TestNetdiskAccounts_CRUD(t *testing.T) {
	store, cleanup := newTestDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := store.Users.Create(ctx, "admin", "admin@e.com", "pw"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	admin, _ := store.Users.GetByUsername(ctx, "admin")

	repo := store.NetdiskAccounts
	if repo == nil {
		t.Fatal("NetdiskAccounts repository 未注入")
	}

	// 平台校验。
	if _, err := repo.Create(ctx, "x1", "xunlei", "cookie", admin.ID, ""); err == nil {
		t.Fatal("非法平台应报错")
	}

	cookie := "b-user-id=abc; __puus=xyz"
	if _, err := repo.Create(ctx, "q1", "quark", cookie, admin.ID, "主号"); err != nil {
		t.Fatalf("Create quark: %v", err)
	}
	if _, err := repo.Create(ctx, "b1", "baidu", "BDUSS=tok", admin.ID, ""); err != nil {
		t.Fatalf("Create baidu: %v", err)
	}
	// 重复 id 应报 ErrAlreadyExists。
	if _, err := repo.Create(ctx, "q1", "quark", cookie, admin.ID, ""); err != ErrAlreadyExists {
		t.Fatalf("重复创建应返回 ErrAlreadyExists，got %v", err)
	}

	// 读取应返回解密后的明文。
	got, err := repo.Get(ctx, "q1", admin.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != cookie || got.Platform != "quark" || got.Remark != "主号" || got.Status != "active" {
		t.Fatalf("Get 返回不匹配: %+v", got)
	}

	// 摘要不应包含 cookie 明文。
	list, err := repo.List(ctx, admin.ID, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List 应有 2 条，got %d", len(list))
	}
	for _, item := range list {
		raw := item.ID + item.Platform + item.Remark + item.Status
		if strings.Contains(raw, "BDUSS") || strings.Contains(raw, "b-user-id") {
			t.Fatalf("摘要泄露了 cookie: %+v", item)
		}
	}
	// 按平台过滤。
	quarks, err := repo.List(ctx, admin.ID, "quark")
	if err != nil || len(quarks) != 1 || quarks[0].Platform != "quark" {
		t.Fatalf("按平台过滤失败: %+v err=%v", quarks, err)
	}

	// 更新 cookie 与元信息。
	if err := repo.UpdateValue(ctx, "q1", "b-user-id=new", admin.ID); err != nil {
		t.Fatalf("UpdateValue: %v", err)
	}
	if err := repo.UpdateMeta(ctx, "q1", "改名", "disabled", admin.ID); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}
	got, _ = repo.Get(ctx, "q1", admin.ID)
	if got.Value != "b-user-id=new" || got.Remark != "改名" || got.Status != "disabled" {
		t.Fatalf("更新后字段不匹配: %+v", got)
	}

	// 非法状态拒绝。
	if err := repo.UpdateMeta(ctx, "q1", "", "bad", admin.ID); err == nil {
		t.Fatal("非法状态应报错")
	}

	// 越权访问：另一个用户看不到。
	if _, err := repo.Get(ctx, "q1", admin.ID+1); err != ErrNotFound {
		t.Fatalf("越权 Get 应返回 ErrNotFound，got %v", err)
	}
	if err := repo.Delete(ctx, "q1", admin.ID+1); err != ErrNotFound {
		t.Fatalf("越权 Delete 应返回 ErrNotFound，got %v", err)
	}

	// 删除。
	if err := repo.Delete(ctx, "q1", admin.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "q1", admin.ID); err != ErrNotFound {
		t.Fatalf("删除后应返回 ErrNotFound，got %v", err)
	}
}

// TestNetdiskAccounts_EncryptionAtRest 验证配置密钥后数据库中存的是密文而非明文。
func TestNetdiskAccounts_EncryptionAtRest(t *testing.T) {
	t.Setenv("XIANYU_DATA_KEY", "netdisk-account-test-key")
	store, cleanup := newTestDB(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := store.Users.Create(ctx, "admin", "admin@e.com", "pw"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	admin, _ := store.Users.GetByUsername(ctx, "admin")

	repo := store.NetdiskAccounts
	cookie := "BDUSS=super-secret-value"
	if _, err := repo.Create(ctx, "b1", "baidu", cookie, admin.ID, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stored string
	if err := store.DB.QueryRowContext(ctx, `SELECT value FROM netdisk_accounts WHERE id=?`, "b1").Scan(&stored); err != nil {
		t.Fatalf("query raw value: %v", err)
	}
	if stored == cookie || strings.Contains(stored, "super-secret-value") {
		t.Fatalf("数据库存了明文 cookie: %s", stored)
	}

	// 同一密钥下读取应还原明文。
	got, err := repo.Get(ctx, "b1", admin.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != cookie {
		t.Fatalf("解密后应还原原值，got %q", got.Value)
	}
}
