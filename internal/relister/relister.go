// Package relister 负责「售罄下架自动重新上架」的定时巡检。
//
// 闲鱼没有独立的上架接口，重新上架等价于把商品编辑详情原样回填一次
// mtop.idle.pc.idleitem.edit，因此这里的巡检只做三件事：
// 找到已结束分组里的商品、筛掉仍在履约中的订单、按配置的库存重新上架。
package relister

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

// 系统设置键；未配置时使用代码内默认值，保存后下一轮巡检即生效。
const (
	// settingEnabled 控制巡检总开关，取 "false"/"0"/"off" 时关闭。
	settingEnabled = "reshelf_enabled"
	// settingIntervalMinutes 控制巡检周期（分钟），默认 2。
	settingIntervalMinutes = "reshelf_interval_minutes"
	// settingQuantity 控制重新上架时提交的库存数量，默认 1。
	settingQuantity = "reshelf_quantity"
	// settingMaxPerScan 控制单轮最多重新上架的商品数，默认 5。
	settingMaxPerScan = "reshelf_max_per_scan"
	// settingIncludeOffline 控制是否连「已下架」分组的商品一起上架，默认关闭，
	// 避免把人工主动下架的商品又自动挂回去。
	settingIncludeOffline = "reshelf_include_offline"
)

// 巡检默认参数。
const (
	defaultInterval   = 2 * time.Minute
	defaultQuantity   = 5
	defaultMaxPerScan = 5
	// failCooldown 是同一商品连续失败后的退避时间，避免平台持续拒绝时反复打接口。
	failCooldown = 30 * time.Minute
	// successCooldown 是同一商品成功上架后的最小间隔，防止一轮之内反复上架同一件商品。
	successCooldown = time.Minute
	// scanPageSize 是读取已结束分组时的单页条数。
	scanPageSize = 20
	// scanMaxPages 是单个分组单轮最多翻页数，避免历史商品很多时拖长扫描。
	scanMaxPages = 5
	// itemGap 是同一账号连续重上架之间的间隔，压低触发风控的概率。
	itemGap = 2 * time.Second
	// accountGap 是两个账号之间的间隔。
	accountGap = time.Second
)

// Platform 是巡检需要的平台能力；由 mtop 客户端实现，测试可注入替身。
type Platform interface {
	// FetchItemGroups 返回商品分组清单。
	FetchItemGroups(ctx context.Context, cookies string) ([]mtop.ItemGroup, error)
	// FetchItemsInGroup 返回指定分组的一页商品。
	FetchItemsInGroup(ctx context.Context, cookies, groupID, groupName string, pageNumber, pageSize int) (*mtop.ItemListResult, error)
	// FetchItemsPage 返回在售商品的一页；无分组账号的兜底巡检用它判断哪些商品仍在售。
	FetchItemsPage(ctx context.Context, cookies string, pageNumber, pageSize int) (*mtop.ItemListResult, error)
	// FetchItemEditDetail 返回商品编辑详情；兜底巡检用它区分售罄与人工下架。
	FetchItemEditDetail(ctx context.Context, cookies, itemID string) (*mtop.ItemEditDetail, error)
	// ReshelfItem 重新上架指定商品。
	ReshelfItem(ctx context.Context, cookies, itemID string, quantity int) (*mtop.ReshelfResult, error)
}

// Outcome 是单个商品的处理结果。
type Outcome struct {
	// AccountID 是商品所属账号。
	AccountID string
	// ItemID 是平台商品标识。
	ItemID string
	// Title 是商品标题，便于人工核对。
	Title string
	// GroupName 是商品所在分组（已售出/已下架等）。
	GroupName string
	// Action 描述本轮动作：reshelf、skipped_active_order、failed。
	Action string
	// Detail 是失败原因或跳过原因。
	Detail string
	// At 是动作发生时间。
	At time.Time
}

// Status 是巡检的整体运行状态，供日志与排查使用。
type Status struct {
	// LastRun 是最近一轮巡检的开始时间。
	LastRun time.Time
	// LastDuration 是最近一轮巡检耗时。
	LastDuration time.Duration
	// LastOutcomes 是最近一轮的处理明细。
	LastOutcomes []Outcome
	// TotalReshelf 是进程启动以来成功重新上架的次数。
	TotalReshelf int
}

