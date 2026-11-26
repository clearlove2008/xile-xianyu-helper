// Package pansou 提供网盘资源搜索能力，搜索内核改写自 PanSou（MIT 许可）。
//
// 本包只负责搜索域的编排：配置注入、插件注册、搜索执行与结果整形。
// HTTP 传输层由同包的 handler 以 chi 形式提供，认证复用闲鱼助手的会话体系，
// 不引入 PanSou 原有的 JWT 与 gin 依赖。
package pansou

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"xianyu-go/internal/pansou/config"
	"xianyu-go/internal/pansou/model"
	"xianyu-go/internal/pansou/plugin"
	"xianyu-go/internal/pansou/service"
	"xianyu-go/internal/pansou/util"
	"xianyu-go/internal/pansou/util/cache"

	// 插件通过 init() 自注册到全局注册表，因此必须空导入以保留副作用。
	_ "xianyu-go/internal/pansou/plugin/ahhhhfs"
	_ "xianyu-go/internal/pansou/plugin/aikanzy"
	_ "xianyu-go/internal/pansou/plugin/alupan"
	_ "xianyu-go/internal/pansou/plugin/ash"
	_ "xianyu-go/internal/pansou/plugin/bixin"
	_ "xianyu-go/internal/pansou/plugin/cldi"
	_ "xianyu-go/internal/pansou/plugin/clmao"
	_ "xianyu-go/internal/pansou/plugin/clxiong"
	_ "xianyu-go/internal/pansou/plugin/cyg"
	_ "xianyu-go/internal/pansou/plugin/daishudj"
	_ "xianyu-go/internal/pansou/plugin/ddys"
	_ "xianyu-go/internal/pansou/plugin/discourse"
	_ "xianyu-go/internal/pansou/plugin/djgou"
	_ "xianyu-go/internal/pansou/plugin/duoduo"
	_ "xianyu-go/internal/pansou/plugin/dyyj"
	_ "xianyu-go/internal/pansou/plugin/erxiao"
	_ "xianyu-go/internal/pansou/plugin/feikuai"
	_ "xianyu-go/internal/pansou/plugin/fox4k"
	_ "xianyu-go/internal/pansou/plugin/haisou"
	_ "xianyu-go/internal/pansou/plugin/hdmoli"
	_ "xianyu-go/internal/pansou/plugin/hdr4k"
	_ "xianyu-go/internal/pansou/plugin/huban"
	_ "xianyu-go/internal/pansou/plugin/hunhepan"
	_ "xianyu-go/internal/pansou/plugin/javdb"
	_ "xianyu-go/internal/pansou/plugin/jikepan"
	_ "xianyu-go/internal/pansou/plugin/jsnoteclub"
	_ "xianyu-go/internal/pansou/plugin/jutoushe"
	_ "xianyu-go/internal/pansou/plugin/kkmao"
	_ "xianyu-go/internal/pansou/plugin/kkv"
	_ "xianyu-go/internal/pansou/plugin/labi"
	_ "xianyu-go/internal/pansou/plugin/leijing"
	_ "xianyu-go/internal/pansou/plugin/libvio"
	_ "xianyu-go/internal/pansou/plugin/lou1"
	_ "xianyu-go/internal/pansou/plugin/meitizy"
	_ "xianyu-go/internal/pansou/plugin/miaoso"
	_ "xianyu-go/internal/pansou/plugin/mikuclub"
	_ "xianyu-go/internal/pansou/plugin/mizixing"
	_ "xianyu-go/internal/pansou/plugin/muou"
	_ "xianyu-go/internal/pansou/plugin/nsgame"
	_ "xianyu-go/internal/pansou/plugin/nyaa"
	_ "xianyu-go/internal/pansou/plugin/ouge"
	_ "xianyu-go/internal/pansou/plugin/pan666"
	_ "xianyu-go/internal/pansou/plugin/pansearch"
	_ "xianyu-go/internal/pansou/plugin/panta"
	_ "xianyu-go/internal/pansou/plugin/panwiki"
	_ "xianyu-go/internal/pansou/plugin/panyq"
	_ "xianyu-go/internal/pansou/plugin/pianku"
	_ "xianyu-go/internal/pansou/plugin/qingying"
	_ "xianyu-go/internal/pansou/plugin/quark4k"
	_ "xianyu-go/internal/pansou/plugin/quarksoo"
	_ "xianyu-go/internal/pansou/plugin/qupanshe"
	_ "xianyu-go/internal/pansou/plugin/qupansou"
	_ "xianyu-go/internal/pansou/plugin/sdso"
	_ "xianyu-go/internal/pansou/plugin/shandian"
	_ "xianyu-go/internal/pansou/plugin/sousou"
	_ "xianyu-go/internal/pansou/plugin/susu"
	_ "xianyu-go/internal/pansou/plugin/thepiratebay"
	_ "xianyu-go/internal/pansou/plugin/u3c3"
	_ "xianyu-go/internal/pansou/plugin/wanou"
	_ "xianyu-go/internal/pansou/plugin/wuji"
	_ "xianyu-go/internal/pansou/plugin/xb6v"
	_ "xianyu-go/internal/pansou/plugin/xdpan"
	_ "xianyu-go/internal/pansou/plugin/xdyh"
	_ "xianyu-go/internal/pansou/plugin/xiaoji"
	_ "xianyu-go/internal/pansou/plugin/xiaozhang"
	_ "xianyu-go/internal/pansou/plugin/xinjuc"
	_ "xianyu-go/internal/pansou/plugin/xuebapan"
	_ "xianyu-go/internal/pansou/plugin/xuexizhinan"
	_ "xianyu-go/internal/pansou/plugin/xys"
	_ "xianyu-go/internal/pansou/plugin/yiove"
	_ "xianyu-go/internal/pansou/plugin/ypfxw"
	_ "xianyu-go/internal/pansou/plugin/yuhuage"
	_ "xianyu-go/internal/pansou/plugin/yunsou"
	_ "xianyu-go/internal/pansou/plugin/zhizhen"
	_ "xianyu-go/internal/pansou/plugin/zxzj"
)

