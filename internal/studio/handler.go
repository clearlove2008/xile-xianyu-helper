package studio

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"xianyu-go/internal/httpapi"
)

// maxRequestBodyBytes 限制单次出图请求体。原图以 base64 内联传输，留 48MB 余量。
const maxRequestBodyBytes = 48 << 20

// Handler 提供出图工作台的 HTTP 端点，认证由外层路由组统一负责。
// 模型地址与密钥随请求传入、用完即弃，服务端不落库也不写日志。
type Handler struct {
	// client 是中转站客户端。
	client *client
}

// NewHandler 构造出图端点处理器。
func NewHandler() *Handler {
	return &Handler{client: newClient()}
}

// modelRequest 是模型接入参数，图片与文字两侧共用同一结构。
type modelRequest struct {
	// BaseURL 是接口根地址。
	BaseURL string `json:"baseUrl"`
	// APIKey 是接口密钥。
	APIKey string `json:"apiKey"`
	// Model 是首选模型。
	Model string `json:"model"`
	// Fallback 是备用模型。
	Fallback string `json:"fallback"`
}

// toConfig 把请求参数转为内部配置。
func (m modelRequest) toConfig() ModelConfig {
	return ModelConfig{
		BaseURL:  m.BaseURL,
		APIKey:   m.APIKey,
		Model:    m.Model,
		Fallback: m.Fallback,
	}
}

// polishRequest 是文案润色请求体。
type polishRequest struct {
	modelRequest
	// Copy 是待润色的商品文案。
	Copy string `json:"copy"`
	// Shipping 是一键发货文字，作为语气参考。
	Shipping string `json:"shipping"`
}

// polishSystemPrompt 约束文字模型的输出形态：只改写，不解释、不加前后缀。
const polishSystemPrompt = `你是闲鱼二手交易平台的资深文案。请把用户给的资料类商品文案改写成闲鱼口语风格：
- 开头一句话说清"是什么、谁需要"，不要标题党
- 分段用短句，每段不超过三行，可用「■」分段
- 保留价格、交付方式、售后口径等事实，不许编造功能或承诺
- 结尾用一句短促的说明（如电子资料发出不退）
- 只输出改写后的文案正文，不要任何解释、不要 markdown 代码块`

// Polish 调用文字模型把文案改写成闲鱼口吻；未配置文字模型时由前端直接跳过。
func (h *Handler) Polish(w http.ResponseWriter, r *http.Request) {
	var req polishRequest
	if err := decodeJSON(r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请求格式错误", requestID(r))
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.Model) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请先在设置页填写文字模型接口地址与模型名", requestID(r))
		return
	}
	if strings.TrimSpace(req.Copy) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "待润色的文案不能为空", requestID(r))
		return
	}
	// user 是交给文字模型的输入，把发货口径作为事实一起给出，避免改写时跑偏。
	user := "商品文案：\n" + strings.TrimSpace(req.Copy)
	if v := strings.TrimSpace(req.Shipping); v != "" {
		user += "\n\n发货与售后口径（必须保持一致）：\n" + v
	}
	text, err := h.client.Chat(r.Context(), req.toConfig(), polishSystemPrompt, user)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, httpapi.CodeBadGateway, err.Error(), requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"copy": text, "model": req.Model})
}

// generateRequest 是出图请求体。
type generateRequest struct {
	modelRequest
	// Image 是 data URL 或裸 base64 原图；为空表示文生图。
	Image string `json:"image"`
	// SystemPrompt 是本次使用的系统提示词。
	SystemPrompt string `json:"systemPrompt"`
	// Headline 是主标题。
	Headline string `json:"headline"`
	// Price 是价格。
	Price string `json:"price"`
	// Points 是卖点清单。
	Points []string `json:"points"`
	// Extra 是补充要求。
	Extra string `json:"extra"`
	// Copy 是完整商品文案。
	Copy string `json:"copy"`
	// Shipping 是一键发货文字。
	Shipping string `json:"shipping"`
	// Size 是输出尺寸。
	Size string `json:"size"`
}

// promptRequest 是提示词预览请求体。
type promptRequest struct {
	// SystemPrompt 是本次使用的系统提示词。
	SystemPrompt string `json:"systemPrompt"`
	// Headline 是主标题。
	Headline string `json:"headline"`
	// Price 是价格。
	Price string `json:"price"`
	// Points 是卖点清单。
	Points []string `json:"points"`
	// Extra 是补充要求。
	Extra string `json:"extra"`
	// Copy 是完整商品文案。
	Copy string `json:"copy"`
	// Shipping 是一键发货文字。
	Shipping string `json:"shipping"`
	// HasImage 表示是否带原图。
	HasImage bool `json:"hasImage"`
}

// Models 读取上游可用模型清单，供设置页验证地址与密钥是否可用。
func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	var req modelRequest
	if err := decodeJSON(r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请求格式错误", requestID(r))
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "接口地址不能为空", requestID(r))
		return
	}
	models, err := h.client.ListModels(r.Context(), req.toConfig())
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, httpapi.CodeBadGateway, err.Error(), requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "count": len(models)})
}

// Preview 返回组装后的提示词，不调用模型，便于用户先确认指令是否正确。
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var req promptRequest
	if err := decodeJSON(r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请求格式错误", requestID(r))
		return
	}
	prompt := BuildPrompt(PromptFields{
		SystemPrompt: req.SystemPrompt,
		Headline:     req.Headline,
		Price:        req.Price,
		Points:       req.Points,
		Extra:        req.Extra,
		Copy:         req.Copy,
		Shipping:     req.Shipping,
		HasImage:     req.HasImage,
	})
	writeJSON(w, http.StatusOK, map[string]any{"prompt": prompt})
}

// Generate 执行一次出图。耗时由上游决定，前端需给出较长的等待提示。
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := decodeJSON(r, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请求格式错误", requestID(r))
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请先在设置页填写图片模型接口地址", requestID(r))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "请先在设置页填写图片模型名称", requestID(r))
		return
	}
	if strings.TrimSpace(req.Headline) == "" && strings.TrimSpace(req.Price) == "" && len(req.Points) == 0 && strings.TrimSpace(req.Extra) == "" {
		httpapi.WriteError(w, http.StatusBadRequest, httpapi.CodeBadRequest, "至少填写主标题、价格、卖点或补充要求中的一项", requestID(r))
		return
	}

	prompt := BuildPrompt(PromptFields{
		SystemPrompt: req.SystemPrompt,
		Headline:     req.Headline,
		Price:        req.Price,
		Points:       req.Points,
		Extra:        req.Extra,
		Copy:         req.Copy,
		Shipping:     req.Shipping,
		HasImage:     strings.TrimSpace(req.Image) != "",
	})

	result, err := h.client.Generate(r.Context(), req.toConfig(), GenerateRequest{
		Image:  req.Image,
		Prompt: prompt,
		Size:   req.Size,
	})
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, httpapi.CodeBadGateway, err.Error(), requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":    result.Model,
		"seconds":  result.Seconds,
		"image":    result.Image,
		"fallback": result.Fallback,
		"prompt":   prompt,
	})
}

// decodeJSON 解析请求体，拒绝空体与语法错误，但容忍未知字段以兼容前端灰度升级。
func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体为空")
	}
	body := io.LimitReader(r.Body, maxRequestBodyBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		return err
	}
	return nil
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