// Scanner 周期巡检所有账号的已结束商品并重新上架。
type Scanner struct {
	// store 提供账号凭证与设置读取。
	store *db.Store
	// platform 是平台调用入口。
	platform Platform
	// logger 记录巡检结果。
	logger *slog.Logger
	// mu 保护运行状态与冷却表。
	mu sync.Mutex
	// attempts 记录每个商品上次尝试重新上架的时间与结果。
	attempts map[string]attempt
	// multiInventoryDenied 记录被平台拒绝多库存的账号（账号级限制，发布与编辑都拒）；
	// 命中后该账号直接按 1 件上架，避免每轮都打一次注定失败的请求。进程内状态，重启后重新探测。
	multiInventoryDenied map[string]bool
	// status 保存最近一轮运行状态。
	status Status
	// totalReshelf 是累计成功次数。
	totalReshelf int
}

// New 构造巡检器；store 或 platform 为空时返回 nil，由调用方跳过登记。
func New(store *db.Store, platform Platform, logger *slog.Logger) *Scanner {
	if store == nil || platform == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		store:                store,
		platform:             platform,
		logger:               logger,
		attempts:             make(map[string]attempt),
		multiInventoryDenied: make(map[string]bool),
	}
}

// attempt 是单个商品的最近一次上架尝试结果，用于区分成功与失败的退避节奏。
type attempt struct {
	// at 是尝试发生时间。
	at time.Time
	// ok 表示该次尝试是否成功。
	ok bool
}

// Run 按配置周期执行巡检，直到 ctx 取消。调用方应在独立 goroutine 中启动。
func (s *Scanner) Run(ctx context.Context) {
	if s == nil || ctx == nil {
		return
	}
	// 启动后先等一个周期，避免和服务启动阶段的账号连接、订单同步抢占平台配额。
	timer := time.NewTimer(s.interval(ctx))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.ScanOnce(ctx)
			timer.Reset(s.interval(ctx))
		}
	}
}

// ScanOnce 执行一轮巡检并返回处理明细。
func (s *Scanner) ScanOnce(ctx context.Context) []Outcome {
	if s == nil {
		return nil
	}
	// enabled 是总开关；关闭时静默返回。
	if !s.enabled(ctx) {
		return nil
	}
	started := time.Now()
	// quantity、maxPerScan 是本轮使用的参数。
	quantity, maxPerScan := s.quantity(ctx), s.maxPerScan(ctx)
	// accounts 是本次要巡检的账号与凭证。
	accounts, err := s.accounts(ctx)
	if err != nil {
		s.logger.Warn("重新上架巡检读取账号失败", "err", err)
		return nil
	}

	outcomes := make([]Outcome, 0, 8)
	// reshelfCount 累计本轮成功次数，达到上限即停止。
	reshelfCount := 0
	for i, account := range accounts {
		if ctx.Err() != nil || reshelfCount >= maxPerScan {
			break
		}
		if i > 0 {
			// 账号之间留一点间隔，避免同一时刻并发打平台接口。
			if !sleepCtx(ctx, accountGap) {
				break
			}
		}
		groups, err := s.platform.FetchItemGroups(ctx, account.cookie)
		if err != nil {
			s.logger.Warn("重新上架巡检读取商品分组失败", "account", account.id, "err", err)
			continue
		}
		if len(groups) == 0 {
			// 部分账号（2026-11-17 实测喜乐API）的 item.list 不返回 itemGroupList，
			// 分组巡检拿不到「已售出」分组；改用本地商品记录 + 在售列表差集兜底。
			outcomes = append(outcomes, s.scanGrouplessAccount(ctx, account, quantity, &reshelfCount, maxPerScan)...)
			continue
		}
		for _, group := range groups {
			if reshelfCount >= maxPerScan || ctx.Err() != nil {
				break
			}
			if !s.groupEligible(ctx, group) {
				continue
			}
			items, err := s.endedItems(ctx, account.cookie, group)
			if err != nil {
				s.logger.Warn("重新上架巡检读取已结束商品失败", "account", account.id, "group", group.GroupName, "err", err)
				continue
			}
			for _, item := range items {
				if reshelfCount >= maxPerScan || ctx.Err() != nil {
					break
				}
				outcome, reshelfed := s.processEndedItem(ctx, account, item, group.GroupName, quantity)
				outcomes = append(outcomes, outcome)
				if reshelfed {
					reshelfCount++
					if !sleepCtx(ctx, itemGap) {
						break
					}
				}
			}
		}
	}

	s.mu.Lock()
	s.status = Status{LastRun: started, LastDuration: time.Since(started), LastOutcomes: outcomes}
	s.totalReshelf += reshelfCount
	s.mu.Unlock()
	// 每轮都留一条心跳，方便确认巡检真的在跑（无需等到有商品被上架）。
	s.logger.Info("重新上架巡检完成",
		"accounts", len(accounts),
		"reshelf", reshelfCount,
		"outcomes", len(outcomes),
		"elapsed", time.Since(started).Round(time.Millisecond).String())
	return outcomes
}

