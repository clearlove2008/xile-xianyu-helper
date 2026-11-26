package relister

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

// fakePlatform 记录巡检期间发出的平台调用，用于断言筛选与上架行为。
type fakePlatform struct {
	groups     []mtop.ItemGroup
	items      map[string][]mtop.ItemListItem
	reshelf    []string
	quantities []int
	// reshelfErr 非空时所有上架请求都失败。
	reshelfErr error
	// rejectMultiInventory 开启时 quantity>1 的上架请求返回平台多库存拒绝错误。
	rejectMultiInventory bool
	// onSale 是在售列表（兜底巡检使用）；nil 时返回空列表。
	onSale []mtop.ItemListItem
	// editQuantity 模拟各商品的编辑详情库存；缺省为 0（售罄）。
	editQuantity map[string]int
	// editDetailErr 非空时读取编辑详情失败。
	editDetailErr error
}

func (f *fakePlatform) FetchItemGroups(context.Context, string) ([]mtop.ItemGroup, error) {
	return f.groups, nil
}

func (f *fakePlatform) FetchItemsInGroup(_ context.Context, _, groupID, _ string, _, _ int) (*mtop.ItemListResult, error) {
	return &mtop.ItemListResult{Items: f.items[groupID], PageNumber: 1, TotalPages: 1}, nil
}

func (f *fakePlatform) FetchItemsPage(_ context.Context, _ string, _, _ int) (*mtop.ItemListResult, error) {
	return &mtop.ItemListResult{Items: f.onSale, PageNumber: 1, TotalPages: 1}, nil
}

func (f *fakePlatform) FetchItemEditDetail(_ context.Context, _, itemID string) (*mtop.ItemEditDetail, error) {
	if f.editDetailErr != nil {
		return nil, f.editDetailErr
	}
	return &mtop.ItemEditDetail{ItemID: itemID, Quantity: f.editQuantity[itemID], Title: "商品" + itemID}, nil
}

func (f *fakePlatform) ReshelfItem(_ context.Context, _, itemID string, quantity int) (*mtop.ReshelfResult, error) {
	// quantities 记录所有调用（含被拒绝的），便于断言调用序列。
	f.quantities = append(f.quantities, quantity)
	if f.rejectMultiInventory && quantity > 1 {
		return nil, errors.New("mtop.idle.pc.idleitem.edit 返回非成功（业务错误）；平台原因：FAIL_BIZ_ITEM_ERROR_MULTI_INVENTORY_ITEM_CAN_NOT_PUBLISH::亲，您不能发布多库存宝贝哦")
	}
	if f.reshelfErr != nil {
		return nil, f.reshelfErr
	}
	f.reshelf = append(f.reshelf, itemID)
	return &mtop.ReshelfResult{ItemID: itemID, Quantity: quantity}, nil
}

