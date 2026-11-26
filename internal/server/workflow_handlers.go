package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/workflow"
)

// workflowTaskResponse 是任务列表项的 HTTP 模型。
type workflowTaskResponse struct {
	ID                  int64   `json:"id"`
	Keyword             string  `json:"keyword"`
	SearchCookieID      string  `json:"searchCookieId"`
	PublishCookieIDs    []string `json:"publishCookieIds"`
	PriceMinCents       int64   `json:"priceMinCents"`
	PriceMaxCents       int64   `json:"priceMaxCents"`
	TargetCount         int     `json:"targetCount"`
	IntervalMinutes     int     `json:"intervalMinutes"`
	PublishGapSeconds   int     `json:"publishGapSeconds"`
	UseSourceImage      bool    `json:"useSourceImage"`
	PriceCents          int64   `json:"priceCents"`
	Quantity            int     `json:"quantity"`
	Enabled             bool    `json:"enabled"`
	LastRunAt           string  `json:"lastRunAt,omitempty"`
	NextRunAt           string  `json:"nextRunAt,omitempty"`
}

// workflowLogResponse 是运行日志的 HTTP 模型。
type workflowLogResponse struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"taskId"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

// workflowEngine 返回工作流引擎；未装配（测试零值 Server）时返回 nil。
func (s *Server) workflowEngine() *workflow.Engine {
	if s == nil {
		return nil
	}
	return s.workflowEngineRef
}

// taskFromModel 把应用模型转成 HTTP 响应。
func taskFromModel(task workflow.Task) workflowTaskResponse {
	resp := workflowTaskResponse{
		ID: task.ID, Keyword: task.Keyword, SearchCookieID: task.SearchCookieID,
		PublishCookieIDs: task.PublishCookieIDs, PriceMinCents: task.PriceMinCents, PriceMaxCents: task.PriceMaxCents,
		TargetCount: task.TargetCount, IntervalMinutes: task.IntervalMinutes, PublishGapSeconds: task.PublishGapSeconds,
		UseSourceImage: task.UseSourceImage, PriceCents: task.PriceCents, Quantity: task.Quantity, Enabled: task.Enabled,
	}
	if task.PublishCookieIDs == nil {
		resp.PublishCookieIDs = []string{}
	}
	if task.LastRunAt != nil {
		resp.LastRunAt = task.LastRunAt.Format("2006-01-02 15:04:05")
	}
	if task.NextRunAt != nil {
		resp.NextRunAt = task.NextRunAt.Format("2006-01-02 15:04:05")
	}
	return resp
}

// taskInputFromRequest 解析创建/更新请求体。
func taskInputFromRequest(r *http.Request) (workflow.TaskInput, error) {
	var body struct {
		Keyword           string   `json:"keyword"`
		SearchCookieID    string   `json:"searchCookieId"`
		PublishCookieIDs  []string `json:"publishCookieIds"`
		PriceMinCents     int64    `json:"priceMinCents"`
		PriceMaxCents     int64    `json:"priceMaxCents"`
		TargetCount       int      `json:"targetCount"`
		IntervalMinutes   int      `json:"intervalMinutes"`
		PublishGapSeconds int      `json:"publishGapSeconds"`
		UseSourceImage    bool     `json:"useSourceImage"`
		PriceCents        int64    `json:"priceCents"`
		Quantity          int      `json:"quantity"`
		Enabled           bool     `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		return workflow.TaskInput{}, err
	}
	return workflow.TaskInput{
		Keyword: body.Keyword, SearchCookieID: body.SearchCookieID, PublishCookieIDs: body.PublishCookieIDs,
		PriceMinCents: body.PriceMinCents, PriceMaxCents: body.PriceMaxCents, TargetCount: body.TargetCount,
		IntervalMinutes: body.IntervalMinutes, PublishGapSeconds: body.PublishGapSeconds, UseSourceImage: body.UseSourceImage,
		PriceCents: body.PriceCents, Quantity: body.Quantity, Enabled: body.Enabled,
	}, nil
}

// listWorkflowTasks 返回任务列表。
func (s *Server) listWorkflowTasks(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	tasks, err := engine.ListTasks(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取任务失败")
		return
	}
	items := make([]workflowTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, taskFromModel(task))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

// createWorkflowTask 新建任务。
func (s *Server) createWorkflowTask(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	input, err := taskInputFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	id, createErr := engine.CreateTask(r.Context(), input)
	if createErr != nil {
		writeErr(w, http.StatusBadRequest, createErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "success": true})
}

// updateWorkflowTask 更新任务。
func (s *Server) updateWorkflowTask(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	input, err := taskInputFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if updateErr := engine.UpdateTask(r.Context(), id, input); updateErr != nil {
		writeErr(w, http.StatusBadRequest, updateErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Success: true})
}

// toggleWorkflowTask 启停任务。
func (s *Server) toggleWorkflowTask(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if toggleErr := engine.ToggleTask(r.Context(), id, body.Enabled); toggleErr != nil {
		writeErr(w, http.StatusInternalServerError, "切换状态失败")
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Success: true})
}

// deleteWorkflowTask 删除任务（软删）。
func (s *Server) deleteWorkflowTask(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	if deleteErr := engine.DeleteTask(r.Context(), id); deleteErr != nil {
		writeErr(w, http.StatusInternalServerError, "删除失败")
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Success: true})
}

// runWorkflowTask 手动触发一次任务（异步执行）。
func (s *Server) runWorkflowTask(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	if !engine.RunOnceBackground(r.Context(), id) {
		writeErr(w, http.StatusConflict, "该任务正在执行中，请稍候")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "已触发执行，结果稍后写入运行日志"})
}

// listWorkflowLogs 读取任务运行日志。
func (s *Server) listWorkflowLogs(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "任务 ID 无效")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := engine.ListLogs(r.Context(), id, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取日志失败")
		return
	}
	entries := make([]workflowLogResponse, 0, len(logs))
	for _, entry := range logs {
		entries = append(entries, workflowLogResponse{
			ID: entry.ID, TaskID: entry.TaskID, Level: entry.Level, Message: entry.Message,
			CreatedAt: entry.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": entries})
}

// getWorkflowModels 读取模型配置（密钥打码返回，避免明文回传）。
func (s *Server) getWorkflowModels(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	text, image, err := engine.GetModels(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取模型配置失败")
		return
	}
	masked := func(cfg workflow.ModelConfigInput) workflow.ModelConfigInput {
		if strings.TrimSpace(cfg.APIKey) != "" {
			cfg.APIKey = "********"
		}
		return cfg
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": masked(text), "image": masked(image)})
}

// setWorkflowModels 保存模型配置；密钥为掩码时保留原值。
func (s *Server) setWorkflowModels(w http.ResponseWriter, r *http.Request) {
	engine := s.workflowEngine()
	if engine == nil {
		writeErr(w, http.StatusServiceUnavailable, "工作流引擎未启动")
		return
	}
	var body struct {
		Text  workflow.ModelConfigInput `json:"text"`
		Image workflow.ModelConfigInput `json:"image"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	currentText, currentImage, _ := engine.GetModels(r.Context())
	// 掩码表示未修改，保留已存密钥。
	if strings.Contains(body.Text.APIKey, "*") {
		body.Text.APIKey = currentText.APIKey
	}
	if strings.Contains(body.Image.APIKey, "*") {
		body.Image.APIKey = currentImage.APIKey
	}
	if err := engine.SetModels(r.Context(), body.Text, body.Image); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存模型配置失败")
		return
	}
	writeJSON(w, http.StatusOK, operationResponse{Success: true})
}