// Options 是搜索内核的运行参数，来源为闲鱼助手的系统设置。
type Options struct {
	// Enabled 表示网盘搜索功能是否对用户开放。
	Enabled bool
	// Plugins 是启用的插件名称列表；空表示不启用任何插件。
	Plugins []string
	// Channels 是默认搜索的 Telegram 频道列表。
	Channels []string
	// ProxyURL 是 SOCKS5/HTTP 代理地址，用于访问被限制的搜索源。
	ProxyURL string
	// CachePath 是二级缓存的磁盘目录。
	CachePath string
	// CacheTTLMinutes 是搜索结果缓存有效期。
	CacheTTLMinutes int
	// PluginTimeoutSeconds 是单个插件的执行超时。
	PluginTimeoutSeconds int
	// ResponseTimeoutSeconds 是异步插件的快速响应超时。
	ResponseTimeoutSeconds int
	// EnableTG 控制是否启用 Telegram 频道抓取。
	EnableTG bool
}

// signature 生成影响 SearchService 构造的配置指纹，用于判断是否需要重建服务。
func (o Options) signature() string {
	plugins := append([]string(nil), o.Plugins...)
	sort.Strings(plugins)
	channels := append([]string(nil), o.Channels...)
	sort.Strings(channels)
	return fmt.Sprintf("p=%s|c=%s|x=%s|tg=%t|pc=%d", strings.Join(plugins, ","), strings.Join(channels, ","), o.ProxyURL, o.EnableTG, o.PluginTimeoutSeconds)
}

