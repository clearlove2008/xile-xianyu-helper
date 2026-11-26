package mtop

import (
	"fmt"
	"strings"
	"testing"
)

// TestOpportunityDetailPreservesOriginal 验证长文、空行、缩进及自然省略号完全保留，不替换成标题摘要。
func TestOpportunityDetailPreservesOriginal(t *testing.T) {
	// original 是远长于列表摘要的原始多段落文案。
	original := "  原标题\r\n\r\n一、目录\n  1. 第一项\n  2. 第二项…\n" + strings.Repeat("正文内容\n", 2000) + "\n最后一段完整结尾  "
	// result、err 以模拟平台详情测试字节级完整性。
	result, err := parseOpportunityDetail(map[string]any{"data": map[string]any{"itemDO": map[string]any{"itemId": "123", "title": "摘要...", "desc": original}}}, "123")
	if err != nil || result.Description != original {
		t.Fatal("详情原文被修改或截断")
	}
}

// TestOpportunityDetailImages 验证详情接口返回的全部商品图被解析、去重、补协议头并限制在发布上限内。
func TestOpportunityDetailImages(t *testing.T) {
	imageInfos := []any{
		map[string]any{"url": "http://img.alicdn.com/one.jpg", "major": true},
		map[string]any{"url": "https://img.alicdn.com/one.jpg"},
		map[string]any{"url": "//img.alicdn.com/two.jpg"},
		map[string]any{"url": "not-a-url"},
	}
	for i := 0; i < 12; i++ {
		imageInfos = append(imageInfos, map[string]any{"url": fmt.Sprintf("https://img.alicdn.com/extra-%d.jpg", i)})
	}
	result, err := parseOpportunityDetail(map[string]any{"data": map[string]any{"itemDO": map[string]any{
		"itemId": "123", "desc": "完整描述", "imageInfos": imageInfos,
	}}}, "123")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://img.alicdn.com/one.jpg", "https://img.alicdn.com/two.jpg"}
	if len(result.ImageURLs) != 9 || result.ImageURLs[0] != want[0] || result.ImageURLs[1] != want[1] {
		t.Fatalf("图片列表不符合预期: %v", result.ImageURLs)
	}
	for _, u := range result.ImageURLs {
		if !strings.HasPrefix(u, "https://") {
			t.Fatalf("图片地址未补协议头: %s", u)
		}
	}
}

// TestOpportunityDetailRejectsMissing 防止详情失败后悄悄发布截断摘要。
func TestOpportunityDetailRejectsMissing(t *testing.T) {
	for _, item := range []map[string]any{{"title": "摘要..."}, {"desc": "  \n "}, {"itemId": "456", "desc": "其他商品"}} {
		if _, err := parseOpportunityDetail(map[string]any{"data": map[string]any{"itemDO": item}}, "123"); err == nil {
			t.Fatal("不完整或不匹配的详情被接受")
		}
	}
}
