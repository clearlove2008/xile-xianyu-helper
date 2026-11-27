package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// searchRequest 构造一次网盘搜索相关请求并返回响应录制器。
// 2xx 响应会按 OpenAPI 契约校验并记录成功覆盖，避免手写 operation 名单。
func searchRequest(t *testing.T, handler http.Handler, session *http.Cookie, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if session != nil {
		request.AddCookie(session)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	// 仅版本化的网盘搜索端点参与 OpenAPI 契约覆盖，设置端点属于未版本化历史路由。
	if strings.HasPrefix(path, "/api/v1/search/") && recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices {
		assertOpenAPIRecordedSuccessResponse(t, request, recorder)
	}
	return recorder
}

// TestSearchHandlersContract 验证网盘搜索端点在真实 Router 下的成功响应满足 OpenAPI 契约。
// 测试内先清空插件列表，确保搜索端点不会访问外部网络。
func TestSearchHandlersContract(t *testing.T) {
	// srv、_、cleanup 分别是被测 HTTP 服务、测试数据库句柄和释放函数。
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	// handler 是完整路由树。
	handler := srv.Router()
	// session 是管理员会话 Cookie。
	session := loginHelper(t, handler)

	// 清空插件列表，使搜索在没有外部依赖的情况下返回空结果。
	settingsBody, marshalErr := json.Marshal(map[string]any{"values": map[string]string{searchSettingPlugins: ""}})
	if marshalErr != nil {
		t.Fatalf("构造设置请求失败: %v", marshalErr)
	}
	if rec := searchRequest(t, handler, session, http.MethodPut, "/system-settings", settingsBody); rec.Code != http.StatusOK {
		t.Fatalf("清空插件配置失败 status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 配置端点：返回网盘类型字典与插件列表。
	optionsRec := searchRequest(t, handler, session, http.MethodGet, "/api/v1/search/options", nil)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("配置端点失败 status=%d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	// options 解析配置端点的真实响应，用于断言关键字段。
	var options struct {
		Enabled    bool `json:"enabled"`
		EnableTG   bool `json:"enable_tg"`
		CloudTypes []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		} `json:"cloud_types"`
		Plugins []string `json:"plugins"`
	}
	if err := json.Unmarshal(optionsRec.Body.Bytes(), &options); err != nil {
		t.Fatalf("解析配置响应失败: %v", err)
	}
	if !options.Enabled || len(options.CloudTypes) == 0 {
		t.Fatalf("配置响应缺少网盘类型: %s", optionsRec.Body.String())
	}
	if len(options.Plugins) != 0 {
		t.Fatalf("清空插件后仍返回插件: %v", options.Plugins)
	}

	// POST 检索：无插件时返回空结果而不是错误。
	postBody, postErr := json.Marshal(map[string]any{"kw": "测试", "res": "merge"})
	if postErr != nil {
		t.Fatalf("构造检索请求失败: %v", postErr)
	}
	if rec := searchRequest(t, handler, session, http.MethodPost, "/api/v1/search/search", postBody); rec.Code != http.StatusOK {
		t.Fatalf("POST 检索失败 status=%d body=%s", rec.Code, rec.Body.String())
	}

	// GET 检索：查询参数形式与 POST 等价。
	if rec := searchRequest(t, handler, session, http.MethodGet, "/api/v1/search/search?kw=%E6%B5%8B%E8%AF%95&res=merge", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET 检索失败 status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSearchSearchRejectsEmptyKeyword 验证空关键词返回 400 而不是空结果。
func TestSearchSearchRejectsEmptyKeyword(t *testing.T) {
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	handler := srv.Router()
	session := loginHelper(t, handler)

	rec := searchRequest(t, handler, session, http.MethodPost, "/api/v1/search/search", []byte(`{"kw":"   "}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空关键词应返回 400，实际 status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestSearchSearchRequiresSession 验证未登录无法访问网盘搜索，认证复用闲鱼助手会话。
func TestSearchSearchRequiresSession(t *testing.T) {
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	handler := srv.Router()

	rec := searchRequest(t, handler, nil, http.MethodPost, "/api/v1/search/search", []byte(`{"kw":"测试"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 status=%d body=%s", rec.Code, rec.Body.String())
	}
}
