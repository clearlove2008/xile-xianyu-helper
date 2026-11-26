package server

import (
	"github.com/go-chi/chi/v5"
)

// mountVersionedNetdiskRoutes 挂载网盘转存与网盘账号管理入口。
// 复用闲鱼助手会话，网盘 cookie 独立加密存储，不引入第二套账号体系。
func (s *Server) mountVersionedNetdiskRoutes(r chi.Router) {
	r.Route("/api/v1/netdisk", func(r chi.Router) {
		// 网盘账号管理（cookie 独立存储）。
		r.Get("/accounts", s.listNetdiskAccounts)
		r.Post("/accounts", s.createNetdiskAccount)
		r.Put("/accounts/{id}", s.updateNetdiskAccount)
		r.Delete("/accounts/{id}", s.deleteNetdiskAccount)
		r.Post("/accounts/{id}/check", s.checkNetdiskAccount)

		// 扫码登录（夸克/百度）。
		r.Post("/qr/begin", s.beginNetdiskQR)
		r.Get("/qr/status/{sessionId}", s.netdiskQRStatus)
		r.Post("/qr/cancel/{sessionId}", s.cancelNetdiskQR)

		// 一键转存并生成自己的分享链接。
		r.Post("/transfer", s.transferNetdiskShare)
	})
}
