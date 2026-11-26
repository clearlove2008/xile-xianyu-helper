// Package workflow 实现商品工作流：按关键词定时选品，AI 改写 + 生图后自动发布到多个账号。
// 设计对齐 relister：依赖以窄接口注入，周期循环由生命周期协调器拥有。
package workflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"xianyu-go/internal/application/items"
	"xianyu-go/internal/db"
	"xianyu-go/internal/studio"
	"xianyu-go/internal/xianyu/mtop"
)

// Searcher 是商机搜索依赖，由 mtop 客户端实现。
type Searcher interface {
	SearchItems(ctx context.Context, cookiesStr, keyword string, page, pageSize int) (*mtop.SearchResult, error)
}

// Publisher 是单商品发布依赖，由商品发布应用服务实现。
type Publisher interface {
	PublishSingle(ctx context.Context, input items.PublishInput) (items.PublishOutcome, error)
}

// AIEngine 是文案改写与生图依赖，由 studio 引擎客户端实现。
type AIEngine interface {
	Chat(ctx context.Context, cfg studio.ModelConfig, system, user string) (string, error)
	Generate(ctx context.Context, cfg studio.ModelConfig, in studio.GenerateRequest) (studio.GenerateResult, error)
}

// Task 是一条工作流任务配置。
type Task struct {
	ID                int64
	Keyword           string
	SearchCookieID    string
	PublishCookieIDs  []string
	PriceMinCents     int64
	PriceMaxCents     int64
	TargetCount       int
	IntervalMinutes   int
	PublishGapSeconds int
	UseSourceImage    bool
	PriceCents        int64
	Quantity          int
	Enabled           bool
	LastRunAt         *time.Time
	NextRunAt         *time.Time
}

// RunLog 是一条工作流运行日志。
type RunLog struct {
	ID        int64
	TaskID    int64
	Level     string
	Message   string
	CreatedAt time.Time
}

// 模型配置在 system_settings 中的存储键。
const (
	SettingTextModel  = "workflow_text_model"
	SettingImageModel = "workflow_image_model"
)

// engineTickInterval 是巡检间隔：工作流任务最细粒度为分钟级。
const engineTickInterval = 30 * time.Second

// searchPageSize 是每轮选品的搜索条数。
const searchPageSize = 20

// Engine 是工作流执行引擎，同时承担任务 CRUD 与运行循环。
type Engine struct {
	store     *db.Store
	searcher  Searcher
	publisher Publisher
	ai        AIEngine
	logger    *slog.Logger

	// mu 保护 running 任务的互斥执行（定时与手动触发不重叠）。
	mu sync.Mutex
	// running 保存正在执行的任务 ID 集合。
	running map[int64]struct{}
}

// New 构造工作流引擎。
func New(store *db.Store, searcher Searcher, publisher Publisher, ai AIEngine, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		store: store, searcher: searcher, publisher: publisher, ai: ai, logger: logger,
		running: make(map[int64]struct{}),
	}
}

// Run 是生命周期组件的启动入口：阻塞循环直到 Context 取消。
func (e *Engine) Run(ctx context.Context) error {
	// 启动时把已启用且到期时间在过去的任务补上下一次执行时间，避免停机期间错过的任务永远不再执行。
	e.recoverOverdueTasks(ctx)
	ticker := time.NewTicker(engineTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			e.runDueTasks(ctx)
		}
	}
}

// recoverOverdueTasks 把 next_run_at 为空或已过期的启用任务安排到下一轮执行。
func (e *Engine) recoverOverdueTasks(ctx context.Context) {
	if _, err := e.store.DB.ExecContext(ctx,
		`UPDATE workflow_tasks SET next_run_at = ? WHERE enabled = 1 AND deleted_at IS NULL AND (next_run_at IS NULL OR next_run_at <= ?)`,
		time.Now(), time.Now()); err != nil {
		e.logger.Warn("工作流恢复到期任务失败", "err", err)
	}
}

// runDueTasks 执行所有到期任务。
func (e *Engine) runDueTasks(ctx context.Context) {
	tasks, err := e.ListTasks(ctx)
	if err != nil {
		e.logger.Warn("工作流读取任务失败", "err", err)
		return
	}
	now := time.Now()
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		if task.NextRunAt != nil && task.NextRunAt.After(now) {
			continue
		}
		e.RunOnceBackground(ctx, task.ID)
	}
}

