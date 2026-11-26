package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/auth"
	"xianyu-go/internal/studio"
)

// mountVersionedStudioRoutes 挂载 AI 出图工作台入口。
// 复用闲鱼助手会话：登录后即可出图，不引入第二套账号体系。
// 图片与文字模型的地址、密钥随请求传入，服务端只做转发，不落库、不写日志。
func (s *Server) mountVersionedStudioRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Use(auth.RequireAuth)

		handler := studio.NewHandler()
		r.Post("/api/v1/studio/models", handler.Models)
		r.Post("/api/v1/studio/prompt", handler.Preview)
		r.Post("/api/v1/studio/polish", handler.Polish)
		r.Post("/api/v1/studio/generate", handler.Generate)
		// defaults 提供默认系统提示词，避免前端与后端各存一份文案而漂移。
		r.Get("/api/v1/studio/defaults", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"systemPrompt": studio.DefaultSystemPrompt})
		})
	})
}
