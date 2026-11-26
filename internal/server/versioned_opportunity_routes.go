package server

import (
	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/auth"
)

// mountVersionedOpportunityRoutes 挂载商机挖掘入口。
// 复用闲鱼助手会话与账号 Cookie：登录后即可用任意已登录账号搜索闲鱼在售商品。
// 改写、生图与发布复用 Studio 既有的模型端点与商品发布链路，不重复建设。
func (s *Server) mountVersionedOpportunityRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Use(auth.RequireAuth)

		r.Get("/api/v1/opportunity/search", s.searchOpportunities)
		r.Get("/api/v1/opportunity/detail", s.opportunityDetail)
	})
}