// RunOnceBackground 异步触发一次任务执行；同一任务并发触发会被忽略。
func (e *Engine) RunOnceBackground(ctx context.Context, taskID int64) bool {
	e.mu.Lock()
	if _, busy := e.running[taskID]; busy {
		e.mu.Unlock()
		return false
	}
	e.running[taskID] = struct{}{}
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.running, taskID)
			e.mu.Unlock()
		}()
		// bgCtx 使用引擎自身的生命周期上下文派生，避免 HTTP 请求结束后任务被中途取消。
		bgCtx := context.Background()
		if ctx != nil && ctx.Err() == nil {
			bgCtx = ctx
		}
		e.RunOnce(bgCtx, taskID)
	}()
	return true
}

// RunOnce 同步执行一次任务：搜索 → 筛选 → 改写 → 生图 → 发布，并更新调度时间。
func (e *Engine) RunOnce(ctx context.Context, taskID int64) error {
	task, err := e.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return errors.New("任务不存在")
	}
	// 无论成败都推进 next_run_at，避免失败任务每 30 秒重试打爆平台。
	interval := task.IntervalMinutes
	if interval < 10 {
		interval = 10
	}
	defer func() {
		next := time.Now().Add(time.Duration(interval) * time.Minute)
		last := time.Now()
		if _, err := e.store.DB.ExecContext(context.Background(),
			`UPDATE workflow_tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?`, last, next, taskID); err != nil {
			e.logger.Warn("工作流更新调度时间失败", "task", taskID, "err", err)
		}
	}()

	cookie, err := e.cookieValue(ctx, task.SearchCookieID)
	if err != nil || cookie == "" {
		e.log(ctx, taskID, "error", "搜索账号不可用或已失效，跳过本轮")
		return fmt.Errorf("搜索账号不可用: %w", err)
	}
	result, err := e.searcher.SearchItems(ctx, cookie, task.Keyword, 1, searchPageSize)
	if err != nil {
		e.log(ctx, taskID, "error", "搜索失败："+mtop.SanitizeReshelfFailure(err))
		return err
	}
	// 候选按价格区间过滤并跳过已发布过的源商品。
	candidates := e.filterCandidates(ctx, *task, result.Items)
	if len(candidates) == 0 {
		e.log(ctx, taskID, "info", fmt.Sprintf("搜到 %d 件，价格区间内未发过的新品为 0", len(result.Items)))
		return nil
	}
	if len(candidates) > task.TargetCount {
		candidates = candidates[:task.TargetCount]
	}

	// 发布账号逐一校验可用性。
	publishTargets := make([]struct {
		id     string
		cookie string
	}, 0, len(task.PublishCookieIDs))
	for _, accountID := range task.PublishCookieIDs {
		value, cookieErr := e.cookieValue(ctx, accountID)
		if cookieErr != nil || value == "" {
			e.log(ctx, taskID, "warn", "发布账号 "+accountID+" 不可用，跳过该账号")
			continue
		}
		publishTargets = append(publishTargets, struct {
			id     string
			cookie string
		}{id: accountID, cookie: value})
	}
	if len(publishTargets) == 0 {
		e.log(ctx, taskID, "error", "没有可用的发布账号")
		return errors.New("没有可用的发布账号")
	}

	// 文案与图片模型配置（可能未配置，未配置时降级）。
	textCfg, imageCfg := e.modelConfigs(ctx)
	publishedCount := 0
	for _, item := range candidates {
		if ctx.Err() != nil {
			break
		}
		// 改写：未配置文字模型时沿用源标题。
		title := item.Title
		if len([]rune(title)) > 30 {
			title = string([]rune(title)[:30])
		}
		description := strings.Join([]string{item.Title, "拍下自动发货，闲置好物，非诚勿扰"}, "\n")
		if textCfg.Model != "" && textCfg.BaseURL != "" {
			if rewritten, chatErr := e.rewriteCopy(ctx, textCfg, item); chatErr == nil && strings.TrimSpace(rewritten) != "" {
				description = rewritten
				// 闲鱼标题展示取描述第一行，截到 30 字。
				firstLine := strings.SplitN(description, "\n", 2)[0]
				if len([]rune(firstLine)) > 30 {
					firstLine = string([]rune(firstLine)[:30])
				}
				title = firstLine
			} else if chatErr != nil {
				e.log(ctx, taskID, "warn", "AI 改写失败（沿用源文案）："+chatErr.Error())
			}
		}

		// 图片：源图直用或 AI 生图。
		image, imageErr := e.coverImage(ctx, *task, imageCfg, item, title)
		if imageErr != nil {
			e.log(ctx, taskID, "warn", "封面图获取失败，跳过该商品："+imageErr.Error())
			continue
		}

		// 价格：任务固定价优先，否则用源价。
		priceCents := task.PriceCents
		if priceCents <= 0 {
			priceCents = parsePriceCents(item.Price)
		}
		if priceCents <= 0 {
			e.log(ctx, taskID, "warn", "无法确定售价（源商品无价格且任务未固定价），跳过该商品")
			continue
		}

		for index, target := range publishTargets {
			if ctx.Err() != nil {
				break
			}
			// 账号维度去重：同一账号不重复发同一源商品。
			published, dupErr := e.alreadyPublished(ctx, target.id, item.ItemID)
			if dupErr == nil && published {
				continue
			}
			if index > 0 || publishedCount > 0 {
				if !sleepCtx(ctx, time.Duration(task.PublishGapSeconds)*time.Second) {
					break
				}
			}
			outcome, publishErr := e.publisher.PublishSingle(ctx, items.PublishInput{
				CookieID: target.id, Title: title, Description: description,
				PriceCents: priceCents, Quantity: task.Quantity, PostageMode: "free",
				Images: []items.Image{{Filename: "cover.png", ContentType: image.contentType, Data: image.data}},
			})
			if publishErr != nil || outcome.Result == nil || outcome.Result.ItemID == "" {
				if publishErr == nil {
					publishErr = errors.New("平台未返回商品 ID")
				}
				e.log(ctx, taskID, "error", fmt.Sprintf("发布失败（%s）：%v", item.Title, publishErr))
				continue
			}
			e.recordPublished(ctx, target.id, item.ItemID, taskID, outcome.Result.ItemID, title)
			publishedCount++
			stockNote := ""
			if outcome.Result.StockWarning != "" {
				stockNote = "；" + outcome.Result.StockWarning
			}
			e.log(ctx, taskID, "info", fmt.Sprintf("已上架「%s」到账号 %s（商品 ID %s）%s", title, target.id, outcome.Result.ItemID, stockNote))
		}
	}
	e.log(ctx, taskID, "info", fmt.Sprintf("本轮完成：发布 %d 件商品", publishedCount))
	return nil
}