// processEndedItem 处理一件已结束商品：检查未完成订单、按账号多库存限制降级并重新上架。
// 返回处理明细；第二个返回值表示是否成功上架（供调用方计数与节流）。
func (s *Scanner) processEndedItem(ctx context.Context, account account, item mtop.ItemListItem, groupLabel string, quantity int) (Outcome, bool) {
	// key 是冷却表键，包含账号避免不同账号同商品 ID 冲突。
	key := account.id + "/" + item.ID
	if !s.shouldAttempt(key) {
		return Outcome{}, false
	}
	if active, err := s.hasActiveOrder(ctx, item.ID); err == nil && active {
		return Outcome{AccountID: account.id, ItemID: item.ID, Title: item.Title, GroupName: groupLabel, Action: "skipped_active_order", Detail: "仍有未完成订单，暂不上架", At: time.Now()}, false
	}
	// 首选数量；被平台按账号拒绝多库存的商品降到 1 件上架，
	// 保证「卖完自动上架」在无多库存权限的账号上依然生效。
	submitQuantity := quantity
	if s.accountMultiInventoryDenied(account.id) {
		submitQuantity = 1
	}
	result, err := s.platform.ReshelfItem(ctx, account.cookie, item.ID, submitQuantity)
	degraded := false
	if err != nil && submitQuantity > 1 && mtop.IsMultiInventoryRejected(err) {
		s.markMultiInventoryDenied(account.id)
		s.logger.Warn("账号被平台限制多库存，按 1 件降级重试", "account", account.id, "item", item.ID)
		degraded = true
		result, err = s.platform.ReshelfItem(ctx, account.cookie, item.ID, 1)
	}
	s.record(key, err == nil)
	if err != nil {
		s.logger.Warn("重新上架失败", "account", account.id, "item", item.ID, "title", item.Title, "err", mtop.SanitizeReshelfFailure(err))
		return Outcome{AccountID: account.id, ItemID: item.ID, Title: item.Title, GroupName: groupLabel, Action: "failed", Detail: mtop.SanitizeReshelfFailure(err), At: time.Now()}, false
	}
	detail := fmt.Sprintf("库存 %d", result.Quantity)
	if degraded {
		detail = fmt.Sprintf("库存 %d（账号无多库存权限，已从 %d 件降级）", result.Quantity, quantity)
	}
	s.logger.Info("已重新上架", "account", account.id, "item", item.ID, "title", item.Title, "quantity", result.Quantity, "degraded", degraded)
	return Outcome{AccountID: account.id, ItemID: item.ID, Title: item.Title, GroupName: groupLabel, Action: "reshelf", Detail: detail, At: time.Now()}, true
}