// newTestScanner 构造一个使用临时库的巡检器，并写入一个账号凭证。
func newTestScanner(t *testing.T, platform Platform) *Scanner {
	t.Helper()
	// database 是临时 SQLite 库，db.Open 会执行全部迁移。
	database, dialect, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "relister.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	store := db.NewStore(database, dialect)
	if _, err := database.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES (1,'u','u@e.com','x')`); err != nil {
		t.Fatalf("写入用户失败: %v", err)
	}
	if err := store.Cookies.Save(context.Background(), "acc1", "unb=1; _m_h5_tk=token_1", 1); err != nil {
		t.Fatalf("写入凭证失败: %v", err)
	}
	// 巡检只处理有商品记录的账号，这里补一条商品记录让账号进入候选集。
	if _, err := database.Exec(`INSERT INTO item_info (cookie_id, item_id, item_title) VALUES ('acc1','item-seed','占位商品')`); err != nil {
		t.Fatalf("写入商品失败: %v", err)
	}
	return New(store, platform, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestScanOnceReshelvesEndedItems(t *testing.T) {
	platform := &fakePlatform{
		groups: []mtop.ItemGroup{
			{GroupID: "on", GroupName: "在售", GroupType: 0, ItemNumber: 3},
			{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 1},
		},
		items: map[string][]mtop.ItemListItem{
			"sold": {{ID: "item-sold", Title: "卖完的商品"}},
		},
	}
	scanner := newTestScanner(t, platform)

	outcomes := scanner.ScanOnce(context.Background())

	if len(platform.reshelf) != 1 || platform.reshelf[0] != "item-sold" {
		t.Fatalf("应只重新上架已售出商品，实际: %v", platform.reshelf)
	}
	if len(outcomes) != 1 || outcomes[0].Action != "reshelf" {
		t.Fatalf("处理结果不符合预期: %+v", outcomes)
	}
	if scanner.Status().TotalReshelf != 1 {
		t.Fatalf("累计成功次数应为 1，实际 %d", scanner.Status().TotalReshelf)
	}
}

// 冷却期内同一商品不会被重复上架，避免接口异常时形成死循环。
func TestScanOnceSkipsCoolingItem(t *testing.T) {
	platform := &fakePlatform{
		groups: []mtop.ItemGroup{{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 1}},
		items:  map[string][]mtop.ItemListItem{"sold": {{ID: "item-sold"}}},
	}
	scanner := newTestScanner(t, platform)

	scanner.ScanOnce(context.Background())
	scanner.ScanOnce(context.Background())

	if len(platform.reshelf) != 1 {
		t.Fatalf("冷却期内不应重复上架，实际 %d 次", len(platform.reshelf))
	}
}

// 仍有未完成订单的商品必须跳过，防止同一件虚拟商品被重复出售。
func TestScanOnceSkipsItemWithActiveOrder(t *testing.T) {
	platform := &fakePlatform{
		groups: []mtop.ItemGroup{{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 1}},
		items:  map[string][]mtop.ItemListItem{"sold": {{ID: "item-pending"}}},
	}
	scanner := newTestScanner(t, platform)
	if _, err := scanner.store.DB.Exec(`INSERT INTO orders (order_id, item_id, order_status, cookie_id) VALUES ('o1','item-pending','paid','acc1')`); err != nil {
		t.Fatalf("写入订单失败: %v", err)
	}

	outcomes := scanner.ScanOnce(context.Background())

	if len(platform.reshelf) != 0 {
		t.Fatalf("有未完成订单时不应上架，实际: %v", platform.reshelf)
	}
	if len(outcomes) != 1 || outcomes[0].Action != "skipped_active_order" {
		t.Fatalf("应记录跳过原因，实际: %+v", outcomes)
	}
}

// 总开关关闭时整轮静默返回，不向平台发请求。
func TestScanOnceRespectsDisableSwitch(t *testing.T) {
	platform := &fakePlatform{
		groups: []mtop.ItemGroup{{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 1}},
		items:  map[string][]mtop.ItemListItem{"sold": {{ID: "item-sold"}}},
	}
	scanner := newTestScanner(t, platform)
	if err := scanner.store.Settings.Set(context.Background(), settingEnabled, "false"); err != nil {
		t.Fatalf("写入开关失败: %v", err)
	}

	if outcomes := scanner.ScanOnce(context.Background()); len(outcomes) != 0 {
		t.Fatalf("关闭时不应有处理结果: %+v", outcomes)
	}
	if len(platform.reshelf) != 0 {
		t.Fatalf("关闭时不应上架: %v", platform.reshelf)
	}
}

// 上架失败要落成 failed 明细，而不是中断整轮巡检。
func TestScanOnceRecordsFailure(t *testing.T) {
	platform := &fakePlatform{
		groups: []mtop.ItemGroup{{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 2}},
		items: map[string][]mtop.ItemListItem{"sold": {
			{ID: "item-a"},
			{ID: "item-b"},
		}},
		reshelfErr: errors.New("平台拒绝"),
	}
	scanner := newTestScanner(t, platform)

	outcomes := scanner.ScanOnce(context.Background())

	if len(outcomes) != 2 {
		t.Fatalf("两个商品都应记录结果，实际 %d", len(outcomes))
	}
	for _, outcome := range outcomes {
		if outcome.Action != "failed" {
			t.Fatalf("失败应记录 failed，实际 %+v", outcome)
		}
	}
}

// 只处理售罄类分组；人工下架的「已下架」默认不动。
func TestGroupEligible(t *testing.T) {
	scanner := newTestScanner(t, &fakePlatform{})
	ctx := context.Background()
	cases := []struct {
		group mtop.ItemGroup
		want  bool
	}{
		{mtop.ItemGroup{GroupName: "已售出", GroupType: 1, ItemNumber: 1}, true},
		{mtop.ItemGroup{GroupName: "在售", GroupType: 0, ItemNumber: 3}, false},
		{mtop.ItemGroup{GroupName: "已下架", ItemNumber: 1}, false},
		{mtop.ItemGroup{GroupName: "综合", GroupType: 0, ItemNumber: 9}, false},
		{mtop.ItemGroup{GroupName: "已售出", GroupType: 1, ItemNumber: 0}, false},
	}
	for _, c := range cases {
		if got := scanner.groupEligible(ctx, c.group); got != c.want {
			t.Errorf("groupEligible(%+v) = %v, want %v", c.group, got, c.want)
		}
	}
	if err := scanner.store.Settings.Set(ctx, settingIncludeOffline, "true"); err != nil {
		t.Fatalf("写入设置失败: %v", err)
	}
	if !scanner.groupEligible(ctx, mtop.ItemGroup{GroupName: "已下架", ItemNumber: 1}) {
		t.Fatal("显式开启后「已下架」应参与巡检")
	}
}

// 设置项缺失时使用默认周期与库存，保证开箱即用。
func TestDefaults(t *testing.T) {
	scanner := newTestScanner(t, &fakePlatform{})
	ctx := context.Background()
	if got := scanner.interval(ctx); got != defaultInterval {
		t.Fatalf("默认周期应为 %s，实际 %s", defaultInterval, got)
	}
	if got := scanner.quantity(ctx); got != defaultQuantity {
		t.Fatalf("默认库存应为 %d，实际 %d", defaultQuantity, got)
	}
	if !scanner.enabled(ctx) {
		t.Fatal("默认应开启巡检")
	}
	_ = time.Now()
}

// 平台按账号拒绝多库存时，售罄商品应降级按 1 件上架，且账号被记住后不再试多件。
func TestScanOnceDegradesMultiInventoryDeniedAccount(t *testing.T) {
	platform := &fakePlatform{
		groups:               []mtop.ItemGroup{{GroupID: "sold", GroupName: "已售出", GroupType: 1, ItemNumber: 1}},
		items:                map[string][]mtop.ItemListItem{"sold": {{ID: "item-sold"}}},
		rejectMultiInventory: true,
	}
	scanner := newTestScanner(t, platform)

	outcomes := scanner.ScanOnce(context.Background())

	// 第一轮：先按默认 5 件被拒，随后降级 1 件成功。
	if len(outcomes) != 1 || outcomes[0].Action != "reshelf" || !strings.Contains(outcomes[0].Detail, "降级") {
		t.Fatalf("应降级上架并注明原因，实际: %+v", outcomes)
	}
	if len(platform.quantities) != 2 || platform.quantities[0] != 5 || platform.quantities[1] != 1 {
		t.Fatalf("调用序列应为 [5,1]，实际: %v", platform.quantities)
	}

	// 清空冷却记录模拟商品再次售罄；账号已被记住，直接按 1 件提交，不再浪费一次注定失败的请求。
	scanner.mu.Lock()
	scanner.attempts = make(map[string]attempt)
	scanner.mu.Unlock()
	scanner.ScanOnce(context.Background())
	if len(platform.quantities) != 3 || platform.quantities[2] != 1 {
		t.Fatalf("记住账号后应直接按 1 件提交，调用序列: %v", platform.quantities)
	}
}

// item.list 不返回分组的账号走兜底巡检：本地记录不在售且库存为 0 的商品重新上架；
// 仍有库存（人工下架）的商品不动；在售商品不处理。
func TestScanOnceGrouplessAccountFallback(t *testing.T) {
	platform := &fakePlatform{
		groups:       nil,                                    // 模拟喜乐API：平台不返回 itemGroupList
		onSale:       []mtop.ItemListItem{{ID: "item-seed"}}, // 占位商品仍在售
		editQuantity: map[string]int{"item-manual": 3},
	}
	scanner := newTestScanner(t, platform)
	// item-sold 已卖完：先全量上架卖掉，同步把本地行软删。
	if _, err := scanner.store.DB.Exec(`INSERT INTO item_info (cookie_id, item_id, item_title, deleted_at) VALUES ('acc1','item-sold','卖完的商品', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("写入售罄商品失败: %v", err)
	}
	if _, err := scanner.store.DB.Exec(`INSERT INTO item_info (cookie_id, item_id, item_title) VALUES ('acc1','item-manual','人工下架的商品')`); err != nil {
		t.Fatalf("写入人工下架商品失败: %v", err)
	}

	outcomes := scanner.ScanOnce(context.Background())

	if len(platform.reshelf) != 1 || platform.reshelf[0] != "item-sold" {
		t.Fatalf("兜底巡检应只上架售罄商品，实际: %v", platform.reshelf)
	}
	var sold, manual int
	for _, o := range outcomes {
		switch o.ItemID {
		case "item-sold":
			sold++
			if o.Action != "reshelf" || o.GroupName != "已售出(兜底)" {
				t.Fatalf("售罄商品处理结果不符合预期: %+v", o)
			}
		case "item-manual":
			manual++
		}
	}
	if sold != 1 {
		t.Fatalf("售罄商品应有一条处理记录，实际 %d", sold)
	}
	if manual != 0 {
		t.Fatalf("人工下架（仍有库存）的商品不应有处理记录，实际 %d", manual)
	}
}