// DefaultOptions 返回可用的默认配置：不启用 TG、启用全部内置插件、缓存落在用户缓存目录。
// dataDir 为空时回退到 os.UserCacheDir()，避免依赖进程工作目录。
func DefaultOptions(dataDir string) Options {
	if strings.TrimSpace(dataDir) == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			dataDir = filepath.Join(cacheDir, "ydisks-xianyu-helper")
		} else {
			dataDir = "."
		}
	}
	all := RegisteredPluginNames()
	return Options{
		Enabled:                true,
		Plugins:                all,
		Channels:               nil,
		ProxyURL:               "",
		CachePath:              filepath.Join(dataDir, "pansou-cache"),
		CacheTTLMinutes:        60,
		PluginTimeoutSeconds:   30,
		ResponseTimeoutSeconds: 4,
		EnableTG:               false,
	}
}

// RegisteredPluginNames 返回所有已自注册的插件名称，按字典序排列。
// 该函数依赖插件包的 init() 副作用，因此只能在包初始化完成后调用。
func RegisteredPluginNames() []string {
	plugins := plugin.GetRegisteredPlugins()
	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.Name())
	}
	sort.Strings(names)
	return names
}

// Engine 持有搜索内核的进程级状态，并负责按配置变更重建搜索服务。
// Engine 的所有导出方法都是并发安全的。
type Engine struct {
	// mu 保护 infraReady、service 与 activeSig，避免并发请求重建同一份服务。
	mu sync.Mutex
	// infraReady 表示 config、HTTP 客户端与缓存管理器已完成一次性初始化。
	infraReady bool
	// service 是当前生效的搜索服务；nil 表示尚未按配置构造。
	service *service.SearchService
	// activeSig 是 service 对应的配置指纹。
	activeSig string
	// pluginNames 是最近一次注册成功的插件名，供设置页展示。
	pluginNames []string
}

// NewEngine 构造尚未初始化的搜索引擎；实际的网络与缓存初始化推迟到首次搜索。
func NewEngine() *Engine {
	return &Engine{}
}

// ensureInfra 完成一次性基础设施初始化。config 与 HTTP 传输必须早于插件注册，
// 因为插件在构造时会读取超时与代理配置。
func (e *Engine) ensureInfra(opts Options) error {
	if e.infraReady {
		return nil
	}
	config.Init()
	applyOptions(opts)
	util.InitHTTPClient()

	// 缓存写入管理器负责把异步插件的增量结果落盘，失败时降级为无落盘模式。
	manager, err := cache.NewDelayedBatchWriteManager()
	if err == nil {
		if initErr := manager.Initialize(); initErr != nil {
			log.Printf("[pansou] 缓存写入管理器初始化失败，降级为纯内存缓存: %v", initErr)
		} else {
			service.SetGlobalCacheWriteManager(manager)
		}
	} else {
		log.Printf("[pansou] 缓存写入管理器创建失败，降级为纯内存缓存: %v", err)
	}

	plugin.InitAsyncPluginSystem()
	e.infraReady = true
	return nil
}

// applyOptions 用闲鱼助手的设置覆盖 PanSou 的环境变量默认值。
func applyOptions(opts Options) {
	if config.AppConfig == nil {
		return
	}
	proxyURL := normalizeProxyURL(opts.ProxyURL)
	config.AppConfig.ProxyURL = proxyURL
	config.AppConfig.UseProxy = proxyURL != ""
	config.AppConfig.HTTPProxyURL = proxyURL
	config.AppConfig.HTTPSProxyURL = proxyURL
	if opts.CachePath != "" {
		config.AppConfig.CachePath = opts.CachePath
	}
	if opts.CacheTTLMinutes > 0 {
		config.AppConfig.CacheTTLMinutes = opts.CacheTTLMinutes
	}
	if opts.PluginTimeoutSeconds > 0 {
		config.AppConfig.PluginTimeoutSeconds = opts.PluginTimeoutSeconds
		config.AppConfig.PluginTimeout = time.Duration(opts.PluginTimeoutSeconds) * time.Second
	}
	if opts.ResponseTimeoutSeconds > 0 {
		config.AppConfig.AsyncResponseTimeout = opts.ResponseTimeoutSeconds
		config.AppConfig.AsyncResponseTimeoutDur = time.Duration(opts.ResponseTimeoutSeconds) * time.Second
	}
	config.AppConfig.AsyncPluginEnabled = len(opts.Plugins) > 0
	config.AppConfig.EnabledPlugins = opts.Plugins
}

