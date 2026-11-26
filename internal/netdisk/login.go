package netdisk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// 扫码登录各平台的页面与选择器。集中定义便于平台改版时快速调整。
const (
	quarkLoginURL = "https://pan.quark.cn/"
	// 夸克首页二维码是 canvas#react-qrcode-logo；旧选择器从未命中。
	quarkQRCanvas = "#react-qrcode-logo"
	// 百度首页需要先点登录且二维码不直接出现，passport 登录页会直接展示二维码。
	baiduLoginURL     = "https://passport.baidu.com/v2/?login&tpl=netdisk&u=https%3A%2F%2Fpan.baidu.com%2F"
	baiduQRSelector   = "img.tang-pass-qrcode-img"
	loginPollInterval = 2 * time.Second
	loginTimeout      = 4 * time.Minute
	qrRefreshInterval = 90 * time.Second
	qrWaitTimeout     = 15 * time.Second
	// 夸克在未登录时也会自动写入 b-user-id 等 cookie，不能拿它当登录成功标志；
	// 必须调用账号接口确认返回了真实用户。
	// 登录态校验最多每 4 秒发一次，避免每轮轮询都打接口。
	loginCheckInterval = 4 * time.Second
)

// quarkAccountInfoURL 是校验夸克登录态的接口地址，测试时替换为本地服务。
var quarkAccountInfoURL = "https://pan.quark.cn/account/info?fr=pc&platform=pc"

// loginVerifier 判断一组 cookie 是否处于真实登录态。
// 用函数类型便于注入假实现做离线测试。
type loginVerifier func(ctx context.Context, platform, cookie string) (bool, error)

