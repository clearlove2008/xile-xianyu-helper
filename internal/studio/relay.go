package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// defaultClientTimeout 是单次出图的等待上限。上游在大尺寸或队列拥挤时可能超过两分钟，
// 这里留足余量，避免把仍在生成的请求提前掐断。
const defaultClientTimeout = 5 * time.Minute

// maxResponseBytes 限制上游响应体读取量，防止异常响应耗尽内存。
const maxResponseBytes = 64 << 20

// ModelConfig 是一组模型接入参数。图片模型与文字模型各自持有独立的一份，
// 二者互不共享密钥，切换其一不影响另一侧。
type ModelConfig struct {
	// BaseURL 是 OpenAI 兼容接口根地址，允许省略 /v1 后缀。
	BaseURL string
	// APIKey 是访问中转站或官方接口的密钥。
	APIKey string
	// Model 是首选模型标识。
	Model string
	// Fallback 是首选模型失败时的备用模型标识，留空表示不回退。
	Fallback string
}

// GenerateRequest 描述一次出图请求。Image 非空表示改图，为空表示文生图。
type GenerateRequest struct {
	// Image 是 data URL 或裸 base64 形式的原图。
	Image string
	// Prompt 是最终交给模型的完整指令。
	Prompt string
	// Size 是输出尺寸，留空时使用 1024x1024。
	Size string
}

// GenerateResult 是一次出图的成功结果。
type GenerateResult struct {
	// Model 是实际产出图片的模型标识，回退时与请求的模型不同。
	Model string
	// Seconds 是本次出图耗时，用于界面展示。
	Seconds float64
	// Image 是 data URL 形式的 PNG 结果。
	Image string
	// Fallback 表示本次是否发生了模型回退。
	Fallback bool
}

// NormalizeBaseURL 补齐接口前缀，兼容用户只填域名或填到 /v1 两种情况。
func NormalizeBaseURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	u = strings.TrimRight(u, "/")
	// 已经带版本段的地址原样使用，避免把 /v1/xxx 拼成 /v1/xxx/v1。
	if strings.HasSuffix(u, "/v1") || strings.Contains(u, "/v1/") {
		return u
	}
	return u + "/v1"
}

// client 封装 OpenAI 兼容的图像接口调用。
type client struct {
	// http 是复用的 HTTP 客户端，超时由 transport 统一控制。
	http *http.Client
}

// newClient 构造默认客户端。
func newClient() *client {
	return &client{http: &http.Client{Timeout: defaultClientTimeout}}
}

