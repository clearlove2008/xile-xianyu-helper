package pansou

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"xianyu-go/internal/httpapi"
	"xianyu-go/internal/pansou/model"
)

// maxRequestBodyBytes 限制搜索请求体大小，避免异常客户端耗尽内存。
const maxRequestBodyBytes = 64 << 10

// OptionsProvider 从闲鱼助手的设置体系解析网盘搜索运行参数。
// 由 server 包注入，使 pansou 包不反向依赖传输层。
type OptionsProvider func(ctx context.Context, r *http.Request) (Options, error)

// Handler 提供网盘搜索的 HTTP 端点，认证由外层路由组统一负责。
type Handler struct {
	// engine 是进程级搜索引擎。
	engine *Engine
	// provider 在每次请求时解析最新配置，支持设置页修改后即时生效。
	provider OptionsProvider
}

// NewHandler 构造搜索端点处理器；provider 为 nil 时退化为默认配置。
func NewHandler(engine *Engine, provider OptionsProvider) *Handler {
	if engine == nil {
		engine = NewEngine()
	}
	return &Handler{engine: engine, provider: provider}
}

// resolveOptions 读取本次请求应使用的配置。
func (h *Handler) resolveOptions(r *http.Request) (Options, error) {
	if h.provider == nil {
		return DefaultOptions(""), nil
	}
	return h.provider(r.Context(), r)
}

// Search 处理 GET 与 POST 两种形式的搜索请求。
// GET 通过查询参数传参，POST 通过 JSON 请求体传参，两者语义一致。
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	opts, err := h.resolveOptions(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, httpapi.CodeInternalError, "读取搜索配置失败", requestID(r))
		return
	}
	if !opts.Enabled {
		httpapi.WriteError(w, http.StatusForbidden, httpapi.CodeForbidden, "网盘搜索功能已关闭", requestID(r))
		return
	}

	req, err := decodeSearchRequest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, err.Error(), requestID(r))
		return
	}
	if strings.TrimSpace(req.Keyword) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "关键词不能为空", requestID(r))
		return
	}

	resp, err := h.engine.Search(opts, req)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, httpapi.CodeBadGateway, "搜索源均未返回结果："+err.Error(), requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Options 返回当前可用的配置选项，供前端渲染搜索配置页。
// 响应同时给出插件列表与网盘类型字典，前端不再硬编码。
func (h *Handler) Options(w http.ResponseWriter, r *http.Request) {
	opts, err := h.resolveOptions(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, httpapi.CodeInternalError, "读取搜索配置失败", requestID(r))
		return
	}
	// plugins 反映生效配置：nil 表示设置中未定义该键，此时展示全部已注册插件；
	// 非 nil 的空切片表示用户明确关闭了全部插件。
	plugins := opts.Plugins
	if plugins == nil {
		plugins = h.engine.PluginNames()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":          opts.Enabled,
		"enable_tg":        opts.EnableTG,
		"channels":         opts.Channels,
		"plugins":          plugins,
		"cloud_types":      SupportedCloudTypes(),
		"proxy_configured": strings.TrimSpace(opts.ProxyURL) != "",
	})
}

// decodeSearchRequest 兼容 JSON 请求体与查询参数两种入参形式。
func decodeSearchRequest(r *http.Request) (SearchRequest, error) {
	if r.Method == http.MethodGet {
		return searchRequestFromQuery(r), nil
	}

	// 请求体按上限截断读取，超限时 JSON 解析会自然失败并返回参数错误。
	body := io.LimitReader(r.Body, maxRequestBodyBytes)
	defer r.Body.Close()
	var req SearchRequest
	decoder := json.NewDecoder(body)
	// 不启用 DisallowUnknownFields：前端版本可能领先于后端，未知字段应被忽略而非报错。
	if err := decoder.Decode(&req); err != nil {
		// 仅当请求体为空时回退到查询参数，真正的 JSON 语法错误仍需报错。
		if errors.Is(err, io.EOF) {
			return searchRequestFromQuery(r), nil
		}
		return SearchRequest{}, errors.New("请求格式错误")
	}
	return req, nil
}

// searchRequestFromQuery 从 URL 查询参数解析搜索请求。
func searchRequestFromQuery(r *http.Request) SearchRequest {
	q := r.URL.Query()
	req := SearchRequest{
		Keyword:    q.Get("kw"),
		Channels:   splitCSV(q.Get("channels")),
		ResultType: q.Get("res"),
		SourceType: q.Get("src"),
		Plugins:    splitCSV(q.Get("plugins")),
		CloudTypes: splitCSV(q.Get("cloud_types")),
		Refresh:    strings.EqualFold(q.Get("refresh"), "true"),
	}
	if v := q.Get("conc"); v != "" {
		if conc, err := strconv.Atoi(v); err == nil {
			req.Concurrency = conc
		}
	}
	if raw := q.Get("ext"); raw != "" {
		var ext map[string]interface{}
		if json.Unmarshal([]byte(raw), &ext) == nil {
			req.Ext = ext
		}
	}
	if raw := q.Get("filter"); raw != "" {
		var filter SearchFilter
		if json.Unmarshal([]byte(raw), &filter) == nil {
			req.Filter = &filter
		}
	}
	return req
}

// splitCSV 将逗号分隔参数拆成非空切片，空输入返回 nil。
func splitCSV(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// writeJSON 以统一 Content-Type 写入成功响应。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// requestID 提取由 RequestID 中间件写入的追踪标识。
func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.Header.Get("X-Request-Id")
}

// SupportedCloudTypes 返回前端可筛选的网盘类型字典，顺序用于展示优先级。
func SupportedCloudTypes() []map[string]string {
	return []map[string]string{
		{"key": "baidu", "label": "百度网盘"},
		{"key": "aliyun", "label": "阿里云盘"},
		{"key": "quark", "label": "夸克网盘"},
		{"key": "tianyi", "label": "天翼云盘"},
		{"key": "uc", "label": "UC网盘"},
		{"key": "mobile", "label": "移动云盘"},
		{"key": "115", "label": "115网盘"},
		{"key": "pikpak", "label": "PikPak"},
		{"key": "xunlei", "label": "迅雷网盘"},
		{"key": "123", "label": "123网盘"},
		{"key": "magnet", "label": "磁力链接"},
		{"key": "ed2k", "label": "电驴链接"},
		{"key": "others", "label": "其他"},
	}
}

// SearchResponse 是搜索结果的类型别名，便于同包内引用而不引入 model 的完整路径。
type SearchResponse = model.SearchResponse
