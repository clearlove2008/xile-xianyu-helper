package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/db"
	"xianyu-go/internal/netdisk"
)

// netdiskID 生成网盘账号 ID（时间 + 随机串）。
func netdiskID() string {
	return time.Now().Format("20060102150405") + netdisk.RandomID(8)
}

// netdiskRepo 返回网盘账号 repository；未装配时返回 nil。
func (s *Server) netdiskRepo() *db.NetdiskAccounts {
	if s == nil {
		return nil
	}
	return s.netdiskAccountsRef
}

// netdiskSvc 返回网盘转存服务；未装配时惰性创建，保证测试零值 Server 可用。
func (s *Server) netdiskSvc() *netdisk.Service {
	if s == nil {
		return nil
	}
	if s.netdiskServiceRef == nil {
		s.netdiskServiceRef = netdisk.NewService()
	}
	return s.netdiskServiceRef
}

// listNetdiskAccounts 返回当前用户的网盘账号列表（不含 cookie）。
func (s *Server) listNetdiskAccounts(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	if repo == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘账号存储未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	items, err := repo.List(r.Context(), sess.UserID, r.URL.Query().Get("platform"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": items})
}

// createNetdiskAccount 手动添加网盘账号（粘贴 cookie）。
func (s *Server) createNetdiskAccount(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	if repo == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘账号存储未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	var body struct {
		Platform string `json:"platform"`
		Cookie   string `json:"cookie"`
		Remark   string `json:"remark"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := repo.Create(r.Context(), netdiskID(), body.Platform, body.Cookie, sess.UserID, strings.TrimSpace(body.Remark))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

// updateNetdiskAccount 更新账号 cookie 或备注/状态。
func (s *Server) updateNetdiskAccount(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	if repo == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘账号存储未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Cookie string `json:"cookie"`
		Remark string `json:"remark"`
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Cookie) != "" {
		if err := repo.UpdateValue(r.Context(), id, body.Cookie, sess.UserID); err != nil {
			writeNetdiskErr(w, err)
			return
		}
	}
	if err := repo.UpdateMeta(r.Context(), id, strings.TrimSpace(body.Remark), strings.TrimSpace(body.Status), sess.UserID); err != nil {
		writeNetdiskErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// deleteNetdiskAccount 删除网盘账号。
func (s *Server) deleteNetdiskAccount(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	if repo == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘账号存储未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	if err := repo.Delete(r.Context(), chi.URLParam(r, "id"), sess.UserID); err != nil {
		writeNetdiskErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// checkNetdiskAccount 校验账号 cookie 是否有效。
func (s *Server) checkNetdiskAccount(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	svc := s.netdiskSvc()
	if repo == nil || svc == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘服务未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	account, err := repo.Get(r.Context(), chi.URLParam(r, "id"), sess.UserID)
	if err != nil {
		writeNetdiskErr(w, err)
		return
	}
	ok, err := svc.CheckLogin(r.Context(), account.Platform, account.Value)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": ok})
}

// beginNetdiskQR 发起扫码登录。
func (s *Server) beginNetdiskQR(w http.ResponseWriter, r *http.Request) {
	svc := s.netdiskSvc()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘服务未初始化")
		return
	}
	var body struct {
		Platform string `json:"platform"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := svc.BeginQRLogin(r.Context(), body.Platform)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// netdiskQRStatus 查询扫码登录状态。
func (s *Server) netdiskQRStatus(w http.ResponseWriter, r *http.Request) {
	svc := s.netdiskSvc()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘服务未初始化")
		return
	}
	status, err := svc.QRLoginStatus(chi.URLParam(r, "sessionId"))
	if err != nil {
		if errors.Is(err, netdisk.ErrSessionNotFound) {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// cancelNetdiskQR 取消扫码会话。
func (s *Server) cancelNetdiskQR(w http.ResponseWriter, r *http.Request) {
	svc := s.netdiskSvc()
	if svc == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘服务未初始化")
		return
	}
	if err := svc.CancelQRLogin(chi.URLParam(r, "sessionId")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// transferNetdiskShare 执行一键转存并生成自己的分享链接。
func (s *Server) transferNetdiskShare(w http.ResponseWriter, r *http.Request) {
	repo := s.netdiskRepo()
	svc := s.netdiskSvc()
	if repo == nil || svc == nil {
		writeErr(w, http.StatusServiceUnavailable, "网盘服务未初始化")
		return
	}
	sess := authSess(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	var body struct {
		AccountID   string `json:"accountId"`
		ShareURL    string `json:"shareUrl"`
		Passcode    string `json:"passcode"`
		NewPasscode string `json:"newPasscode"`
		PeriodDays  int    `json:"periodDays"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	account, err := repo.Get(r.Context(), strings.TrimSpace(body.AccountID), sess.UserID)
	if err != nil {
		writeNetdiskErr(w, err)
		return
	}
	if account.Status != "active" {
		writeErr(w, http.StatusBadRequest, "该账号已停用")
		return
	}
	result, err := svc.Transfer(r.Context(), account.Platform, netdisk.ShareInput{
		ShareURL:    strings.TrimSpace(body.ShareURL),
		Passcode:    strings.TrimSpace(body.Passcode),
		Cookie:      account.Value,
		NewPasscode: strings.TrimSpace(body.NewPasscode),
		PeriodDays:  body.PeriodDays,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	repo.TouchLastUsed(r.Context(), account.ID)
	writeJSON(w, http.StatusOK, result)
}

// writeNetdiskErr 把 repository 错误映射为 HTTP 状态。
func writeNetdiskErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeErr(w, http.StatusNotFound, "账号不存在")
	case errors.Is(err, db.ErrAlreadyExists):
		writeErr(w, http.StatusConflict, "账号已存在")
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}