// scanGrouplessAccount 兜底巡检 item.list 不返回分组信息的账号。
// 候选 = 本地商品记录（含被同步软删的行）中不在当前在售列表的商品；
// 逐个读取编辑详情，库存为 0（售罄）才重新上架，仍有库存（人工下架）则保持
// include_offline=false 的语义不动，避免把人工主动下架的商品自动挂回去。
func (s *Scanner) scanGrouplessAccount(ctx context.Context, account account, quantity int, reshelfCount *int, maxPerScan int) []Outcome {
	// onSaleIDs 是当前在售商品 ID 集合；兜底候选据此排除仍在售的商品。
	// 平台限制单页最大商品数（pageSize 过大直接返回 FAIL_BIZ_FORBIDDEN），按常规页大小取最多 3 页。
	onSaleIDs := make(map[string]struct{})
	for page := 1; page <= 3; page++ {
		res, err := s.platform.FetchItemsPage(ctx, account.cookie, page, scanPageSize)
		if err != nil {
			s.logger.Warn("兜底巡检读取在售列表失败", "account", account.id, "page", page, "err", err)
			return nil
		}
		for _, item := range res.Items {
			onSaleIDs[item.ID] = struct{}{}
		}
		if len(res.Items) == 0 {
			break
		}
	}
	candidates, err := s.candidateItemIDs(ctx, account.id)
	if err != nil {
		s.logger.Warn("兜底巡检读取本地商品记录失败", "account", account.id, "err", err)
		return nil
	}
	var outcomes []Outcome
	for _, itemID := range candidates {
		if *reshelfCount >= maxPerScan || ctx.Err() != nil {
			break
		}
		if _, ok := onSaleIDs[itemID]; ok {
			continue
		}
		key := account.id + "/" + itemID
		if !s.shouldAttempt(key) {
			continue
		}
		detail, derr := s.platform.FetchItemEditDetail(ctx, account.cookie, itemID)
		if derr != nil {
			// 读取失败常见于商品已在平台侧删除；记失败退避，避免每轮重复请求。
			s.record(key, false)
			outcomes = append(outcomes, Outcome{AccountID: account.id, ItemID: itemID, GroupName: "已售出(兜底)", Action: "failed", Detail: mtop.SanitizeReshelfFailure(derr), At: time.Now()})
			s.logger.Warn("兜底巡检读取商品编辑详情失败", "account", account.id, "item", itemID, "err", mtop.SanitizeReshelfFailure(derr))
			continue
		}
		if detail.Quantity > 0 {
			continue
		}
		outcome, reshelfed := s.processEndedItem(ctx, account, mtop.ItemListItem{ID: itemID, Title: detail.Title}, "已售出(兜底)", quantity)
		outcomes = append(outcomes, outcome)
		if reshelfed {
			*reshelfCount++
			if !sleepCtx(ctx, itemGap) {
				break
			}
		}
	}
	return outcomes
}