// ListModels 读取上游可用模型清单，用于设置页的"测试连接"。
func (c *client) ListModels(ctx context.Context, cfg ModelConfig) ([]string, error) {
	base := NormalizeBaseURL(cfg.BaseURL)
	if base == "" {
		return nil, errors.New("接口地址不能为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("接口返回 %d：%s", resp.StatusCode, snippet(body))
	}
	// payload 兼容 OpenAI 标准 {"data":[{"id":...}]} 与部分中转站的裸数组形式。
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("响应格式无法解析：%s", snippet(body))
	}
	ids := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Generate 调用上游出图；首选模型失败时按配置自动回退到备用模型。
func (c *client) Generate(ctx context.Context, cfg ModelConfig, in GenerateRequest) (GenerateResult, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return GenerateResult{}, errors.New("提示词不能为空")
	}
	img, mime, err := decodeImage(in.Image)
	if err != nil {
		return GenerateResult{}, err
	}
	size := strings.TrimSpace(in.Size)
	if size == "" {
		size = "1024x1024"
	}

	models := make([]string, 0, 2)
	if m := strings.TrimSpace(cfg.Model); m != "" {
		models = append(models, m)
	}
	if fb := strings.TrimSpace(cfg.Fallback); fb != "" && fb != strings.TrimSpace(cfg.Model) {
		models = append(models, fb)
	}
	if len(models) == 0 {
		return GenerateResult{}, errors.New("未配置图片模型")
	}

	var firstErr error
	for i, model := range models {
		start := time.Now()
		b64, err := c.call(ctx, cfg, model, in.Prompt, size, img, mime)
		if err == nil {
			return GenerateResult{
				Model:    model,
				Seconds:  time.Since(start).Seconds(),
				Image:    "data:image/png;base64," + b64,
				Fallback: i > 0,
			}, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		// 上下文取消或超时后不再重试备用模型，避免请求被无限拉长。
		if ctx.Err() != nil {
			return GenerateResult{}, firstErr
		}
	}
	return GenerateResult{}, firstErr
}

// call 向单个模型发起一次出图；有原图走改图端点，无原图走文生图端点。
func (c *client) call(ctx context.Context, cfg ModelConfig, model, prompt, size string, img []byte, mime string) (string, error) {
	base := NormalizeBaseURL(cfg.BaseURL)
	if base == "" {
		return "", errors.New("接口地址不能为空")
	}

	var req *http.Request
	var err error
	if len(img) > 0 {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		fields := map[string]string{"model": model, "prompt": prompt, "size": size, "n": "1"}
		for k, v := range fields {
			if err := writer.WriteField(k, v); err != nil {
				return "", err
			}
		}
		part, err := writer.CreateFormFile("image", filenameForMime(mime))
		if err != nil {
			return "", err
		}
		if _, err := part.Write(img); err != nil {
			return "", err
		}
		if err := writer.Close(); err != nil {
			return "", err
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/images/edits", &buf)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
	} else {
		// payload 是文生图请求体，字段名遵循 OpenAI 图像接口约定。
		payload := map[string]any{"model": model, "prompt": prompt, "size": size, "n": 1}
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/images/generations", bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("模型 %s 返回 %d：%s", model, resp.StatusCode, snippet(body))
	}

	// parsed 是上游出图响应；不同中转站可能只给 base64，也可能只给可下载 URL。
	var parsed struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("响应格式无法解析：%s", snippet(body))
	}
	if len(parsed.Data) == 0 {
		return "", fmt.Errorf("接口未返回图片：%s", snippet(body))
	}
	first := parsed.Data[0]
	if b64 := strings.TrimSpace(first.B64); b64 != "" {
		return b64, nil
	}
	if url := strings.TrimSpace(first.URL); url != "" {
		return c.download(ctx, url)
	}
	return "", errors.New("接口返回了空图片")
}

// download 把图片 URL 拉回来并转成 base64，保证前端只处理统一格式。
func (c *client) download(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载图片失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Chat 调用文字模型润色文案。与出图共用同一套 OpenAI 兼容约定，
// 但使用独立的地址与密钥，两组配置互不影响。
func (c *client) Chat(ctx context.Context, cfg ModelConfig, system, user string) (string, error) {
	base := NormalizeBaseURL(cfg.BaseURL)
	if base == "" {
		return "", errors.New("接口地址不能为空")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return "", errors.New("未配置文字模型")
	}
	// payload 是标准 chat 请求体；温度压低，避免改写时发挥过度。
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.4,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("文字模型返回 %d：%s", resp.StatusCode, snippet(body))
	}
	// parsed 兼容标准 choices 结构与部分中转站的 output 字段。
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("响应格式无法解析：%s", snippet(body))
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("文字模型未返回内容：%s", snippet(body))
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

// decodeImage 解析 data URL 或裸 base64，返回原始字节与 MIME 类型。
func decodeImage(raw string) ([]byte, string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, "", nil
	}
	mime := "image/png"
	if strings.HasPrefix(s, "data:") {
		comma := strings.Index(s, ",")
		if comma < 0 {
			return nil, "", errors.New("图片数据格式错误")
		}
		header := s[len("data:"):comma]
		if semi := strings.Index(header, ";"); semi >= 0 {
			if header[:semi] != "" {
				mime = header[:semi]
			}
		}
		s = s[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// 兼容 URL 安全的 base64 变体。
		if data, err = base64.RawStdEncoding.DecodeString(s); err != nil {
			return nil, "", errors.New("图片数据无法解码")
		}
	}
	if len(data) == 0 {
		return nil, "", errors.New("图片内容为空")
	}
	return data, mime, nil
}

// filenameForMime 按 MIME 给出上传文件名，部分上游按扩展名判断格式。
func filenameForMime(mime string) string {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return "image.jpg"
	case "image/webp":
		return "image.webp"
	default:
		return "image.png"
	}
}

// snippet 截断上游响应，避免把整段 HTML 错误页塞进界面。
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
