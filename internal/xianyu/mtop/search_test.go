package mtop

import (
	"testing"
)

// buildSearchResponse 构造一个最小可用的搜索成功响应。
func buildSearchResponse() map[string]any {
	return map[string]any{
		"api": searchAPIName,
		"ret": []any{"SUCCESS::调用成功"},
		"data": map[string]any{
			"resultInfo": map[string]any{"hasNextPage": true, "numFound": float64(1234)},
			"resultList": []any{
				map[string]any{
					"data": map[string]any{
						"item": map[string]any{
							"main": map[string]any{
								"exContent": map[string]any{
									"itemId":       "1060173583182",
									"title":        "露营车 折叠小推车",
									"picUrl":       "//img.alicdn.com/a.jpg",
									"userNickName": "老王",
									"area":         "浙江杭州",
									"price": []any{
										map[string]any{"type": "integer", "text": "160"},
										map[string]any{"type": "decimal", "text": ".5"},
									},
								},
								"clickParam": map[string]any{
									"args": map[string]any{"price": "999", "itemId": "1060173583182"},
								},
							},
						},
					},
				},
				map[string]any{
					"data": map[string]any{
						"item": map[string]any{
							"main": map[string]any{
								"exContent": map[string]any{
									"itemId": "2098765432101",
									"title":  "二手相机",
									"picUrl": "https://img.alicdn.com/b.jpg",
									"area":   "广东深圳",
									"price": []any{
										map[string]any{"type": "integer", "text": "3200"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// 深层嵌套结构应完整解析：标题、价格（整数+小数拼接）、图片协议头、卖家、分页。
func TestParseSearchResponse(t *testing.T) {
	result := parseSearchResponse(buildSearchResponse())
	if len(result.Items) != 2 {
		t.Fatalf("应解析出 2 条商品，实际 %d", len(result.Items))
	}
	first := result.Items[0]
	if first.ItemID != "1060173583182" {
		t.Fatalf("itemId = %q", first.ItemID)
	}
	if first.Title != "露营车 折叠小推车" {
		t.Fatalf("title = %q", first.Title)
	}
	if first.Price != "160.5" {
		t.Fatalf("整数+小数应拼接为 160.5，实际 %q", first.Price)
	}
	if first.ImageURL != "https://img.alicdn.com/a.jpg" {
		t.Fatalf("图片应补 https 协议头，实际 %q", first.ImageURL)
	}
	if first.Seller != "老王" {
		t.Fatalf("seller = %q", first.Seller)
	}
	if first.Area != "浙江杭州" {
		t.Fatalf("area = %q", first.Area)
	}
	if first.ItemURL != "https://www.goofish.com/item?id=1060173583182" {
		t.Fatalf("itemUrl = %q", first.ItemURL)
	}
	second := result.Items[1]
	if second.Price != "3200" {
		t.Fatalf("纯整数价格应为 3200，实际 %q", second.Price)
	}
	if second.Seller != "" {
		t.Fatalf("缺失卖家应为空，实际 %q", second.Seller)
	}
	if !result.HasMore || result.Total != 1234 {
		t.Fatalf("分页信息错误: hasMore=%v total=%d", result.HasMore, result.Total)
	}
}

// exContent 缺价格时应回退到 clickParam.args.price。
func TestSearchPriceFallbackToClickParam(t *testing.T) {
	entry := map[string]any{
		"data": map[string]any{
			"item": map[string]any{
				"main": map[string]any{
					"exContent":  map[string]any{"itemId": "1001", "title": "无价格商品"},
					"clickParam": map[string]any{"args": map[string]any{"price": "88"}},
				},
			},
		},
	}
	item := searchItemFromResultEntry(entry)
	if item.Price != "88" {
		t.Fatalf("应回退到 clickParam 价格，实际 %q", item.Price)
	}
}

// 结果容器放到 items 时（平台结构变动兜底）也应解析。
func TestParseSearchResponseItemsFallback(t *testing.T) {
	response := map[string]any{
		"ret": []any{"SUCCESS::调用成功"},
		"data": map[string]any{
			"items": []any{
				map[string]any{
					"data": map[string]any{
						"item": map[string]any{
							"main": map[string]any{
								"exContent": map[string]any{"itemId": "777", "title": "兜底商品"},
							},
						},
					},
				},
			},
		},
	}
	result := parseSearchResponse(response)
	if len(result.Items) != 1 || result.Items[0].ItemID != "777" {
		t.Fatalf("items 容器兜底解析失败: %+v", result.Items)
	}
}

// 数据缺失或类型不对时返回空结果而不是崩溃。
func TestParseSearchResponseMalformed(t *testing.T) {
	result := parseSearchResponse(map[string]any{"data": map[string]any{"resultList": "not-a-list"}})
	if len(result.Items) != 0 {
		t.Fatalf("畸形数据应返回空列表，实际 %d", len(result.Items))
	}
	empty := parseSearchResponse(nil)
	if len(empty.Items) != 0 {
		t.Fatalf("空响应应返回空列表")
	}
}

// 图片 URL 归一化：协议相对、http、完整 https 三种形态。
func TestNormalizeImageURL(t *testing.T) {
	cases := map[string]string{
		"//img.alicdn.com/x.jpg":   "https://img.alicdn.com/x.jpg",
		"http://img.alicdn.com/y":  "https://img.alicdn.com/y",
		"https://img.a.com/z":      "https://img.a.com/z",
		"":                         "",
		"  //img.alicdn.com/w.jpg": "https://img.alicdn.com/w.jpg",
	}
	for in, want := range cases {
		if got := normalizeImageURL(in); got != want {
			t.Fatalf("normalizeImageURL(%q) = %q, want %q", in, got, want)
		}
	}
}
