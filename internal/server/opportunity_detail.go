package server

import (
	"net/http"
	"regexp"
	"xianyu-go/internal/xianyu/mtop"
)

// opportunityDetail 在登录与账号归属校验后读取完整原文；详情失败时不返回截断摘要。
func (s *Server) opportunityDetail(w http.ResponseWriter, r *http.Request) {
	// itemID 只接受平台数字 ID，不能将任意网址交给出站请求。
	itemID := r.URL.Query().Get("item_id")
	if !regexp.MustCompile(`^[0-9]{1,24}$`).MatchString(itemID) {
		writeErr(w, http.StatusBadRequest, "商品 ID 无效")
		return
	}
	// cookies 为当前用户有权操作的搜索账号凭证，只传入平台客户端。
	cookies, _, ok := s.cookieForCurrentUser(w, r, r.URL.Query().Get("cookie_id"))
	if !ok {
		return
	}
	// detail、err 是详情原文或安全错误，失败不允许继续使用摘要。
	detail, err := mtop.NewClient().FetchOpportunityDetail(r.Context(), cookies, itemID)
	if err != nil {
		s.writeOpportunitySearchError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, detail)
}