// current 返回按配置就绪的搜索服务；配置指纹变化时重建插件管理器与搜索服务。
func (e *Engine) current(opts Options) (*service.SearchService, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureInfra(opts); err != nil {
		return nil, err
	}
	sig := opts.signature()
	if e.service != nil && e.activeSig == sig {
		return e.service, nil
	}

	// 配置指纹变化意味着代理、频道或超时发生了修改。
	// 必须重新应用配置并重建 HTTP 客户端：传输层在初始化时固化代理设置，
	// 只换插件管理器而不换客户端，会让代理配置永远无法生效。
	applyOptions(opts)
	util.InitHTTPClient()
	if proxy := config.AppConfig.ProxyURL; proxy != "" {
		log.Printf("[pansou] 搜索客户端已应用代理 %s（TG=%t，频道=%v）", proxy, opts.EnableTG, opts.Channels)
	} else if opts.EnableTG {
		log.Printf("[pansou] 未配置代理，Telegram 抓取将走直连")
	}

	manager := plugin.NewPluginManager()
	manager.RegisterGlobalPluginsWithFilter(opts.Plugins)
	registered := make([]string, 0, len(manager.GetPlugins()))
	for _, p := range manager.GetPlugins() {
		registered = append(registered, p.Name())
	}
	sort.Strings(registered)
	config.UpdateDefaultConcurrency(len(registered))

	e.service = service.NewSearchService(manager)
	e.activeSig = sig
	e.pluginNames = registered
	return e.service, nil
}

// SearchRequest 描述一次搜索的用户输入，字段语义与 PanSou 的搜索 API 保持一致。
type SearchRequest struct {
	// Keyword 是搜索关键词，必填。
	Keyword string `json:"kw"`
	// Channels 指定本次搜索的 Telegram 频道，为空则使用默认频道。
	Channels []string `json:"channels"`
	// Concurrency 是并发数，0 表示自动。
	Concurrency int `json:"conc"`
	// Refresh 为 true 时跳过缓存强制刷新。
	Refresh bool `json:"refresh"`
	// ResultType 控制返回结构：all、results 或 merge。
	ResultType string `json:"res"`
	// SourceType 控制数据来源：all、tg 或 plugin。
	SourceType string `json:"src"`
	// Plugins 指定本次使用的插件，为空则使用全部已启用插件。
	Plugins []string `json:"plugins"`
	// CloudTypes 指定返回的网盘类型。
	CloudTypes []string `json:"cloud_types"`
	// Ext 是透传给插件的扩展参数。
	Ext map[string]interface{} `json:"ext"`
	// Filter 是结果过滤规则。
	Filter *SearchFilter `json:"filter"`
}

// SearchFilter 描述结果包含与排除关键词。
type SearchFilter struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// Search 执行一次搜索并按请求完成结果整形与过滤。
func (e *Engine) Search(opts Options, req SearchRequest) (model.SearchResponse, error) {
	// 未启用 TG 时清空频道，避免每条请求等待必然超时的境外抓取。
	// 请求未显式指定频道时回退到服务端配置的默认频道（设置页维护）。
	channels := req.Channels
	if len(channels) == 0 {
		channels = opts.Channels
	}
	channels = normalizeChannels(channels)
	sourceType := req.SourceType
	if !opts.EnableTG {
		channels = nil
		if sourceType == "" || sourceType == "all" || sourceType == "tg" {
			sourceType = "plugin"
		}
	} else if len(channels) == 0 {
		// 已启用 TG 但没有任何频道可搜时，跳过 tg 分支而不是空跑。
		if sourceType == "tg" {
			return model.SearchResponse{}, errors.New("未配置 Telegram 频道")
		}
		if sourceType == "" || sourceType == "all" {
			sourceType = "plugin"
		}
	}
	if sourceType == "" {
		sourceType = "all"
	}

	svc, err := e.current(opts)
	if err != nil {
		return model.SearchResponse{}, err
	}
	resp, err := svc.Search(req.Keyword, channels, req.Concurrency, req.Refresh, normalizeResultType(req.ResultType), sourceType, req.Plugins, req.CloudTypes, req.Ext)
	if err != nil {
		return model.SearchResponse{}, err
	}
	if req.Filter != nil {
		resp = applyFilter(resp, *req.Filter)
	}
	return resp, nil
}