// verifyLoginByAccount 通过平台账号接口确认 cookie 是登录态，而不是游客态。
// 夸克未扫码也会有 b-user-id，只靠 cookie 名称判断会把游客凭证当成登录成功。
func verifyLoginByAccount(ctx context.Context, platform, cookie string) (bool, error) {
	if strings.TrimSpace(cookie) == "" {
		return false, nil
	}
	switch platform {
	case "quark":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, quarkAccountInfoURL, nil)
		if err != nil {
			return false, err
		}
		req.Header.Set("Cookie", cookie)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
		req.Header.Set("Referer", quarkShareReferer)
		resp, err := defaultHTTPClient().Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close() //nolint:errcheck
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return false, fmt.Errorf("校验夸克登录态失败: HTTP %d", resp.StatusCode)
		}
		var raw struct {
			Data struct {
				Nickname string `json:"nickname"`
				UserID   string `json:"user_id"`
			} `json:"data"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return false, fmt.Errorf("解析夸克账号信息失败: %w", err)
		}
		// 游客态返回 data:{}；登录后返回昵称或 user_id。
		return raw.Data.Nickname != "" || raw.Data.UserID != "", nil
	case "baidu":
		// 百度的 bdstoken 只有登录后才会返回，用它校验登录态。
		client := &BaiduClient{Doer: defaultHTTPClient()}
		token, err := client.baiduGetBdstoken(ctx, cookie)
		if err != nil {
			return false, err
		}
		return token != "", nil
	default:
		return false, fmt.Errorf("不支持的网盘平台: %s", platform)
	}
}

// collectPlatformCookie 从浏览器上下文拼出 cookie 字符串。
func collectPlatformCookie(bctx playwright.BrowserContext) (string, error) {
	cookies, err := bctx.Cookies()
	if err != nil || len(cookies) == 0 {
		return "", fmt.Errorf("浏览器中没有可用 cookie")
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; "), nil
}

// QRLoginStatus 是扫码登录会话对外暴露的状态。
type QRLoginStatus struct {
	// SessionID 是会话标识。
	SessionID string `json:"sessionId"`
	// Platform 是 netdisk 平台（quark/baidu）。
	Platform string `json:"platform"`
	// State 是状态：waiting / scanned / success / failed / expired。
	State string `json:"state"`
	// QRDataURL 是二维码图片（data:image/png;base64,...）。
	QRDataURL string `json:"qrDataUrl,omitempty"`
	// Cookie 是登录成功后提取的凭证，仅 success 状态返回。
	Cookie string `json:"cookie,omitempty"`
	// Error 是失败原因。
	Error string `json:"error,omitempty"`
}

// QRLoginSession 是一次扫码登录会话。
type QRLoginSession struct {
	mu        sync.Mutex
	id        string
	platform  string
	state     string
	qrDataURL string
	cookie    string
	errText   string
	createdAt time.Time
	cancel    context.CancelFunc
}

// Snapshot 返回会话的对外快照。
func (s *QRLoginSession) Snapshot() QRLoginStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := QRLoginStatus{
		SessionID: s.id,
		Platform:  s.platform,
		State:     s.state,
		QRDataURL: s.qrDataURL,
		Cookie:    s.cookie,
		Error:     s.errText,
	}
	// cookie 只在 success 时暴露给调用方持久化。
	if s.state != "success" {
		status.Cookie = ""
	}
	return status
}

func (s *QRLoginSession) setState(state, qr, cookie, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	if qr != "" {
		s.qrDataURL = qr
	}
	if cookie != "" {
		s.cookie = cookie
	}
	if errText != "" {
		s.errText = errText
	}
}

// hasQR 返回是否已经抓到二维码。
func (s *QRLoginSession) hasQR() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.qrDataURL != ""
}

// QRLoginManager 管理多个并发扫码会话。
type QRLoginManager struct {
	mu       sync.Mutex
	sessions map[string]*QRLoginSession
	// newID 可注入用于测试。
	newID func() string
	// browserStarter 可注入用于测试；nil 时使用真实 Playwright。
	browserStarter QRBrowserStarter
}

// QRBrowserStarter 抽象浏览器启动，便于测试注入假实现。
type QRBrowserStarter interface {
	// Start 启动登录流程，持续更新会话状态直到成功/失败/取消。
	Start(ctx context.Context, session *QRLoginSession, platform string) error
}

// NewQRLoginManager 创建扫码登录管理器。
func NewQRLoginManager() *QRLoginManager {
	return &QRLoginManager{
		sessions: make(map[string]*QRLoginSession),
		newID:    func() string { return randomPasscode(16) },
	}
}

// Begin 创建并发起一次扫码登录。平台必须为 quark/baidu。
func (m *QRLoginManager) Begin(ctx context.Context, platform string) (QRLoginStatus, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "quark" && platform != "baidu" {
		return QRLoginStatus{}, fmt.Errorf("不支持的网盘平台: %s", platform)
	}
	id := m.newID()
	session := &QRLoginSession{id: id, platform: platform, state: "waiting", createdAt: time.Now()}
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	// 扫码是跨请求的长会话：发起请求的 HTTP handler 返回后 request context 会被取消，
	// 若直接继承该取消信号，浏览器还没抓到二维码会话就被终止，前端会一直停在生成中。
	// 这里只保留 context 中的值，生命周期由 Cancel 和 Start 内部的超时控制。
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	session.mu.Lock()
	session.cancel = cancel
	session.mu.Unlock()

	starter := m.browserStarter
	if starter == nil {
		starter = &playwrightQRStarter{}
	}
	go func() {
		defer cancel()
		if err := starter.Start(runCtx, session, platform); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			session.setState("failed", "", "", err.Error())
		}
	}()
	return session.Snapshot(), nil
}

// Status 查询会话状态。
func (m *QRLoginManager) Status(sessionID string) (QRLoginStatus, error) {
	m.mu.Lock()
	session := m.sessions[sessionID]
	m.mu.Unlock()
	if session == nil {
		return QRLoginStatus{}, ErrSessionNotFound
	}
	return session.Snapshot(), nil
}

// Cancel 取消会话并删除。
func (m *QRLoginManager) Cancel(sessionID string) error {
	m.mu.Lock()
	session := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if session == nil {
		return ErrSessionNotFound
	}
	session.mu.Lock()
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()
	return nil
}

// Cleanup 清理超过 maxAge 的会话。
func (m *QRLoginManager) Cleanup(maxAge time.Duration) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if now.Sub(session.createdAt) > maxAge {
			session.mu.Lock()
			if session.cancel != nil {
				session.cancel()
			}
			session.mu.Unlock()
			delete(m.sessions, id)
		}
	}
}

// ErrSessionNotFound 表示扫码会话不存在或已过期。
var ErrSessionNotFound = errors.New("扫码会话不存在或已过期")

// browserExecutable 返回本机可用的浏览器路径；未找到时返回 nil，让 Playwright 使用自带 Chromium。
// 服务运行在 Windows Session 0，无法启动有界面窗口，这里只跑无头模式。
// 可用 NETDISK_CHROME_PATH 显式指定浏览器。
func browserExecutable() *string {
	if p := strings.TrimSpace(os.Getenv("NETDISK_CHROME_PATH")); p != "" {
		return playwright.String(p)
	}
	for _, p := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return playwright.String(p)
		}
	}
	return nil
}

// playwrightQRStarter 使用 Playwright 抓二维码并轮询登录态。
type playwrightQRStarter struct {
	// verifier 校验 cookie 是否为真实登录态；nil 时使用账号接口校验。
	verifier loginVerifier
}

// Start 实现 QRBrowserStarter。
func (p *playwrightQRStarter) Start(ctx context.Context, session *QRLoginSession, platform string) error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("启动 playwright 失败: %w", err)
	}
	defer pw.Stop() //nolint:errcheck

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		// 二维码通过 data URL 回传给前端展示，服务端不需要可见窗口。
		// Windows 服务在 Session 0，headful 模式根本起不来，必须无头。
		Headless:       playwright.Bool(true),
		ExecutablePath: browserExecutable(),
		Args: []string{
			"--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage",
			"--disable-blink-features=AutomationControlled", "--lang=zh-CN",
		},
	})
	if err != nil {
		return fmt.Errorf("启动浏览器失败: %w", err)
	}
	defer browser.Close() //nolint:errcheck

	bctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport:   &playwright.Size{Width: 1100, Height: 760},
		Locale:     playwright.String("zh-CN"),
		TimezoneId: playwright.String("Asia/Shanghai"),
	})
	if err != nil {
		return fmt.Errorf("创建浏览器上下文失败: %w", err)
	}
	defer bctx.Close() //nolint:errcheck

	page, err := bctx.NewPage()
	if err != nil {
		return fmt.Errorf("创建页面失败: %w", err)
	}

	loginURL := quarkLoginURL
	if platform == "baidu" {
		loginURL = baiduLoginURL
	}
	if _, err := page.Goto(loginURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(20000),
	}); err != nil {
		return fmt.Errorf("打开登录页失败: %w", err)
	}

	// 二维码由页面脚本异步渲染；等它真正可见再截图，否则会拿到没有二维码的整页。
	if err := waitQRVisible(page, platform); err != nil {
		return fmt.Errorf("等待二维码出现失败: %w", err)
	}

	checker := p.verifier
	if checker == nil {
		checker = verifyLoginByAccount
	}
	deadline := time.Now().Add(loginTimeout)
	lastQR := time.Time{}
	lastCheck := time.Time{}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 刷新二维码：截取二维码区域或整页。
		if time.Since(lastQR) >= qrRefreshInterval || !session.hasQR() {
			qr, err := captureQR(page, platform)
			if err == nil && qr != "" {
				session.setState("waiting", qr, "", "")
				lastQR = time.Now()
			}
		}

		// 轮询登录态：必须经账号接口确认是真实登录，不能只看 cookie 名称。
		// 夸克未扫码也会写入 b-user-id；只认它会在用户扫码前就把游客 cookie 存下来。
		if time.Since(lastCheck) >= loginCheckInterval {
			lastCheck = time.Now()
			cookie, err := collectPlatformCookie(bctx)
			if err == nil && cookie != "" {
				ok, verr := checker(ctx, platform, cookie)
				if verr != nil {
					// 网络抖动或接口暂不可用，下一轮再试，不作为失败。
					ok = false
				}
				if ok {
					session.setState("success", "", cookie, "")
					return nil
				}
			}
		}

		// 检测已扫码但未完成。
		if scanned := detectScanned(page, platform); scanned {
			session.setState("scanned", "", "", "")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(loginPollInterval):
		}
	}
	session.setState("expired", "", "", "扫码超时，请重试")
	return nil
}

// qrSelector 返回平台的二维码元素选择器。
func qrSelector(platform string) string {
	if platform == "baidu" {
		return baiduQRSelector
	}
	return quarkQRCanvas
}

// qrCanvasHasPixelsJS 判断 canvas 是否已经绘制了二维码。
// 夸克的 canvas 会先以透明画布插入 DOM，约 2.5 秒后才画出二维码；
// 只看元素存在或宽高会抓到全透明（显示为全白）的图片。
func qrCanvasHasPixelsJS(selector string) string {
	return fmt.Sprintf(`() => {
		const el = document.querySelector(%q);
		if (!el || typeof el.toDataURL !== 'function') return false;
		if (el.getBoundingClientRect().width <= 0) return false;
		try {
			const ctx = el.getContext('2d');
			if (!ctx) return false;
			const data = ctx.getImageData(0, 0, el.width, el.height).data;
			let opaque = 0;
			for (let i = 3; i < data.length; i += 16) { if (data[i] > 32) opaque++; }
			return opaque > 20;
		} catch (e) { return false; }
	}`, selector)
}

// waitQRVisible 等待二维码真正渲染完成。用 JS 判断避免 React 重挂载导致 locator 失效。
func waitQRVisible(page playwright.Page, platform string) error {
	expr := ""
	if platform == "quark" {
		expr = qrCanvasHasPixelsJS(qrSelector(platform))
	} else {
		expr = fmt.Sprintf(`() => { const el = document.querySelector(%q); return !!el && el.getBoundingClientRect().width > 0 && el.complete && el.naturalWidth > 0; }`, qrSelector(platform))
	}
	_, err := page.WaitForFunction(expr, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(float64(qrWaitTimeout.Milliseconds())),
	})
	return err
}

// captureCanvasQR 从 canvas 直接导出 data URL。夸克的二维码 canvas 会被 React
// 反复重挂载，locator 截图经常报 "Element is not attached to the DOM"；
// 用一次同步的 JS 求值取当前 canvas 内容，避免这个竞态。
func captureCanvasQR(page playwright.Page, selector string) (string, error) {
	expr := fmt.Sprintf(`() => {
		const el = document.querySelector(%q);
		if (!el || typeof el.toDataURL !== 'function') return '';
		try {
			const ctx = el.getContext('2d');
			if (!ctx) return '';
			const data = ctx.getImageData(0, 0, el.width, el.height).data;
			let opaque = 0;
			for (let i = 3; i < data.length; i += 16) { if (data[i] > 32) opaque++; }
			if (opaque <= 20) return '';
		} catch (e) { return ''; }
		return el.toDataURL('image/png');
	}`, selector)
	raw, err := page.Evaluate(expr)
	if err != nil {
		return "", err
	}
	qr, ok := raw.(string)
	if !ok || qr == "" {
		return "", fmt.Errorf("canvas 尚未生成二维码")
	}
	return qr, nil
}

// captureQR 抓取二维码图片。选择器集中在此，平台改版只改这里。
func captureQR(page playwright.Page, platform string) (string, error) {
	if platform == "quark" {
		// 未绘制完成时返回错误，绝不用空白图覆盖已有的有效二维码。
		for i := 0; i < 12; i++ {
			qr, err := captureCanvasQR(page, quarkQRCanvas)
			if err == nil && qr != "" {
				return qr, nil
			}
			time.Sleep(300 * time.Millisecond)
		}
		return "", fmt.Errorf("二维码尚未绘制完成")
	}

	locator := page.Locator(qrSelector(platform), playwright.PageLocatorOptions{})
	if locator == nil {
		return screenshotPage(page)
	}
	count, err := locator.Count()
	if err != nil || count == 0 {
		return screenshotPage(page)
	}
	img, err := locator.First().Screenshot()
	if err != nil {
		return screenshotPage(page)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(img), nil
}

// screenshotPage 截取整页作为二维码兜底。
func screenshotPage(page playwright.Page) (string, error) {
	img, err := page.Screenshot()
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(img), nil
}

// detectScanned 检测是否已扫码（平台展示"确认登录"等提示）。
func detectScanned(page playwright.Page, platform string) bool {
	hints := []string{"确认登录", "请在手机", "已扫描", "登录确认", "请使用百度App"}
	text, err := page.Locator("body").TextContent()
	if err != nil {
		return false
	}
	for _, hint := range hints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}
