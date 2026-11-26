package netdisk

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeDoer 按请求 URL 的关键字返回预设响应，用于离线覆盖完整转存流程。
type fakeDoer struct {
	handler func(req *http.Request) (int, string)
	calls   []string
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.calls = append(f.calls, req.URL.String())
	status, body := f.handler(req)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

// TestQuarkTransfer_Flow 覆盖夸克一键转存的完整 6 步流程。
func TestQuarkTransfer_Flow(t *testing.T) {
	doer := &fakeDoer{handler: func(req *http.Request) (int, string) {
		u := req.URL.String()
		switch {
		case strings.Contains(u, "/share/sharepage/token"):
			return 200, `{"status":200,"data":{"stoken":"st-123"}}`
		case strings.Contains(u, "/share/sharepage/detail"):
			return 200, `{"status":200,"data":{"list":[{"fid":"f1","share_fid_token":"t1","file_name":"资料.pdf"},{"fid":"f2","share_fid_token":"t2","file_name":"课程.mp4"}]}}`
		case strings.Contains(u, "/share/sharepage/save"):
			return 200, `{"status":200,"data":{"task_id":"task-1"}}`
		case strings.Contains(u, "/clouddrive/task"):
			return 200, `{"status":200,"data":{"status":2}}`
		case strings.Contains(u, "/file/sort"):
			return 200, `{"status":200,"data":{"list":[{"fid":"nf1","file_name":"资料.pdf"},{"fid":"nf2","file_name":"课程.mp4"}]}}`
		case strings.Contains(u, "/clouddrive/share?") && req.Method == "POST":
			return 200, `{"status":200,"data":{"share_id":"sid-9"}}`
		case strings.Contains(u, "/share/password"):
			return 200, `{"status":200,"data":{"share_url":"https://pan.quark.cn/s/myshare"}}`
		default:
			return 200, `{"status":200,"data":{}}`
		}
	}}
	client := &QuarkClient{Doer: doer}
	result, err := client.Transfer(context.Background(), ShareInput{
		ShareURL:    "https://pan.quark.cn/s/sourceid123456",
		Passcode:    "ab12",
		Cookie:      "b-user-id=test",
		NewPasscode: "9xk2",
		PeriodDays:  0,
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if result.ShareURL != "https://pan.quark.cn/s/myshare" {
		t.Fatalf("分享链接不匹配: %s", result.ShareURL)
	}
	if result.Passcode != "9xk2" || result.FileCount != 2 || result.Platform != "quark" {
		t.Fatalf("结果字段不匹配: %+v", result)
	}
	if !strings.Contains(result.Title, "资料.pdf") || !strings.Contains(result.Title, "2 个文件") {
		t.Fatalf("标题不匹配: %s", result.Title)
	}
	if len(doer.calls) < 7 {
		t.Fatalf("调用步骤不足: %d", len(doer.calls))
	}
}

// TestQuarkTransfer_ExtractPwdID 覆盖链接解析的边界。
func TestQuarkTransfer_ExtractPwdID(t *testing.T) {
	cases := []struct {
		url  string
		want string
		err  bool
	}{
		{"https://pan.quark.cn/s/abc123", "abc123", false},
		{"https://pan.quark.cn/s/abc123?pwd=1234", "abc123", false},
		{"abc123", "abc123", false},
		{"https://pan.quark.cn/", "", true},
	}
	for _, tc := range cases {
		got, err := extractQuarkPwdID(tc.url)
		if tc.err {
			if err == nil {
				t.Fatalf("%s 应报错", tc.url)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%s: got=%q err=%v want=%q", tc.url, got, err, tc.want)
		}
	}
}

// TestQuarkTransfer_Errors 覆盖关键失败路径。
func TestQuarkTransfer_Errors(t *testing.T) {
	ctx := context.Background()

	// 空 cookie。
	client := &QuarkClient{Doer: &fakeDoer{handler: func(*http.Request) (int, string) { return 200, `{}` }}}
	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.quark.cn/s/x"}); err == nil {
		t.Fatal("空 cookie 应报错")
	}

	// stoken 失败（需要提取码）。
	client = &QuarkClient{Doer: &fakeDoer{handler: func(*http.Request) (int, string) {
		return 200, `{"status":400,"code":31002,"message":"need passcode"}`
	}}}
	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.quark.cn/s/x", Cookie: "c"}); err == nil || !strings.Contains(err.Error(), "stoken") {
		t.Fatalf("stoken 失败应返回明确错误: %v", err)
	}

	// 空分享。
	client = &QuarkClient{Doer: &fakeDoer{handler: func(req *http.Request) (int, string) {
		if strings.Contains(req.URL.String(), "/detail") {
			return 200, `{"status":200,"data":{"list":[]}}`
		}
		return 200, `{"status":200,"data":{"stoken":"s"}}`
	}}}
	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.quark.cn/s/x", Cookie: "c"}); err == nil || !strings.Contains(err.Error(), "没有可转存") {
		t.Fatalf("空分享应报错: %v", err)
	}
}
