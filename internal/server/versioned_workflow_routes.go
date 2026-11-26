package server

import (
	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/auth"
)

// mountVersionedWorkflowRoutes 挂载商品工作流入口。
// 任务 CRUD、手动触发与模型配置都在这里；引擎的定时循环由生命周期协调器拥有。
func (s *Server) mountVersionedWorkflowRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Use(auth.RequireAuth)

		r.Get("/api/v1/workflow/tasks", s.listWorkflowTasks)
		r.Post("/api/v1/workflow/tasks", s.createWorkflowTask)
		r.Put("/api/v1/workflow/tasks/{id}", s.updateWorkflowTask)
		r.Delete("/api/v1/workflow/tasks/{id}", s.deleteWorkflowTask)
		r.Post("/api/v1/workflow/tasks/{id}/toggle", s.toggleWorkflowTask)
		r.Post("/api/v1/workflow/tasks/{id}/run", s.runWorkflowTask)
		r.Get("/api/v1/workflow/tasks/{id}/logs", s.listWorkflowLogs)
		r.Get("/api/v1/workflow/models", s.getWorkflowModels)
		r.Put("/api/v1/workflow/models", s.setWorkflowModels)
	})
}