// normalizeResultType 将空值归一到 merge，其余取值原样透传由内核判定。
func normalizeResultType(v string) string {
	switch v {
	case "", "merge":
		return "merge"
	case "all", "results":
		return v
	default:
		return "merge"
	}
}

// applyFilter 按包含与排除关键词裁剪结果集，合并视图与原始视图同步处理。
func applyFilter(resp model.SearchResponse, filter SearchFilter) model.SearchResponse {
	include := normalizeKeywords(filter.Include)
	exclude := normalizeKeywords(filter.Exclude)
	if len(include) == 0 && len(exclude) == 0 {
		return resp
	}

	keep := func(text string) bool {
		lower := strings.ToLower(text)
		if len(include) > 0 {
			matched := false
			for _, kw := range include {
				if strings.Contains(lower, kw) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		for _, kw := range exclude {
			if strings.Contains(lower, kw) {
				return false
			}
		}
		return true
	}

	results := make([]model.SearchResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		if keep(item.Title + " " + item.Content) {
			results = append(results, item)
		}
	}

	merged := make(model.MergedLinks, len(resp.MergedByType))
	for kind, links := range resp.MergedByType {
		kept := make([]model.MergedLink, 0, len(links))
		for _, link := range links {
			if keep(link.Note) {
				kept = append(kept, link)
			}
		}
		if len(kept) > 0 {
			merged[kind] = kept
		}
	}

	resp.Results = results
	resp.MergedByType = merged
	resp.Total = len(results)
	if resp.Total == 0 {
		for _, links := range merged {
			resp.Total += len(links)
		}
	}
	return resp
}

// normalizeKeywords 去除空白关键词并转小写，便于不区分大小写匹配。
func normalizeKeywords(in []string) []string {
	out := make([]string, 0, len(in))
	for _, kw := range in {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" {
			out = append(out, kw)
		}
	}
	return out
}

// PluginNames 返回当前已注册的插件名称，供设置页展示可用数据源。
func (e *Engine) PluginNames() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.pluginNames) > 0 {
		return append([]string(nil), e.pluginNames...)
	}
	return RegisteredPluginNames()
}

// normalizeProxyURL 补全代理地址的协议头。
// 用户常直接填 "127.0.0.1:7890"（Clash 的混合端口），缺少协议头时
// url.Parse 会解析失败并把代理静默丢弃，导致本应走代理的请求全部直连超时。
// 缺省按 HTTP 代理处理：Clash/V2Ray 的混合端口同时支持 HTTP 与 SOCKS5 握手。
func normalizeProxyURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	return v
}

// normalizeChannels 把用户输入的频道描述归一化成 t.me/s/ 适用的频道名。
// 兼容三种常见输入：纯频道名（tgsearchers3）、带 @ 前缀（@tgsearchers3）、
// 完整链接（https://t.me/s/tgsearchers3 或 https://t.me/tgsearchers3）。
// 无法归一化的输入（例如纯 URL 无频道段）会被丢弃，避免拼出必然 404 的抓取地址。
func normalizeChannels(channels []string) []string {
	out := make([]string, 0, len(channels))
	for _, raw := range channels {
		name := strings.TrimSpace(raw)
		name = strings.TrimPrefix(name, "@")
		if i := strings.Index(name, "t.me/"); i >= 0 {
			name = name[i+len("t.me/"):]
		}
		name = strings.TrimPrefix(name, "s/")
		name = strings.TrimSuffix(name, "/")
		name = strings.TrimSpace(strings.TrimPrefix(name, "@"))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
