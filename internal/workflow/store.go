// 任务 CRUD 与日志读取：SQL 参数化查询，跨 SQLite/MySQL/PostgreSQL 兼容（不用方言特有语法）。
package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// scanTask 把一行任务记录扫描到 Task 模型。
func scanTask(scan func(dest ...any) error) (*Task, error) {
	task := &Task{}
	var publishJSON string
	var lastRun, nextRun sql.NullTime
	if err := scan(&task.ID, &task.Keyword, &task.SearchCookieID, &publishJSON,
		&task.PriceMinCents, &task.PriceMaxCents, &task.TargetCount, &task.IntervalMinutes,
		&task.PublishGapSeconds, &task.UseSourceImage, &task.PriceCents, &task.Quantity,
		&task.Enabled, &lastRun, &nextRun); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(publishJSON), &task.PublishCookieIDs); err != nil {
		task.PublishCookieIDs = nil
	}
	if lastRun.Valid {
		task.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		task.NextRunAt = &nextRun.Time
	}
	return task, nil
}

// taskColumns 是任务查询的稳定列清单。
const taskColumns = `id, keyword, search_cookie_id, publish_cookie_ids, price_min_cents, price_max_cents, target_count, interval_minutes, publish_interval_seconds, use_source_image, price_cents, quantity, enabled, last_run_at, next_run_at`

// GetTask 按 ID 读取未删除的任务。
func (e *Engine) GetTask(ctx context.Context, id int64) (*Task, error) {
	row := e.store.DB.QueryRowContext(ctx,
		`SELECT `+taskColumns+` FROM workflow_tasks WHERE id = ? AND deleted_at IS NULL`, id)
	task, err := scanTask(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

// ListTasks 返回全部未删除任务（按创建时间倒序）。
func (e *Engine) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := e.store.DB.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM workflow_tasks WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		task, scanErr := scanTask(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

// TaskInput 是创建或更新任务的输入模型。
type TaskInput struct {
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
}

// validate 校验任务输入并归一化默认值。
func (input *TaskInput) validate() error {
	input.Keyword = strings.TrimSpace(input.Keyword)
	input.SearchCookieID = strings.TrimSpace(input.SearchCookieID)
	if input.Keyword == "" {
		return errors.New("关键词不能为空")
	}
	if len([]rune(input.Keyword)) > 60 {
		return errors.New("关键词过长（最多 60 字）")
	}
	if input.SearchCookieID == "" {
		return errors.New("请选择搜索账号")
	}
	if len(input.PublishCookieIDs) == 0 {
		return errors.New("请至少勾选一个发布账号")
	}
	if input.TargetCount < 1 || input.TargetCount > 10 {
		return errors.New("每轮上架数量必须在 1-10 之间")
	}
	if input.IntervalMinutes < 10 {
		input.IntervalMinutes = 10
	}
	if input.IntervalMinutes > 1440 {
		input.IntervalMinutes = 1440
	}
	if input.PublishGapSeconds < 5 {
		input.PublishGapSeconds = 5
	}
	if input.PublishGapSeconds > 600 {
		input.PublishGapSeconds = 600
	}
	if input.Quantity < 1 {
		input.Quantity = 1
	}
	return nil
}

// CreateTask 新建任务；新任务未启用时 next_run_at 为空，启用时立即安排执行。
func (e *Engine) CreateTask(ctx context.Context, input TaskInput) (int64, error) {
	if err := input.validate(); err != nil {
		return 0, err
	}
	publishJSON, _ := json.Marshal(input.PublishCookieIDs)
	var nextRun any
	if input.Enabled {
		nextRun = time.Now()
	}
	result, err := e.store.DB.ExecContext(ctx,
		`INSERT INTO workflow_tasks (keyword, search_cookie_id, publish_cookie_ids, price_min_cents, price_max_cents, target_count, interval_minutes, publish_interval_seconds, use_source_image, price_cents, quantity, enabled, next_run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Keyword, input.SearchCookieID, string(publishJSON), input.PriceMinCents, input.PriceMaxCents,
		input.TargetCount, input.IntervalMinutes, input.PublishGapSeconds, input.UseSourceImage,
		input.PriceCents, input.Quantity, input.Enabled, nextRun)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateTask 更新任务字段；启用状态翻转由 ToggleTask 单独管理。
func (e *Engine) UpdateTask(ctx context.Context, id int64, input TaskInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	publishJSON, _ := json.Marshal(input.PublishCookieIDs)
	_, err := e.store.DB.ExecContext(ctx,
		`UPDATE workflow_tasks SET keyword = ?, search_cookie_id = ?, publish_cookie_ids = ?, price_min_cents = ?, price_max_cents = ?,
		 target_count = ?, interval_minutes = ?, publish_interval_seconds = ?, use_source_image = ?, price_cents = ?, quantity = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		input.Keyword, input.SearchCookieID, string(publishJSON), input.PriceMinCents, input.PriceMaxCents,
		input.TargetCount, input.IntervalMinutes, input.PublishGapSeconds, input.UseSourceImage,
		input.PriceCents, input.Quantity, time.Now(), id)
	return err
}

// ToggleTask 切换任务启停；启用时安排立即执行一次。
func (e *Engine) ToggleTask(ctx context.Context, id int64, enabled bool) error {
	var nextRun any
	if enabled {
		nextRun = time.Now()
	}
	_, err := e.store.DB.ExecContext(ctx,
		`UPDATE workflow_tasks SET enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		enabled, nextRun, time.Now(), id)
	return err
}

// DeleteTask 软删除任务。
func (e *Engine) DeleteTask(ctx context.Context, id int64) error {
	_, err := e.store.DB.ExecContext(ctx,
		`UPDATE workflow_tasks SET deleted_at = ?, enabled = 0 WHERE id = ? AND deleted_at IS NULL`, time.Now(), id)
	return err
}

// ListLogs 返回任务的最近运行日志。
func (e *Engine) ListLogs(ctx context.Context, taskID int64, limit int) ([]RunLog, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	// 最新在前，前端按时间倒序展示。
	rows, err := e.store.DB.QueryContext(ctx,
		`SELECT id, task_id, level, message, created_at FROM workflow_run_logs WHERE task_id = ? ORDER BY id DESC LIMIT `+fmt.Sprint(limit), taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []RunLog
	for rows.Next() {
		var entry RunLog
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.Level, &entry.Message, &entry.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

// ModelConfigInput 读写模型设置。
type ModelConfigInput struct {
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
	Fallback string `json:"fallback"`
}

// GetModels 读取服务端保存的文案与图片模型配置。
func (e *Engine) GetModels(ctx context.Context) (ModelConfigInput, ModelConfigInput, error) {
	var text, image ModelConfigInput
	if raw, err := e.store.Settings.Get(ctx, SettingTextModel); err == nil && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &text)
	}
	if raw, err := e.store.Settings.Get(ctx, SettingImageModel); err == nil && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &image)
	}
	return text, image, nil
}

// SetModels 保存文案与图片模型配置。
func (e *Engine) SetModels(ctx context.Context, text, image ModelConfigInput) error {
	textJSON, _ := json.Marshal(text)
	imageJSON, _ := json.Marshal(image)
	return e.store.Settings.SetMany(ctx, map[string]string{
		SettingTextModel:  string(textJSON),
		SettingImageModel: string(imageJSON),
	})
}
