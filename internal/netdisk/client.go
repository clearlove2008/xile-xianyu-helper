// Package netdisk 实现夸克/百度网盘的「转存他人分享 → 生成自己分享链接」能力。
// 所有 HTTP 调用走可注入的 Doer，便于离线夹具测试；cookie 只作为参数传入，不进入包级状态。
package netdisk

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Doer 是最小 HTTP 执行端口，便于测试注入。
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ShareInput 是一次转存任务的输入。
type ShareInput struct {
	// ShareURL 是他人的分享链接。
	ShareURL string
	// Passcode 是源分享提取码，可为空。
	Passcode string
	// Cookie 是目标账号的登录凭证。
	Cookie string
	// Passcode 生成新分享时使用的提取码；为空时由平台随机生成。
	NewPasscode string
	// PeriodDays 是新分享有效期（0 永久 / 1 / 7 / 30）。
	PeriodDays int
}

// ShareResult 是一次转存任务的输出。
type ShareResult struct {
	// ShareURL 是生成的新分享链接。
	ShareURL string `json:"shareUrl"`
	// Passcode 是新分享的提取码，可为空表示无口令。
	Passcode string `json:"passcode"`
	// Title 是转存的文件标题。
	Title string `json:"title"`
	// FileCount 是转存的文件数量。
	FileCount int `json:"fileCount"`
	// Platform 是网盘平台标识。
	Platform string `json:"platform"`
}

// defaultHTTPClient 返回带超时的默认 HTTP 客户端。
func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// doJSON 执行 JSON 请求并把响应解码到 out。
func doJSON(ctx context.Context, doer Doer, method, target string, header http.Header, body io.Reader, out any) error {
	if doer == nil {
		doer = defaultHTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	if header != nil {
		req.Header = header.Clone()
	}
	resp, err := doer.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := truncate(string(raw), 200)
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("账号登录已失效，请在「网盘账号」里删除后重新扫码登录（HTTP 401: %s）", body)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("解析响应失败: %w (%s)", err, truncate(string(raw), 200))
	}
	return nil
}

// doForm 执行 application/x-www-form-urlencoded 请求。
func doForm(ctx context.Context, doer Doer, target string, header http.Header, form url.Values, out any) error {
	return doJSON(ctx, doer, http.MethodPost, target, header, strings.NewReader(form.Encode()), out)
}

// doGet 执行 GET 请求。
func doGet(ctx context.Context, doer Doer, target string, header http.Header, out any) error {
	return doJSON(ctx, doer, http.MethodGet, target, header, nil, out)
}

// jsonHeader 返回 JSON 请求头。
func jsonHeader(cookie, referer string) http.Header {
	h := make(http.Header)
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Content-Type", "application/json;charset=UTF-8")
	if referer != "" {
		h.Set("Referer", referer)
		h.Set("Origin", strings.TrimSuffix(strings.TrimSuffix(referer, "/"), "/"))
	}
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	return h
}

// formHeader 返回表单请求头。
func formHeader(cookie, referer string) http.Header {
	h := jsonHeader(cookie, referer)
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	return h
}

// truncate 截断字符串用于错误提示。
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// jsonBody 把任意值编码为 JSON reader。
func jsonBody(v any) (io.Reader, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(raw), nil
}

// jsonBytes 把任意值编码为 JSON 字节。
func jsonBytes(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal 解码 JSON 字节。
func jsonUnmarshal(raw []byte, out any) error {
	return json.Unmarshal(raw, out)
}

// decodeUnicode 把 JSON 字符串里的 Unicode 转义还原为可读文本，仅用于错误提示。
func decodeUnicode(s string) string {
	var out string
	if err := json.Unmarshal([]byte(strconv.Quote(s)), &out); err == nil {
		return out
	}
	return s
}

// RandomID 生成 n 位十六进制随机串，用于账号/会话 ID。
func RandomID(n int) string {
	const chars = "0123456789abcdef"
	buf := make([]byte, n)
	if _, err := crand.Read(buf); err != nil {
		return "00000000"
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = chars[int(b)%len(chars)]
	}
	return string(out)
}

// randomPasscode 生成 n 位网盘提取码，排除易混淆字符。
func randomPasscode(n int) string {
	const chars = "23456789abcdefghjkmnpqrstuvwxyz"
	buf := make([]byte, n)
	if _, err := crand.Read(buf); err != nil {
		return "abcd"
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = chars[int(b)%len(chars)]
	}
	return string(out)
}

// extractQuarkPwdID 从夸克分享链接提取 pwd_id（/s/ 后的 20~32 位 id）。
func extractQuarkPwdID(shareURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(shareURL))
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "s" && i+1 < len(parts) {
			id := strings.TrimSpace(parts[i+1])
			if id != "" {
				return id, nil
			}
		}
	}
	// 兼容不带 /s/ 前缀的短链。
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], nil
	}
	return "", fmt.Errorf("无法从链接提取夸克分享 ID: %s", shareURL)
}
