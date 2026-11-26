package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// studioRequest 构造一次出图工作台请求并返回响应录制器。
// 2xx 响应会按 OpenAPI 契约校验并记录成功覆盖，避免手写 operation 名单。
func studioRequest(t *testing.T, handler http.Handler, session *http.Cookie, method, path string, body []byte) *httptest.ResponseRecorder {
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
	// 仅版本化的出图端点参与 OpenAPI 契约覆盖。
	if strings.HasPrefix(path, "/api/v1/studio/") && recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices {
		assertOpenAPIRecordedSuccessResponse(t, request, recorder)
	}
	return recorder
}

// TestStudioHandlersContract 验证出图工作台端点在真实 Router 下的成功响应满足 OpenAPI 契约。
// 上游用本地 httptest 冒充 OpenAI 兼容接口，测试过程不访问外部网络。
func TestStudioHandlersContract(t *testing.T) {
	// srv、_、cleanup 分别是被测 HTTP 服务、测试数据库句柄和释放函数。
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	// handler 是完整路由树。
	handler := srv.Router()
	// session 是管理员会话 Cookie。
	session := loginHelper(t, handler)

	// upstream 冒充中转站：/models 返回模型清单，图像端点返回固定 base64。
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"},{"id":"gpt-image-2.5-sunburst"}]}`))
		case strings.HasSuffix(r.URL.Path, "/images/edits"), strings.HasSuffix(r.URL.Path, "/images/generations"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString([]byte("fake-png"))}},
			})
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"content": "改写后的文案"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	// 默认系统提示词：未登录时不可读，登录后必须返回非空文案。
	defaultsRec := studioRequest(t, handler, session, http.MethodGet, "/api/v1/studio/defaults", nil)
	if defaultsRec.Code != http.StatusOK {
		t.Fatalf("默认提示词端点失败 status=%d body=%s", defaultsRec.Code, defaultsRec.Body.String())
	}
	// defaults 解析默认提示词响应。
	var defaults struct {
		SystemPrompt string `json:"systemPrompt"`
	}
	if err := json.Unmarshal(defaultsRec.Body.Bytes(), &defaults); err != nil {
		t.Fatalf("解析默认提示词失败: %v", err)
	}
	if strings.TrimSpace(defaults.SystemPrompt) == "" {
		t.Fatal("默认系统提示词为空")
	}

	// 提示词预览：不调用上游即可返回组装结果。
	promptBody, promptErr := json.Marshal(map[string]any{
		"headline": "年终汇报", "price": "19.9", "points": []string{"100套模板"}, "hasImage": true,
	})
	if promptErr != nil {
		t.Fatalf("构造提示词请求失败: %v", promptErr)
	}
	promptRec := studioRequest(t, handler, session, http.MethodPost, "/api/v1/studio/prompt", promptBody)
	if promptRec.Code != http.StatusOK {
		t.Fatalf("提示词预览失败 status=%d body=%s", promptRec.Code, promptRec.Body.String())
	}
	// prompt 解析预览响应。
	var prompt struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(promptRec.Body.Bytes(), &prompt); err != nil {
		t.Fatalf("解析提示词预览失败: %v", err)
	}
	if !strings.Contains(prompt.Prompt, "年终汇报") || !strings.Contains(prompt.Prompt, "KEEP UNCHANGED") {
		t.Fatalf("提示词缺少关键约束: %s", prompt.Prompt)
	}

	// 模型清单：上游返回两个模型，接口必须原样透出。
	modelsBody, modelsErr := json.Marshal(map[string]any{"baseUrl": upstream.URL, "apiKey": "test-key"})
	if modelsErr != nil {
		t.Fatalf("构造模型请求失败: %v", modelsErr)
	}
	modelsRec := studioRequest(t, handler, session, http.MethodPost, "/api/v1/studio/models", modelsBody)
	if modelsRec.Code != http.StatusOK {
		t.Fatalf("模型清单失败 status=%d body=%s", modelsRec.Code, modelsRec.Body.String())
	}
	// models 解析模型清单响应。
	var models struct {
		Models []string `json:"models"`
		Count  int      `json:"count"`
	}
	if err := json.Unmarshal(modelsRec.Body.Bytes(), &models); err != nil {
		t.Fatalf("解析模型清单失败: %v", err)
	}
	if models.Count != 2 || len(models.Models) != 2 {
		t.Fatalf("模型清单不符合预期: %s", modelsRec.Body.String())
	}

	// 出图：带原图走编辑端点，响应必须给出 data URL 与提示词。
	generateBody, generateErr := json.Marshal(map[string]any{
		"baseUrl":  upstream.URL,
		"apiKey":   "test-key",
		"model":    "gpt-image-2",
		"fallback": "gpt-image-2.5-sunburst",
		"image":    "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("input-png")),
		"headline": "年终汇报",
		"price":    "19.9",
	})
	if generateErr != nil {
		t.Fatalf("构造出图请求失败: %v", generateErr)
	}
	generateRec := studioRequest(t, handler, session, http.MethodPost, "/api/v1/studio/generate", generateBody)
	if generateRec.Code != http.StatusOK {
		t.Fatalf("出图失败 status=%d body=%s", generateRec.Code, generateRec.Body.String())
	}
	// result 解析出图响应。
	var result struct {
		Model    string  `json:"model"`
		Seconds  float64 `json:"seconds"`
		Image    string  `json:"image"`
		Fallback bool    `json:"fallback"`
		Prompt   string  `json:"prompt"`
	}
	if err := json.Unmarshal(generateRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("解析出图响应失败: %v", err)
	}
	if result.Model != "gpt-image-2" || !strings.HasPrefix(result.Image, "data:image/png;base64,") {
		t.Fatalf("出图响应不符合预期: %s", generateRec.Body.String())
	}
	if result.Fallback {
		t.Fatal("首选模型成功时不应标记回退")
	}

	// 文案润色：文字模型与图片模型各自独立的地址和密钥，此处复用同一上游桩。
	polishBody, polishErr := json.Marshal(map[string]any{
		"baseUrl": upstream.URL, "apiKey": "test-key", "model": "gpt-4o-mini",
		"copy": "年终汇报PPT模板 100套 可编辑", "shipping": "电子资料发出不退",
	})
	if polishErr != nil {
		t.Fatalf("构造润色请求失败: %v", polishErr)
	}
	polishRec := studioRequest(t, handler, session, http.MethodPost, "/api/v1/studio/polish", polishBody)
	if polishRec.Code != http.StatusOK {
		t.Fatalf("文案润色失败 status=%d body=%s", polishRec.Code, polishRec.Body.String())
	}
	// polished 解析润色响应。
	var polished struct {
		Copy  string `json:"copy"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(polishRec.Body.Bytes(), &polished); err != nil {
		t.Fatalf("解析润色响应失败: %v", err)
	}
	if polished.Copy == "" || polished.Model != "gpt-4o-mini" {
		t.Fatalf("润色响应不符合预期: %s", polishRec.Body.String())
	}
}

// TestStudioGenerateRejectsMissingModel 验证未配置模型时返回 400 而不是请求上游。
func TestStudioGenerateRejectsMissingModel(t *testing.T) {
	// srv、_、cleanup 分别是被测 HTTP 服务、测试数据库句柄和释放函数。
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	// handler 是完整路由树。
	handler := srv.Router()
	// session 是管理员会话 Cookie。
	session := loginHelper(t, handler)

	// body 缺少 model 字段，必须在调用上游前被拒绝。
	body, err := json.Marshal(map[string]any{"baseUrl": "http://127.0.0.1:1/v1", "headline": "测试"})
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if rec := studioRequest(t, handler, session, http.MethodPost, "/api/v1/studio/generate", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺少模型时应返回 400，实际 status=%d body=%s", rec.Code, rec.Body.String())
	}
}