// 商品行全部被同步软删的账号仍要参与巡检：卖完恰好在此时最需要自动补货。
func TestAccountsIncludeSoftDeletedItems(t *testing.T) {
	platform := &fakePlatform{
		onSale:       []mtop.ItemListItem{{ID: "item-sold"}},
		editQuantity: map[string]int{"item-sold": 0, "item-seed": 2},
	}
	scanner := newTestScanner(t, platform)
	// 软删全部行，模拟「卖完 → 全量同步删本地行」的状态。
	if _, err := scanner.store.DB.Exec(`UPDATE item_info SET deleted_at=CURRENT_TIMESTAMP WHERE cookie_id='acc1'`); err != nil {
		t.Fatalf("软删商品失败: %v", err)
	}

	outcomes := scanner.ScanOnce(context.Background())

	// item-seed 仍有库存（人工下架语义）不动；item-sold 售罄但在售，跳过。
	if len(platform.reshelf) != 0 {
		t.Fatalf("无售罄候选时不应上架，实际: %v", platform.reshelf)
	}
	if len(outcomes) != 0 {
		t.Fatalf("不应产生处理结果: %+v", outcomes)
	}
	// 账号必须仍在候选集内（否则永远不会再被巡检）。
	accounts, err := scanner.accounts(context.Background())
	if err != nil {
		t.Fatalf("读取账号失败: %v", err)
	}
	found := false
	for _, a := range accounts {
		if a.id == "acc1" {
			found = true
		}
	}
	if !found {
		t.Fatal("行全部软删的账号不应退出巡检候选集")
	}
}