// coverImageData 是待发布封面的字节数据。
type coverImage struct {
	data        []byte
	contentType string
}

// coverImage 决定封面来源：任务勾选源图时下载源图，否则 AI 生成。
func (e *Engine) coverImage(ctx context.Context, task Task, imageCfg studio.ModelConfig, item mtop.SearchItem, title string) (*coverImage, error) {
	if task.UseSourceImage {
		if item.ImageURL == "" {
			return nil, errors.New("源商品没有封面图")
		}
		return downloadImage(ctx, item.ImageURL)
	}
	if imageCfg.BaseURL == "" || imageCfg.Model == "" {
		return nil, errors.New("未配置工作流图片模型（且未勾选用源图）")
	}
	result, err := e.ai.Generate(ctx, imageCfg, studio.GenerateRequest{
		Prompt: studio.BuildPrompt(studio.PromptFields{Headline: title, Price: formatCents(taskPriceCents(task, item))}),
	})
	if err != nil {
		return nil, err
	}
	data, contentType, err := decodeDataURL(result.Image)
	if err != nil {
		return nil, err
	}
	return &coverImage{data: data, contentType: contentType}, nil
}

// rewriteCopy 用文字模型把源商品改写成闲鱼风格文案。
func (e *Engine) rewriteCopy(ctx context.Context, cfg studio.ModelConfig, item mtop.SearchItem) (string, error) {
	system := "你是二手电商文案助手，只输出可直接发布的中文文案，不要解释、不要 markdown。"
	user := fmt.Sprintf("把下面的闲鱼商品改写成一版口语化文案：第一行是不超过30字的标题，随后换行写2-4行卖点描述。保留价格与商品事实，不夸大。\n标题：%s\n价格：%s\n地区：%s",
		item.Title, item.Price, item.Area)
	return e.ai.Chat(ctx, cfg, system, user)
}

// modelConfigs 读取服务端保存的文案与图片模型配置。
func (e *Engine) modelConfigs(ctx context.Context) (studio.ModelConfig, studio.ModelConfig) {
	var textCfg, imageCfg studio.ModelConfig
	if raw, err := e.store.Settings.Get(ctx, SettingTextModel); err == nil && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &textCfg)
	}
	if raw, err := e.store.Settings.Get(ctx, SettingImageModel); err == nil && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &imageCfg)
	}
	return textCfg, imageCfg
}

