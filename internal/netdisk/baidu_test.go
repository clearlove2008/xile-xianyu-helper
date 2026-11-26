package netdisk

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestBaiduTransfer_Flow 覆盖百度一键转存完整流程。
func TestBaiduTransfer_Flow(t *testing.T) {
	// 分享页 HTML 含 share_uk 与 shareid。
	sharePage := `<html><script>window.share_uk=123456; var shareid = 987654321;</script></html>`
	doer := &fakeDoer{handler: func(req *http.Request) (int, string) {
		u := req.URL.String()
		switch {
		case strings.Contains(u, "/s/") && req.Header.Get("Accept") == "":
			return 200, sharePage
		case strings.Contains(u, "gettemplatevariable"):
			return 200, `{"errno":0,"result":{"bdstoken":"bd-tok"}}`
		case strings.Contains(u, "/share/verify"):
			return 200, `{"errno":0,"randsk":"randsk-1"}`
		case strings.Contains(u, "/share/list"):
			return 200, `{"errno":0,"list":[{"fs_id":111,"path":"/资料.pdf","file_name":"资料.pdf","isdir":0},{"fs_id":222,"path":"/课程.mp4","file_name":"课程.mp4","isdir":0}]}`
		case strings.Contains(u, "/share/transfer"):
			return 200, `{"errno":0,"taskid":555}`
		case strings.Contains(u, "/share/taskquery"):
			return 200, `{"errno":0,"status":4}`
		case strings.Contains(u, "/api/list"):
			return 200, `{"errno":0,"list":[{"fs_id":111,"path":"/资料.pdf","filename":"资料.pdf"},{"fs_id":222,"path":"/课程.mp4","filename":"课程.mp4"}]}`
		case strings.Contains(u, "/share/set"):
			return 200, `{"errno":0,"link":"https://pan.baidu.com/s/mylink","pwd":"9xk2"}`
		default:
			return 200, `{"errno":0}`
		}
	}}
	// 分享页请求带 text/html Accept，需要单独处理。
	doer.handler = func(req *http.Request) (int, string) {
		u := req.URL.String()
		switch {
		case strings.Contains(u, "pan.baidu.com/s/"):
			return 200, sharePage
		case strings.Contains(u, "gettemplatevariable"):
			return 200, `{"errno":0,"result":{"bdstoken":"bd-tok"}}`
		case strings.Contains(u, "/share/verify"):
			return 200, `{"errno":0,"randsk":"randsk-1"}`
		case strings.Contains(u, "/share/list"):
			return 200, `{"errno":0,"list":[{"fs_id":111,"path":"/资料.pdf","file_name":"资料.pdf","isdir":0},{"fs_id":222,"path":"/课程.mp4","file_name":"课程.mp4","isdir":0}]}`
		case strings.Contains(u, "/share/transfer"):
			return 200, `{"errno":0,"taskid":555}`
		case strings.Contains(u, "/share/taskquery"):
			return 200, `{"errno":0,"status":4}`
		case strings.Contains(u, "/api/list"):
			return 200, `{"errno":0,"list":[{"fs_id":111,"path":"/资料.pdf","filename":"资料.pdf"},{"fs_id":222,"path":"/课程.mp4","filename":"课程.mp4"}]}`
		case strings.Contains(u, "/share/set"):
			return 200, `{"errno":0,"link":"https://pan.baidu.com/s/mylink","pwd":"9xk2"}`
		default:
			return 200, `{"errno":0}`
		}
	}

	client := &BaiduClient{Doer: doer}
	result, err := client.Transfer(context.Background(), ShareInput{
		ShareURL:    "https://pan.baidu.com/s/abc123xyz?pwd=abcd",
		Cookie:      "BDUSS=test",
		NewPasscode: "9xk2",
		PeriodDays:  0,
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if result.ShareURL != "https://pan.baidu.com/s/mylink" || result.Passcode != "9xk2" {
		t.Fatalf("结果不匹配: %+v", result)
	}
	if result.FileCount != 2 || result.Platform != "baidu" || !strings.Contains(result.Title, "2 个文件") {
		t.Fatalf("结果字段不匹配: %+v", result)
	}
}

// TestBaiduVerify_ParseFields 覆盖分享页字段解析与链接 pwd 提取。
func TestBaiduVerify_ParseFields(t *testing.T) {
	doer := &fakeDoer{handler: func(req *http.Request) (int, string) {
		if strings.Contains(req.URL.String(), "/share/verify") {
			return 200, `{"errno":0,"randsk":"sk"}`
		}
		return 200, `<script>share_uk:555; shareid: 666;</script>`
	}}
	client := &BaiduClient{Doer: doer}
	info, err := client.baiduVerify(context.Background(), "BDUSS=x", "https://pan.baidu.com/s/1abc?pwd=wxyz", "")
	if err != nil {
		t.Fatalf("baiduVerify: %v", err)
	}
	if info.UK != "555" || info.ShareID != "666" || info.SURL != "1abc" || info.CleanS != "abc" {
		t.Fatalf("解析字段不匹配: %+v", info)
	}
	if info.SeKey != "sk" {
		t.Fatalf("sekey 应从 randsk 获取: %+v", info)
	}
}

// TestBaiduTransfer_Errors 覆盖失败路径。
func TestBaiduTransfer_Errors(t *testing.T) {
	ctx := context.Background()
	client := &BaiduClient{Doer: &fakeDoer{handler: func(*http.Request) (int, string) { return 200, `{}` }}}

	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.baidu.com/s/x"}); err == nil {
		t.Fatal("空 cookie 应报错")
	}
	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.baidu.com/", Cookie: "BDUSS=x"}); err == nil {
		t.Fatal("无效链接应报错")
	}

	// 提取码错误。
	client = &BaiduClient{Doer: &fakeDoer{handler: func(req *http.Request) (int, string) {
		if strings.Contains(req.URL.String(), "/share/verify") {
			return 200, `{"errno":10,"err_msg":"提取码错误"}`
		}
		return 200, `<script>share_uk:1; shareid:2;</script>`
	}}}
	if _, err := client.Transfer(ctx, ShareInput{ShareURL: "https://pan.baidu.com/s/x", Cookie: "BDUSS=x", Passcode: "abcd"}); err == nil || !strings.Contains(err.Error(), "提取码") {
		t.Fatalf("提取码错误应返回明确错误: %v", err)
	}
}
