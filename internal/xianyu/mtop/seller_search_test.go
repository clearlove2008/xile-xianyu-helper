package mtop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// sellerTestTransport 是离线 MTOP 替身，不对真实闲鱼发送请求。
type sellerTestTransport struct {
	t     *testing.T
	calls int
}

// RoundTrip 验证目标卖家来自请求参数而非登录账号，并返回分页空结果。
func (transport *sellerTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.calls++
	if err := request.ParseForm(); err != nil {
		transport.t.Fatal(err)
	}
	// payload 是测试签名请求的业务参数；不记录请求凭证。
	var payload map[string]any
	if err := json.Unmarshal([]byte(request.FormValue("data")), &payload); err != nil {
		transport.t.Fatal(err)
	}
	if payload["userId"] != "88" || payload["pageNumber"] != float64(2) || payload["pageSize"] != float64(20) {
		transport.t.Fatal("卖家或分页参数不正确")
	}
	if _, exists := payload["groupId"]; exists {
		transport.t.Fatal("不应复用登录账号分组")
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ret":["SUCCESS::调用成功"],"data":{"nextPage":false}}`)), Request: request}, nil
}

// TestSellerSearchRequest 验证新分支只读取一次，无登录重试或额外平台调用。
func TestSellerSearchRequest(t *testing.T) {
	// transport 拦截实际请求用于离线协议验证。
	transport := &sellerTestTransport{t: t}
	// client 使用隔离 HTTP 传输，不接触本机账号。
	client := &ClientImpl{HTTPClient: &http.Client{Transport: transport}}
	// result、err 是显式卖家模式的返回。
	result, err := client.SearchOpportunityItems(context.Background(), "_m_h5_tk=fixture_9999999999999; unb=11", "", 2, 50, "88")
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 2 || result.PageSize != 20 || transport.calls != 1 {
		t.Fatal("分页或调用次数不正确")
	}
}

// TestSellerSearchFilters 验证仅接受目标卖家明确在售商品，并移除重复、已售与推荐卡片。
func TestSellerSearchFilters(t *testing.T) {
	// card 构造公开主页的最小商品卡片。
	card := func(id, owner string, status any) any {
		return map[string]any{"cardType": 1003, "cardData": map[string]any{"id": id, "userId": owner, "itemStatus": status, "title": "商品", "priceInfo": map[string]any{"price": "12.50"}, "picInfo": map[string]any{"picUrl": "//img.alicdn.com/a.jpg"}}}
	}
	// result 同时覆盖重复 ID、错误归属、已售出和状态缺失。
	result := parseSellerSearchResponse(map[string]any{"data": map[string]any{"nextPage": true, "totalCount": 5, "cardList": []any{card("1", "88", 0), card("1", "88", 0), card("2", "88", 1), card("3", "99", 0), card("4", "88", nil)}}}, "88")
	if len(result.Items) != 1 || result.Items[0].ItemID != "1" || result.Items[0].Price != "12.50" || result.Items[0].ImageURL != "https://img.alicdn.com/a.jpg" || !result.HasMore {
		t.Fatal("卖家过滤或分页结果不正确")
	}
}

// TestSellerSearchEmpty 成功响应省略 cardList 应返回有效空页。
func TestSellerSearchEmpty(t *testing.T) {
	if result := parseSellerSearchResponse(map[string]any{"data": map[string]any{}}, "88"); len(result.Items) != 0 || result.HasMore {
		t.Fatal("空页解析失败")
	}
}
