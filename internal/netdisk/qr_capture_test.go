package netdisk

import (
	"encoding/base64"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// qrCanvasPage 模拟夸克登录页：canvas 先出现，延迟一段时间才真正绘制二维码。
const qrCanvasPage = `<!doctype html><html><body>
<canvas id="react-qrcode-logo" width="180" height="180"></canvas>
<script>
setTimeout(function(){
  var c = document.getElementById('react-qrcode-logo');
  var ctx = c.getContext('2d');
  ctx.fillStyle = '#000';
  for (var i = 0; i < 18; i++) ctx.fillRect(i * 10, 0, 5, 180);
}, 1500);
</script></body></html>`

// dataURLHasOpaquePixels 判断二维码图片是否真的画了内容（透明 canvas 导出后是全白）。
func dataURLHasOpaquePixels(t *testing.T, dataURL string) bool {
	t.Helper()
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		t.Fatalf("损坏的 data URL: %q", dataURL[:min64(len(dataURL))])
	}
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("解码二维码失败: %v", err)
	}
	img, err := png.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("解析二维码 PNG 失败: %v", err)
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				return true
			}
		}
	}
	return false
}

func min64(n int) int {
	if n < 64 {
		return n
	}
	return 64
}

// TestCaptureQRWaitsForCanvasContent 复现线上白图：元素存在但尚未绘制时不能返回二维码。
func TestCaptureQRWaitsForCanvasContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, qrCanvasPage)
	}))
	defer srv.Close()

	pw, err := playwright.Run()
	if err != nil {
		t.Skipf("playwright 不可用: %v", err)
	}
	defer pw.Stop() //nolint:errcheck
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless:       playwright.Bool(true),
		ExecutablePath: browserExecutable(),
		Args:           []string{"--no-sandbox", "--disable-dev-shm-usage"},
	})
	if err != nil {
		t.Fatalf("启动浏览器失败: %v", err)
	}
	defer browser.Close() //nolint:errcheck
	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("创建页面失败: %v", err)
	}
	if _, err := page.Goto(srv.URL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded, Timeout: playwright.Float(10000)}); err != nil {
		t.Fatalf("打开测试页失败: %v", err)
	}

	qr, err := captureQR(page, "quark")
	if err != nil {
		t.Fatalf("抓取二维码失败: %v", err)
	}
	if !dataURLHasOpaquePixels(t, qr) {
		t.Fatal("canvas 尚未绘制就被抓取，返回了透明（全白）图片")
	}
}