// candidateItemIDs 返回账号的本地商品记录 ID（含软删行），按更新时间倒序最多 50 条。
// 商品卖完后全量同步会把行软删，因此兜底候选池不能按 deleted_at 过滤。
func (s *Scanner) candidateItemIDs(ctx context.Context, cookieID string) ([]string, error) {
	rows, err := s.store.DB.QueryContext(ctx, `SELECT item_id FROM item_info WHERE cookie_id=? ORDER BY COALESCE(updated_at, created_at) DESC LIMIT 50`, cookieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Status 返回最近一轮巡检状态快照。
func (s *Scanner) Status() Status {
	if s == nil {
		return Status{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	status.TotalReshelf = s.totalReshelf
	return status
}

// account 是巡检使用的账号与凭证组合。
type account struct {
	id     string
	cookie string
}

// accounts 读取所有已启用账号的解密凭证。
func (s *Scanner) accounts(ctx context.Context) ([]account, error) {
	// rows 查询启用且有商品记录的账号。商品记录不过滤 deleted_at：
	// 商品卖完后全量同步会把行软删，恰好是最需要自动补货的时刻，不能因此把账号排除。
	rows, err := s.store.DB.QueryContext(ctx, `SELECT c.id FROM cookies c LEFT JOIN cookie_status s ON s.cookie_id = c.id WHERE COALESCE(s.enabled, 1) = 1 AND EXISTS (SELECT 1 FROM item_info i WHERE i.cookie_id = c.id) ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	accounts := make([]account, 0, len(ids))
	for _, id := range ids {
		cookie, err := s.store.Cookies.GetValue(ctx, id)
		if err != nil || strings.TrimSpace(cookie) == "" {
			continue
		}
		accounts = append(accounts, account{id: id, cookie: cookie})
	}
	return accounts, nil
}

// endedItems 读取某个已结束分组下的商品（最多 scanMaxPages 页）。
func (s *Scanner) endedItems(ctx context.Context, cookie string, group mtop.ItemGroup) ([]mtop.ItemListItem, error) {
	var items []mtop.ItemListItem
	for page := 1; page <= scanMaxPages; page++ {
		res, err := s.platform.FetchItemsInGroup(ctx, cookie, group.GroupID, group.GroupName, page, scanPageSize)
		if err != nil {
			return items, err
		}
		if res == nil || len(res.Items) == 0 {
			break
		}
		items = append(items, res.Items...)
		if len(res.Items) < scanPageSize || (res.TotalPages > 0 && page >= res.TotalPages) {
			break
		}
	}
	return items, nil
}

// hasActiveOrder 判断商品是否还有未完成订单；有则必须先完成履约再上架，避免重复售卖。
func (s *Scanner) hasActiveOrder(ctx context.Context, itemID string) (bool, error) {
	var count int
	err := s.store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders WHERE item_id = ? AND deleted_at IS NULL AND LOWER(COALESCE(order_status,'')) NOT IN ('completed','cancelled','closed','refunded','refund_success')`, itemID).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

// shouldAttempt 判断商品本轮到不到可以尝试上架：
// 上次失败要等满退避时间，上次成功只做一分钟内的最小间隔保护。
func (s *Scanner) shouldAttempt(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.attempts[key]
	if !seen {
		return true
	}
	if last.ok {
		return time.Since(last.at) >= successCooldown
	}
	return time.Since(last.at) >= failCooldown
}

// record 记录本次尝试的时间和结果。
func (s *Scanner) record(key string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[key] = attempt{at: time.Now(), ok: ok}
}

// accountMultiInventoryDenied 返回账号是否已被平台确认拒绝多库存。
func (s *Scanner) accountMultiInventoryDenied(accountID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.multiInventoryDenied[accountID]
}

// markMultiInventoryDenied 记录账号被平台拒绝多库存；后续该账号直接按 1 件上架。
func (s *Scanner) markMultiInventoryDenied(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.multiInventoryDenied[accountID] = true
}

// enabled 读取总开关，未配置时默认开启。
func (s *Scanner) enabled(ctx context.Context) bool {
	value, err := s.store.Settings.Get(ctx, settingEnabled)
	if err != nil || strings.TrimSpace(value) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "no":
		return false
	default:
		return true
	}
}

// interval 读取巡检周期，默认 2 分钟。
func (s *Scanner) interval(ctx context.Context) time.Duration {
	if minutes := s.intSetting(ctx, settingIntervalMinutes, 0); minutes > 0 {
		return time.Duration(minutes) * time.Minute
	}
	return defaultInterval
}

// quantity 读取重新上架时提交的库存数量，默认 1 件。
func (s *Scanner) quantity(ctx context.Context) int {
	return s.intSetting(ctx, settingQuantity, defaultQuantity)
}

// maxPerScan 读取单轮最多上架数量，默认 5 件。
func (s *Scanner) maxPerScan(ctx context.Context) int {
	return s.intSetting(ctx, settingMaxPerScan, defaultMaxPerScan)
}

// intSetting 读取正整数设置；缺失或非法时返回 fallback。
func (s *Scanner) intSetting(ctx context.Context, key string, fallback int) int {
	value, err := s.store.Settings.Get(ctx, key)
	if err != nil {
		return fallback
	}
	n, parseErr := strconv.Atoi(strings.TrimSpace(value))
	if parseErr != nil || n <= 0 {
		return fallback
	}
	return n
}

// groupEligible 判断某个分组是否参与自动重新上架。
// 售罄商品落在「已售出」分组，这类一定参与；「已下架」可能包含人工主动下架，
// 默认不动，只有显式打开 reshelf_include_offline 才纳入。
func (s *Scanner) groupEligible(ctx context.Context, group mtop.ItemGroup) bool {
	if group.ItemNumber <= 0 {
		return false
	}
	name := strings.TrimSpace(group.GroupName)
	if strings.Contains(name, "已售出") || strings.Contains(name, "售罄") {
		return true
	}
	if strings.Contains(name, "已下架") {
		return s.boolSetting(ctx, settingIncludeOffline, false)
	}
	return false
}

// boolSetting 读取布尔设置；缺失或无法识别时返回 fallback。
func (s *Scanner) boolSetting(ctx context.Context, key string, fallback bool) bool {
	value, err := s.store.Settings.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "on", "yes":
		return true
	case "false", "0", "off", "no":
		return false
	default:
		return fallback
	}
}

// sleepCtx 在 ctx 约束内等待；ctx 取消时返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
