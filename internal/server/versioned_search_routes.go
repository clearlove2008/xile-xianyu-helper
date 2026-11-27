package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/auth"
	"xianyu-go/internal/search"
)

// 网盘搜索的系统设置键。这些键走闲鱼助手既有的通用设置存储，
// 首次保存后即生效，无需重启进程。
const (
	searchSettingEnabled         = "search_enabled"
	searchSettingPlugins         = "search_plugins"
	searchSettingChannels        = "search_channels"
	searchSettingProxy           = "search_proxy"
	searchSettingEnableTG        = "search_enable_tg"
	searchSettingCacheTTL        = "search_cache_ttl_minutes"
	searchSettingPluginTimeout   = "search_plugin_timeout"
	searchSettingResponseTimeout = "search_response_timeout"
)

// mountVersionedSearchRoutes 挂载网盘资源搜索入口。
// 复用闲鱼助手会话：登录后即可搜索，不引入第二套账号体系。
func (s *Server) mountVersionedSearchRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Use(auth.RequireAuth)

		handler := search.NewHandler(s.searchEngine, s.searchOptions)
		r.Get("/api/v1/search/search", handler.Search)
		r.Post("/api/v1/search/search", handler.Search)
		r.Get("/api/v1/search/options", handler.Options)
	})
}

// searchOptions 从系统设置解析网盘搜索运行参数。
// 未登录或读取失败时回退到默认配置，保证搜索入口本身不因设置异常而不可用。
func (s *Server) searchOptions(ctx context.Context, r *http.Request) (search.Options, error) {
	opts := s.searchDefaults()

	sess := authSess(r)
	if sess == nil {
		return opts, nil
	}
	values, err := s.settingsApplication().GetSystem(ctx, sess.UserID)
	if err != nil {
		return opts, nil
	}

	if v, ok := values[searchSettingEnabled]; ok {
		opts.Enabled = parseSettingBool(v, opts.Enabled)
	}
	if v, ok := values[searchSettingEnableTG]; ok {
		opts.EnableTG = parseSettingBool(v, opts.EnableTG)
	}
	if v, ok := values[searchSettingPlugins]; ok {
		// 空字符串表示明确不启用任何插件，与"未配置"区分。
		opts.Plugins = splitSettingList(v)
	}
	if v := strings.TrimSpace(values[searchSettingChannels]); v != "" {
		opts.Channels = splitSettingList(v)
	}
	if v := strings.TrimSpace(values[searchSettingProxy]); v != "" {
		opts.ProxyURL = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(values[searchSettingCacheTTL])); err == nil && v > 0 {
		opts.CacheTTLMinutes = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(values[searchSettingPluginTimeout])); err == nil && v > 0 {
		opts.PluginTimeoutSeconds = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(values[searchSettingResponseTimeout])); err == nil && v > 0 {
		opts.ResponseTimeoutSeconds = v
	}
	// 未启用 TG 时清空频道，避免向必然失败的境外源发起请求。
	if !opts.EnableTG {
		opts.Channels = nil
	}
	return opts, nil
}

// searchDefaults 返回进程级默认配置，只在首次调用时计算插件清单。
func (s *Server) searchDefaults() search.Options {
	s.initializationMu.Lock()
	defer s.initializationMu.Unlock()
	if s.searchBaseOptions == nil {
		base := search.DefaultOptions("")
		s.searchBaseOptions = &base
	}
	return *s.searchBaseOptions
}

// parseSettingBool 解析设置为布尔值，无法识别时保留原值。
func parseSettingBool(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

// splitSettingList 拆分逗号或空白分隔的设置列表。
func splitSettingList(v string) []string {
	v = strings.ReplaceAll(v, "，", ",")
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