// filterCandidates 按价格区间与去重表过滤搜索结果。
func (e *Engine) filterCandidates(ctx context.Context, task Task, items []mtop.SearchItem) []mtop.SearchItem {
	candidates := make([]mtop.SearchItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ItemID) == "" || strings.TrimSpace(item.Title) == "" {
			continue
		}
		cents := parsePriceCents(item.Price)
		if cents <= 0 {
			continue
		}
		if task.PriceMinCents > 0 && cents < task.PriceMinCents {
			continue
		}
		if task.PriceMaxCents > 0 && cents > task.PriceMaxCents {
			continue
		}
		// 任一发布账号未发过该源商品即视为候选（发布阶段再做账号级去重）。
		fresh := false
		for _, accountID := range task.PublishCookieIDs {
			published, err := e.alreadyPublished(ctx, accountID, item.ItemID)
			if err == nil && !published {
				fresh = true
				break
			}
		}
		if !fresh {
			continue
		}
		candidates = append(candidates, item)
	}
	return candidates
}

// alreadyPublished 判断某账号是否已发布过某源商品。
func (e *Engine) alreadyPublished(ctx context.Context, cookieID, sourceItemID string) (bool, error) {
	var count int
	err := e.store.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_published_goods WHERE cookie_id = ? AND source_item_id = ?`, cookieID, sourceItemID).Scan(&count)
	return count > 0, err
}

// recordPublished 记录一次成功发布用于去重（已发布的组合在调用前排除，这里只兜底并发插入冲突）。
func (e *Engine) recordPublished(ctx context.Context, cookieID, sourceItemID string, taskID int64, itemID, title string) {
	if _, err := e.store.DB.ExecContext(ctx,
		`INSERT INTO workflow_published_goods (cookie_id, source_item_id, task_id, item_id, title) VALUES (?, ?, ?, ?, ?)`,
		cookieID, sourceItemID, taskID, itemID, title); err != nil {
		e.logger.Warn("工作流去重记录写入失败", "err", err)
	}
}

// log 写一条运行日志（写库失败不影响主流程）。
func (e *Engine) log(ctx context.Context, taskID int64, level, message string) {
	e.logger.Info("工作流", "task", taskID, "level", level, "message", message)
	if _, err := e.store.DB.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO workflow_run_logs (task_id, level, message) VALUES (?, ?, ?)`, taskID, level, message); err != nil {
		e.logger.Warn("工作流日志写入失败", "err", err)
	}
}

// cookieValue 读取账号凭证明文。
func (e *Engine) cookieValue(ctx context.Context, cookieID string) (string, error) {
	value, err := e.store.Cookies.GetValue(ctx, cookieID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// parsePriceCents 把 "160"/"160.5" 形式的价格解析为分。
func parsePriceCents(price string) int64 {
	price = strings.TrimSpace(price)
	if price == "" {
		return 0
	}
	yuan, err := strconv.ParseFloat(price, 64)
	if err != nil || yuan <= 0 {
		return 0
	}
	return int64(yuan*100 + 0.5)
}

// formatCents 把分格式化为元文本。
func formatCents(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', -1, 64)
}

// taskPriceCents 计算任务的发布价（固定价优先，否则源价）。
func taskPriceCents(task Task, item mtop.SearchItem) int64 {
	if task.PriceCents > 0 {
		return task.PriceCents
	}
	return parsePriceCents(item.Price)
}

// downloadImage 下载源商品封面图。
func downloadImage(ctx context.Context, imageURL string) (*coverImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载源图返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		contentType = "image/jpeg"
	}
	return &coverImage{data: data, contentType: contentType}, nil
}

// decodeDataURL 解码 data URL 形式的生图结果。
func decodeDataURL(dataURL string) ([]byte, string, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 || !strings.HasPrefix(dataURL, "data:") {
		return nil, "", errors.New("生图结果不是 data URL")
	}
	header := dataURL[5:comma]
	contentType := "image/png"
	if idx := strings.Index(header, ";"); idx > 0 {
		contentType = header[:idx]
	}
	decoded, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return nil, "", err
	}
	return decoded, contentType, nil
}

// sleepCtx 等待指定时长；Context 取消时返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
