package netdisk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeStarter 是注入的假浏览器启动器，按平台模拟扫码成功/失败。
type fakeStarter struct {
	platform string
	cookie   string
	err      error
	qr       bool
}

func (f *fakeStarter) Start(ctx context.Context, session *QRLoginSession, platform string) error {
	f.platform = platform
	if f.qr {
		session.setState("waiting", "data:image/png;base64,AAAA", "", "")
	}
	if f.err != nil {
		return f.err
	}
	session.setState("success", "", f.cookie, "")
	return nil
}

// TestQRLoginManager_BeginSuccess 覆盖扫码会话从创建到成功的状态流转。
func TestQRLoginManager_BeginSuccess(t *testing.T) {
	m := NewQRLoginManager()
	m.newID = func() string { return "sess-1" }
	m.browserStarter = &fakeStarter{cookie: "b-user-id=ok; __puus=x"}

	status, err := m.Begin(context.Background(), "quark")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if status.SessionID != "sess-1" || status.Platform != "quark" {
		t.Fatalf("会话初始状态不匹配: %+v", status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, err = m.Status("sess-1")
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.State == "success" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.State != "success" || status.Cookie != "b-user-id=ok; __puus=x" {
		t.Fatalf("扫码成功后应返回 cookie: %+v", status)
	}
}

// TestQRLoginManager_InvalidPlatform 覆盖非法平台拒绝。
func TestQRLoginManager_InvalidPlatform(t *testing.T) {
	m := NewQRLoginManager()
	if _, err := m.Begin(context.Background(), "xunlei"); err == nil {
		t.Fatal("非法平台应报错")
	}
}

// TestQRLoginManager_FailureAndCancel 覆盖失败状态与取消。
func TestQRLoginManager_FailureAndCancel(t *testing.T) {
	m := NewQRLoginManager()
	m.newID = func() string { return "sess-fail" }
	m.browserStarter = &fakeStarter{err: errors.New("浏览器启动失败")}

	if _, err := m.Begin(context.Background(), "baidu"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := m.Status("sess-fail")
		if status.State == "failed" {
			if status.Error != "浏览器启动失败" {
				t.Fatalf("失败原因不匹配: %s", status.Error)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _ := m.Status("sess-fail")
	if status.State != "failed" {
		t.Fatalf("应为 failed，got %s", status.State)
	}

	if err := m.Cancel("sess-fail"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := m.Status("sess-fail"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("取消后应为 ErrSessionNotFound，got %v", err)
	}
	if err := m.Cancel("not-exist"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("取消不存在会话应报错: %v", err)
	}
}

// TestQRLoginManager_CookieOnlyOnSuccess 覆盖 cookie 只在 success 状态暴露。
func TestQRLoginManager_CookieOnlyOnSuccess(t *testing.T) {
	session := &QRLoginSession{id: "s", platform: "quark", state: "waiting"}
	session.setState("waiting", "data:image/png;base64,AAAA", "secret-cookie", "")
	if got := session.Snapshot().Cookie; got != "" {
		t.Fatalf("waiting 状态不应暴露 cookie: %s", got)
	}
	session.setState("success", "", "", "")
	if got := session.Snapshot().Cookie; got != "secret-cookie" {
		t.Fatalf("success 状态应暴露 cookie: %s", got)
	}
}

// delayedQRStarter 模拟浏览器启动有耗时，且二维码在这之后才可用。
type delayedQRStarter struct{}

func (f *delayedQRStarter) Start(ctx context.Context, session *QRLoginSession, platform string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}
	session.setState("waiting", "data:image/png;base64,QR", "", "")
	return nil
}

// TestQRLoginManager_BeginSurvivesCallerCancel 覆盖扫码会话不随发起请求的结束而取消。
// HTTP handler 返回后 request context 会被取消；若会话直接绑定它，浏览器还没抓到
// 二维码就被终止，前端会永远停在“正在生成二维码”。
func TestQRLoginManager_BeginSurvivesCallerCancel(t *testing.T) {
	m := NewQRLoginManager()
	m.newID = func() string { return "sess-request-cancel" }
	m.browserStarter = &delayedQRStarter{}

	reqCtx, cancel := context.WithCancel(context.Background())
	if _, err := m.Begin(reqCtx, "quark"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	cancel() // 模拟发起扫码的 HTTP 请求已返回

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, err := m.Status("sess-request-cancel")
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.QRDataURL != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("请求 context 取消后仍应生成二维码，实际一直停在生成中")
}

// TestVerifyLoginByAccount_Quark 覆盖夸克登录态校验：游客态必须被拒绝，登录态才通过。
func TestVerifyLoginByAccount_Quark(t *testing.T) {
	original := quarkAccountInfoURL
	defer func() { quarkAccountInfoURL = original }()

	cases := []struct {
		name     string
		response string
		want     bool
	}{
		{name: "游客态 data 为空", response: `{"success":true,"data":{},"code":"OK"}`, want: false},
		{name: "登录态有昵称", response: `{"success":true,"data":{"nickname":"测试用户","user_id":"123"},"code":"OK"}`, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") == "" {
					t.Error("校验请求必须带 cookie")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer srv.Close()
			quarkAccountInfoURL = srv.URL
			got, err := verifyLoginByAccount(context.Background(), "quark", "b-user-id=guest; ctoken=x")
			if err != nil {
				t.Fatalf("verifyLoginByAccount: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestVerifyLoginByAccount_EmptyCookie 覆盖空 cookie 直接判定为未登录。
func TestVerifyLoginByAccount_EmptyCookie(t *testing.T) {
	got, err := verifyLoginByAccount(context.Background(), "quark", "  ")
	if err != nil {
		t.Fatalf("verifyLoginByAccount: %v", err)
	}
	if got {
		t.Fatal("空 cookie 不应判定为登录")
	}
}
